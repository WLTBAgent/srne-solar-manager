package controller

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/goburrow/modbus"
	"github.com/gofrs/flock"
)

// Version is the application version string.
const Version = "2.1.0"

// Defaults for connection handling.
const (
	DefaultTimeout  = 5 * time.Second // conservative default, suits the CLI
	portLockRetries = 30
	portLockDelay   = 100 * time.Millisecond

	// transactionPause is inserted after every bus transaction. The
	// gateways inside integrated units drop or garble back-to-back
	// frames; a short quiet gap keeps them happy.
	transactionPause = 30 * time.Millisecond
)

// Register represents a single Modbus register from the PDF spec.
type Register struct {
	Address     uint16            `json:"address"` // Modbus address (hex from spec)
	Length      int               `json:"length"`  // Number of 16-bit registers
	Name        string            `json:"name"`    // Human-readable name
	Access      string            `json:"access"`  // "R", "W", "RW"
	Scale       float64           `json:"scale"`   // Magnification factor (e.g. 0.1 means raw/10)
	Unit        string            `json:"unit"`    // Unit string ("V", "A", "°C", etc.)
	Signed      bool              `json:"signed"`  // Whether value is signed
	Min         float64           `json:"min"`     // Min configurable value (display units)
	Max         float64           `json:"max"`     // Max configurable value (display units)
	Default     float64           `json:"default"` // Default value (display units)
	Enum        map[uint16]string `json:"enum,omitempty"`
	Invert      bool              `json:"invert,omitempty"` // Sign-flip for display: device reports discharge-positive
	Description string            `json:"description,omitempty"`
	Category    string            `json:"category"`
}

// Reading holds a decoded register reading.
type Reading struct {
	Address  uint16   `json:"address"`
	Name     string   `json:"name"`
	Raw      uint16   `json:"raw"`
	RawWords []uint16 `json:"rawWords,omitempty"`
	Decoded  string   `json:"decoded"`
	Unit     string   `json:"unit,omitempty"`
	Category string   `json:"category"`
}

// Connection holds the serial connection configuration.
type Connection struct {
	Port    string
	Device  int
	Baud    int
	Timeout time.Duration // zero means DefaultTimeout
}

// Controller manages a Modbus connection to the charge controller.
// It is safe for concurrent use: all bus transactions are serialized
// internally, so callers may read/write from multiple goroutines.
type Controller struct {
	cfg     Connection
	client  modbus.Client
	handler *modbus.RTUClientHandler
	locker  *flock.Flock

	mu sync.Mutex // serializes access to client (Modbus RTU allows one transaction at a time)
}

// NewController creates a new controller with the given connection settings.
func NewController(cfg Connection) *Controller {
	return &Controller{cfg: cfg}
}

// Connect establishes the serial connection.
// It waits at most portLockRetries*portLockDelay for the port lock and
// returns a descriptive error instead of blocking forever.
func (c *Controller) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return fmt.Errorf("already connected to %s", c.cfg.Port)
	}

	// Fail fast with a clear message when the device node is absent
	// (unplugged adapter, stale PTY path) instead of surfacing the
	// lock library's confusing O_CREAT permission error.
	if _, err := os.Stat(c.cfg.Port); err != nil {
		return fmt.Errorf("serial port %s is not available: %w", c.cfg.Port, err)
	}

	c.locker = flock.New(c.cfg.Port)
	locked := false
	var lastErr error
	for i := 0; i < portLockRetries; i++ {
		got, err := c.locker.TryLock()
		if err != nil {
			lastErr = err
		} else if got {
			locked = true
			break
		} else {
			lastErr = fmt.Errorf("port is locked by another process")
		}
		time.Sleep(portLockDelay)
	}
	if !locked {
		c.locker = nil
		return fmt.Errorf("serial port %s is busy (retry for up to %.0fs): %w",
			c.cfg.Port, portLockRetries*portLockDelay.Seconds(), lastErr)
	}

	timeout := c.cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	c.handler = modbus.NewRTUClientHandler(c.cfg.Port)
	c.handler.BaudRate = c.cfg.Baud
	c.handler.DataBits = 8
	c.handler.Parity = "N"
	c.handler.StopBits = 1
	c.handler.SlaveId = byte(c.cfg.Device)
	c.handler.Timeout = timeout

	if err := c.handler.Connect(); err != nil {
		c.locker.Unlock()
		c.locker = nil
		return fmt.Errorf("cannot connect to %s: %w", c.cfg.Port, err)
	}

	c.client = modbus.NewClient(c.handler)
	return nil
}

// Close releases the serial connection.
func (c *Controller) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handler != nil {
		c.handler.Close()
		c.handler = nil
	}
	c.client = nil
	if c.locker != nil {
		c.locker.Unlock()
		c.locker = nil
	}
}

func (c *Controller) readHolding(addr uint16, count uint16) ([]uint16, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	results, err := c.client.ReadHoldingRegisters(addr, count)
	time.Sleep(transactionPause)
	if err != nil {
		return nil, err
	}
	return BytesToUint16(results), nil
}

// ReadRegister reads a register and returns the raw uint16 values.
func (c *Controller) ReadRegister(reg *Register) ([]uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readHolding(reg.Address, uint16(reg.Length))
}

// ReadChunk reads count registers starting at addr (raw modbus call).
func (c *Controller) ReadChunk(addr uint16, count uint16) ([]uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readHolding(addr, count)
}

// WriteRegister writes a single uint16 value to an address.
func (c *Controller) WriteRegister(addr uint16, val uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return fmt.Errorf("not connected")
	}
	_, err := c.client.WriteSingleRegister(addr, val)
	time.Sleep(transactionPause)
	return err
}

// WriteRegisters writes multiple uint16 values starting at addr (FC16).
func (c *Controller) WriteRegisters(addr uint16, vals []uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return fmt.Errorf("not connected")
	}
	buf := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.BigEndian.PutUint16(buf[i*2:], v)
	}
	_, err := c.client.WriteMultipleRegisters(addr, uint16(len(vals)), buf)
	time.Sleep(transactionPause)
	return err
}

// ReadReading reads a register and returns a decoded Reading.
func (c *Controller) ReadReading(reg *Register) (*Reading, error) {
	raw, err := c.ReadRegister(reg)
	if err != nil {
		return nil, err
	}
	r := &Reading{
		Address:  reg.Address,
		Name:     reg.Name,
		Decoded:  DecodeValue(reg, raw),
		Unit:     reg.Unit,
		Category: reg.Category,
		RawWords: raw,
	}
	if len(raw) > 0 {
		r.Raw = raw[0]
	}
	return r, nil
}

// ReadMultiple reads multiple registers in sequence with a small delay.
func (c *Controller) ReadMultiple(regs []*Register) ([]*Reading, []error) {
	var readings []*Reading
	var errs []error
	for _, reg := range regs {
		r, err := c.ReadReading(reg)
		if err != nil {
			errs = append(errs, fmt.Errorf("0x%04X %s: %w", reg.Address, reg.Name, err))
			continue
		}
		readings = append(readings, r)
		time.Sleep(10 * time.Millisecond)
	}
	return readings, errs
}

// ReadBlock reads a single planned block and maps the raw words back
// to the registers the block covers.
func (c *Controller) ReadBlock(b Block) (map[uint16][]uint16, error) {
	words, err := c.ReadChunk(b.Start, b.Count)
	if err != nil {
		return nil, err
	}
	out := make(map[uint16][]uint16, len(b.Regs))
	for _, reg := range b.Regs {
		off := int(reg.Address - b.Start)
		end := off + reg.Length
		if end <= len(words) {
			out[reg.Address] = words[off:end]
		}
	}
	return out, nil
}

// ReadPlanned reads the given registers using as few block reads as
// possible. It returns a map of address → raw words for every register
// that was read successfully, plus one error per failed block.
func (c *Controller) ReadPlanned(regs []*Register) (map[uint16][]uint16, []error) {
	out := make(map[uint16][]uint16, len(regs))
	var errs []error
	for _, b := range PlanRegisterBlocks(regs, MaxBlockCount, MaxBlockGap) {
		v, err := c.ReadBlock(b)
		if err != nil {
			errs = append(errs, fmt.Errorf("block 0x%04X (%d regs): %w", b.Start, b.Count, err))
			continue
		}
		for addr, words := range v {
			out[addr] = words
		}
	}
	return out, errs
}

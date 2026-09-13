// Package devsim implements a virtual SRNE-style integrated inverter
// controller speaking Modbus RTU. It exists so the GUI and the
// controller package can be exercised end-to-end without hardware.
package devsim

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math/rand"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// Device is a Modbus RTU slave holding a virtual register map.
type Device struct {
	SlaveID byte
	// Deny lists addresses that never answer (emulating dead areas
	// behind a real unit's internal gateway); reads touching them
	// time out on the client.
	Deny []uint16

	mu   sync.Mutex
	regs map[uint16]uint16
	rng  *rand.Rand
}

// denied reports whether the read range [start, start+count) touches a
// denied address.
func (d *Device) denied(start, count uint16) bool {
	for _, a := range d.Deny {
		if a >= start && a < start+count {
			return true
		}
	}
	return false
}

// New creates a device seeded with plausible register values derived
// from the SRNE V1.3 register map defaults.
func New(slaveID byte) *Device {
	d := &Device{
		SlaveID: slaveID,
		regs:    make(map[uint16]uint16),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.seed()
	return d
}

func (d *Device) seed() {
	setString := func(addr uint16, length int, s string) {
		for i := 0; i < length; i++ {
			var c byte
			if i < len(s) {
				c = s[i]
			}
			d.regs[addr+uint16(i)] = uint16(c)
		}
	}

	// Product info
	d.regs[0x000A] = 0x3046 // 48 V system, 70 A rated
	d.regs[0x000B] = 4      // integrated inverter controller
	setString(0x000C, 8, "HF4850S8")
	d.regs[0x0014], d.regs[0x0015] = 100, 103 // CPU1/CPU2 V1.00 / V1.03
	d.regs[0x0016], d.regs[0x0017] = 102, 100 // control / power board
	d.regs[0x0018], d.regs[0x0019] = 0x1234, 0x5678
	d.regs[0x001A] = 1 // RS485 address
	d.regs[0x001B] = 33
	d.regs[0x001C], d.regs[0x001D] = 100, 0
	d.regs[0x001E], d.regs[0x001F] = 0x1905, 0x0612 // built 2019-05-06 12:00
	d.regs[0x0020] = 0                              // Shenzhen
	setString(0x0021, 20, "Mar 12 2024 10:23")
	setString(0x0035, 20, "SN48508020240001")

	// Live controller data (2nd gen, 48 V system)
	now := time.Now()
	d.regs[0x0100] = 76     // SOC %
	d.regs[0x0101] = 536    // 53.6 V
	d.regs[0x0102] = 0xFED4 // device polarity: -300 = charging at 30.0 A
	d.regs[0x0103] = 0x231E
	d.regs[0x0104] = 0
	d.regs[0x0105] = 0
	d.regs[0x0106] = 0
	d.regs[0x0107] = 782 // 78.2 V PV
	d.regs[0x0108] = 63  // 6.3 A PV
	d.regs[0x0109] = 492 // 492 W
	d.regs[0x010A] = 1   // DC load on
	d.regs[0x010B] = 0x8002
	d.regs[0x010C], d.regs[0x010D] = 0, 0

	// Live inverter data
	d.regs[0x0200], d.regs[0x0201], d.regs[0x0202], d.regs[0x0203] = 0, 0, 0, 0
	d.regs[0x0204], d.regs[0x0205], d.regs[0x0206], d.regs[0x0207] = 0, 0, 0, 0
	tv := controller.EncodeTime(now.Year()%100, int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
	for i, w := range tv {
		d.regs[0x020C+uint16(i)] = w
	}
	d.regs[0x0210] = 5    // inverter operation
	d.regs[0x0211] = 0    // no password
	d.regs[0x0212] = 3600 // bus 360.0 V
	d.regs[0x0213] = 2302 // grid 230.2 V
	d.regs[0x0214] = 21   // grid 2.1 A
	d.regs[0x0215] = 5000 // 50.00 Hz
	d.regs[0x0216] = 2300 // out 230.0 V
	d.regs[0x0217] = 87   // out 8.7 A
	d.regs[0x0218] = 5001 // 50.01 Hz
	d.regs[0x0219] = 87   // load 8.7 A
	d.regs[0x021A] = 98   // PF 0.98
	d.regs[0x021B] = 2000 // 2000 W
	d.regs[0x021C] = 2020 // 2020 VA
	d.regs[0x021D] = 0
	d.regs[0x021E] = 0
	d.regs[0x021F] = 40 // 40 % load
	d.regs[0x0220] = 412
	d.regs[0x0221] = 405
	d.regs[0x0222] = 398
	d.regs[0x0223] = 320
	d.regs[0x0224] = 31
	d.regs[0x0225] = 32

	// RW parameter defaults straight from the register map.
	for i := range controller.AllRegisters {
		r := &controller.AllRegisters[i]
		if r.Category == "Battery Params" || r.Category == "Inverter Settings" {
			d.regs[r.Address] = controller.FloatToRaw(r, r.Default)
		}
	}
	// A few realistic overrides for otherwise zero defaults.
	d.regs[0xE00F] = 0x5A14 // charge cutoff 90 %, discharge cutoff 20 %

	// Statistics: today + accumulators + 7-day series.
	seed7 := func(addr uint16, base uint16) {
		for i := 0; i < 7; i++ {
			d.regs[addr+uint16(i)] = base - uint16(i)*2
		}
	}
	seed7(0xF000, 41) // PV Ah/day
	seed7(0xF007, 43)
	seed7(0xF00E, 38)
	seed7(0xF015, 12)
	seed7(0xF01C, 45) // load kWh/day (0.1 → 4.5)
	seed7(0xF023, 8)
	d.regs[0xF02D], d.regs[0xF02E] = 43, 38
	d.regs[0xF02F], d.regs[0xF030] = 49, 45
	d.regs[0xF031] = 231
	d.regs[0xF032], d.regs[0xF033] = 3, 189
	d.regs[0xF034], d.regs[0xF035] = 0, 8123
	d.regs[0xF036], d.regs[0xF037] = 0, 7455
	d.regs[0xF038], d.regs[0xF039] = 1, 2457 // 925.7 kWh
	d.regs[0xF03A], d.regs[0xF03B] = 0, 9102
	d.regs[0xF03C], d.regs[0xF03D] = 12, 8
	d.regs[0xF03E], d.regs[0xF03F] = 611, 12
	d.regs[0xF040], d.regs[0xF041], d.regs[0xF042] = tv[0], tv[1], tv[2]
	d.regs[0xF043], d.regs[0xF044], d.regs[0xF045] = 0x1905, 0x060E, 0x0230
	d.regs[0xF046], d.regs[0xF047] = 0, 1102
	d.regs[0xF048], d.regs[0xF049] = 0, 5120
	d.regs[0xF04A], d.regs[0xF04B] = 3800, 45

	// One historic fault (record 0: battery under-voltage), rest invalid.
	d.regs[0xF800] = 0x01
	d.regs[0xF801], d.regs[0xF802], d.regs[0xF803] = 0x090D, 0x0E23, 0x2D00
}

// Tick jitters the live values so monitoring shows movement.
func (d *Device) Tick() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Jitter in int space; uint16 conversion wraps negatives into the
	// two's complement a real device transmits.
	jitter := func(addr uint16, amount, min, max int) {
		v := int(d.regs[addr]) + d.rng.Intn(2*amount+1) - amount
		if v < min {
			v = min
		}
		if v > max {
			v = max
		}
		d.regs[addr] = uint16(v)
	}

	jitter(0x0100, 1, 40, 100) // SOC
	jitter(0x0101, 2, 500, 560)
	jitter(0x0102, 20, -400, 100) // device polarity: -400..100 = charging 40A .. discharging 10A
	jitter(0x0107, 8, 700, 850)
	jitter(0x0108, 4, 0, 90)
	jitter(0x0109, 20, 0, 900)
	jitter(0x0213, 3, 2200, 2400)
	jitter(0x0216, 2, 2250, 2350)
	jitter(0x0219, 2, 50, 120)
	jitter(0x021B, 20, 1500, 2600)
	jitter(0x0220, 2, 350, 480)
}

// Read implements the Modbus FC03 (read holding registers).
func (d *Device) read(start, count uint16) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]byte, 2*int(count))
	for i := uint16(0); i < count; i++ {
		binary.BigEndian.PutUint16(out[i*2:], d.regs[start+i])
	}
	return out
}

// WriteSingle implements FC06.
func (d *Device) writeSingle(addr, val uint16) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.regs[addr] = val
}

// WriteMulti implements FC16.
func (d *Device) writeMulti(start uint16, vals []uint16) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, v := range vals {
		d.regs[start+uint16(i)] = v
	}
}

// Serve processes Modbus RTU frames from port until it is closed.
// Reading the master end of a PTY fails with EIO whenever the last
// client hangs up; that is normal, so Serve waits for the next client
// instead of giving up.
func (d *Device) Serve(port io.ReadWriteCloser) error {
	buf := make([]byte, 512)
	var pending []byte
	for {
		n, err := port.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for len(pending) >= 4 {
				resp, consumed := d.handleFrame(pending)
				if consumed == 0 {
					// Incomplete frame: wait for more bytes.
					break
				}
				pending = pending[consumed:]
				if resp != nil {
					if _, werr := port.Write(resp); werr != nil {
						log.Printf("devsim: write failed (%v); waiting for new client", werr)
						pending = pending[:0]
						break
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
				time.Sleep(200 * time.Millisecond) // client hung up; keep serving
				continue
			}
			return err
		}
	}
}

// handleFrame parses one RTU frame from the front of buf.
// It returns the response (nil for non-matching slave IDs and malformed
// frames) and the number of bytes consumed (0 when more bytes needed).
func (d *Device) handleFrame(buf []byte) (resp []byte, consumed int) {
	if len(buf) < 4 {
		return nil, 0
	}
	addr := buf[0]
	fc := buf[1]

	var need int
	switch fc {
	case 3, 6:
		need = 8
	case 16:
		if len(buf) < 7 {
			return nil, 0
		}
		need = 9 + int(buf[6])
	default:
		need = 4 // garbage: drop the header and resync
	}
	if len(buf) < need {
		return nil, 0
	}
	frame := buf[:need]
	consumed = need

	if !crcOK(frame) {
		log.Printf("devsim: bad CRC, dropping frame % X", frame)
		return nil, consumed
	}
	if addr != d.SlaveID {
		return nil, consumed // other devices stay silent
	}

	switch fc {
	case 3:
		start := binary.BigEndian.Uint16(frame[2:4])
		count := binary.BigEndian.Uint16(frame[4:6])
		if count == 0 || count > 125 {
			return exception(addr, fc, 3), consumed
		}
		if d.denied(start, count) {
			return nil, consumed // dead area: silence, like real hardware
		}
		data := d.read(start, count)
		resp = append([]byte{addr, 3, byte(len(data))}, data...)
		return append(resp, crcBytes(resp)...), consumed
	case 6:
		raddr := binary.BigEndian.Uint16(frame[2:4])
		val := binary.BigEndian.Uint16(frame[4:6])
		d.writeSingle(raddr, val)
		return append([]byte{}, frame...), consumed // echo
	case 16:
		start := binary.BigEndian.Uint16(frame[2:4])
		qty := binary.BigEndian.Uint16(frame[4:6])
		if int(frame[6]) != 2*int(qty) {
			return exception(addr, fc, 3), consumed
		}
		vals := make([]uint16, qty)
		for i := range vals {
			vals[i] = binary.BigEndian.Uint16(frame[7+i*2:])
		}
		d.writeMulti(start, vals)
		resp = []byte{addr, 16, frame[2], frame[3], frame[4], frame[5]}
		return append(resp, crcBytes(resp)...), consumed
	}
	return exception(addr, fc, 1), consumed
}

func exception(addr, fc, code byte) []byte {
	resp := []byte{addr, fc | 0x80, code}
	return append(resp, crcBytes(resp)...)
}

// crc16 computes the Modbus RTU CRC (reflected poly 0xA001).
func crc16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

func crcBytes(data []byte) []byte {
	c := crc16(data)
	return []byte{byte(c), byte(c >> 8)} // low byte first per spec
}

func crcOK(frame []byte) bool {
	if len(frame) < 2 {
		return false
	}
	want := crc16(frame[:len(frame)-2])
	got := uint16(frame[len(frame)-2]) | uint16(frame[len(frame)-1])<<8
	return want == got
}

package devsim

import (
	"os"
	"testing"
	"time"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// openTestPTY skips on systems without /dev/ptmx.
func openTestPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	master, slavePath, err := OpenPTY()
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	return master, slavePath
}

// TestControllerAgainstSimulator wires the real Controller to the virtual
// device over a PTY and runs the same operations the GUI performs:
// block reads, single write + readback, and a multi-register write.
func TestControllerAgainstSimulator(t *testing.T) {
	master, slavePath := openTestPTY(t)
	defer master.Close()

	dev := New(1)
	served := make(chan error, 1)
	go func() { served <- dev.Serve(master) }()

	c := controller.NewController(controller.Connection{
		Port:    slavePath,
		Device:  1,
		Baud:    9600, // irrelevant on a PTY
		Timeout: 2 * time.Second,
	})
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	// ── Dashboard block read ──
	dashRegs := []*controller.Register{}
	for _, addr := range []uint16{0x0100, 0x0101, 0x0102, 0x010B, 0x010C, 0x0204, 0x0210, 0x0223} {
		dashRegs = append(dashRegs, controller.FindRegisterByAddr(addr))
	}
	values, errs := c.ReadPlanned(dashRegs)
	if len(errs) != 0 {
		t.Fatalf("dashboard read errors: %v", errs)
	}
	if got := controller.DecodeValue(controller.FindRegisterByAddr(0x0100), values[0x0100]); got != "76 %" {
		t.Errorf("SOC decode = %q, want %q", got, "76 %")
	}
	if got := controller.DecodeValue(controller.FindRegisterByAddr(0x0101), values[0x0101]); got != "53.6 V" {
		t.Errorf("battery voltage decode = %q", got)
	}
	if got := controller.DecodeValue(controller.FindRegisterByAddr(0x010B), values[0x010B]); got != "MPPT · load on" {
		t.Errorf("charge stage decode = %q", got)
	}

	// ── Info block read (ASCII model string, versions) ──
	infoRegs := []*controller.Register{}
	for _, addr := range []uint16{0x000A, 0x000C, 0x0014, 0x0021, 0x0035} {
		infoRegs = append(infoRegs, controller.FindRegisterByAddr(addr))
	}
	info, errs := c.ReadPlanned(infoRegs)
	if len(errs) != 0 {
		t.Fatalf("info read errors: %v", errs)
	}
	if got := controller.DecodeValue(controller.FindRegisterByAddr(0x000C), info[0x000C]); got != "HF4850S8" {
		t.Errorf("product model = %q", got)
	}
	if got := controller.DecodeValue(controller.FindRegisterByAddr(0x0035), info[0x0035]); got != "SN48508020240001" {
		t.Errorf("SN string = %q", got)
	}

	// ── Settings: read, write, readback ──
	e006 := controller.FindRegister("Limited Charge Voltage")
	if e006 == nil {
		t.Fatal("register E006 not found")
	}
	before, err := c.ReadRegister(e006)
	if err != nil {
		t.Fatalf("read E006: %v", err)
	}
	if got := controller.DecodeValue(e006, before); got != "14.4 V" {
		t.Errorf("E006 default = %q, want 14.4 V", got)
	}
	if err := c.WriteRegister(0xE006, 141); err != nil { // 14.1 V
		t.Fatalf("write E006: %v", err)
	}
	after, err := c.ReadRegister(e006)
	if err != nil {
		t.Fatalf("readback E006: %v", err)
	}
	if after[0] != 141 {
		t.Errorf("E006 readback = %d, want 141", after[0])
	}

	// ── Multi-register write: device clock ──
	if err := c.WriteRegisters(0x020C, controller.EncodeTime(26, 9, 13, 14, 30, 0)); err != nil {
		t.Fatalf("WriteRegisters clock: %v", err)
	}
	clock, err := c.ReadRegister(controller.FindRegisterByAddr(0x020C))
	if err != nil {
		t.Fatalf("read clock: %v", err)
	}
	if got := controller.DecodeValue(controller.FindRegisterByAddr(0x020C), clock); got != "2026-09-13 14:30:00" {
		t.Errorf("clock = %q", got)
	}

	// ── Fault history ──
	f800, err := c.ReadChunk(0xF800, 16)
	if err != nil {
		t.Fatalf("read fault record: %v", err)
	}
	rec := controller.DecodeFaultRecord(0, f800)
	if !rec.Valid || rec.CodeName != "Battery under-voltage" || rec.Time != "09-13 14:35:45" {
		t.Errorf("fault record decode: %+v", rec)
	}
}

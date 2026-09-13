package main

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// TestBuildUI constructs the entire widget tree with Fyne's test driver
// and pushes fake snapshots through every panel's update path. It fails
// on panic (nil widgets, bad type assertions, layout misuse) and checks
// the visible values.
func TestBuildUI(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("test")

	g := newApp(a, w, LaunchOptions{})
	w.SetContent(g.buildRoot())

	if g.dash == nil || g.setting == nil || g.control == nil || g.stats == nil || g.faults == nil || g.info == nil {
		t.Fatal("panels not constructed")
	}
	if n := g.tabs.Items; len(n) != 6 {
		t.Fatalf("expected 6 tabs, got %d", len(n))
	}

	// Connection state transitions must not panic and must flip the button.
	g.setState(stateConnecting)
	if g.connBtn.Text != "Connect" || !g.connBtn.Disabled() {
		t.Errorf("connecting state: button = %q disabled=%v", g.connBtn.Text, g.connBtn.Disabled())
	}
	g.setState(stateConnected)
	if g.connBtn.Text != "Disconnect" {
		t.Errorf("connected state: button = %q", g.connBtn.Text)
	}
	g.setState(stateDisconnected)
	if g.connBtn.Disabled() {
		t.Error("disconnect must re-enable the button")
	}

	// Dashboard: apply a fake block-read snapshot.
	g.dash.apply(fakeSnapshot())
	if got := g.dash.socText.Segments[0].(*widget.TextSegment).Text; got != "76 %" {
		t.Errorf("dashboard SOC text = %q", got)
	}
	if got := g.dash.battRows[0x0101].Text; got != "53.6 V" {
		t.Errorf("dashboard battery voltage = %q", got)
	}
	if got := g.dash.acRows[0x0210].Text; got != "Inverter operation" {
		t.Errorf("machine state = %q", got)
	}
	if g.dash.faults.Visible() {
		t.Error("fault banner should be hidden with no faults")
	}

	// With a fault bit set (B16 battery over-discharge), the banner appears.
	snap := fakeSnapshot()
	snap[0x010C] = []uint16{0, 1}
	g.dash.apply(snap)
	if !g.dash.faults.Visible() {
		t.Error("fault banner should be visible with a fault bit set")
	}

	// Battery current is inverted for display: raw 0xFED4 (device -300,
	// i.e. 30 A charging) must display as positive 30.0 A.
	g.dash.apply(map[uint16][]uint16{0x0102: {0xFED4}})
	if got := g.dash.dcRows[0x0102].Text; got != "30.0 A" {
		t.Errorf("charging current = %q, want 30.0 A", got)
	}
	g.dash.apply(map[uint16][]uint16{0x0102: {85}})
	if got := g.dash.dcRows[0x0102].Text; got != "-8.5 A" {
		t.Errorf("discharging current = %q, want -8.5 A", got)
	}

	// 0xFFFF on an unsigned register is the device's "not available".
	g.dash.apply(map[uint16][]uint16{0x0108: {0xFFFF}})
	if got := g.dash.dcRows[0x0108].Text; got != "n/a" {
		t.Errorf("invalid PV current = %q, want n/a", got)
	}

	// Raw mode appends the Modbus words to each value.
	g.dash.showRaw.SetChecked(true)
	g.dash.apply(fakeSnapshot())
	if got := g.dash.dcRows[0x0102].Text; got != "-8.5 A  (0x0055)" {
		t.Errorf("raw mode text = %q", got)
	}
	g.dash.showRaw.SetChecked(false)

	// Settings: rows exist, filter works, editor selection renders.
	if len(g.setting.rows) == 0 {
		t.Fatal("settings rows empty")
	}
	g.setting.rebuildView()
	if len(g.setting.view) == 0 {
		t.Error("settings view should match all rows after unfiltered rebuild")
	}
	g.setting.filter.SetText("boost")
	g.setting.rebuildView()
	if len(g.setting.view) == 0 || len(g.setting.view) >= len(g.setting.rows) {
		t.Errorf("filter 'boost' matched %d of %d rows", len(g.setting.view), len(g.setting.rows))
	}
	g.setting.filter.SetText("")
	g.setting.rebuildView()
	g.setting.editIdx = g.setting.view[0]
	g.setting.showEditor()

	// Stats: applying a snapshot must populate rows and the 7-day table.
	g.stats.apply(fakeSnapshotStats())
	if got := g.stats.rows[0xF02F].Text; got != "4.0 kWh" {
		t.Errorf("today PV generation cell = %q", got)
	}
	if got := g.stats.cell(widget.TableCellID{Row: 1, Col: 1}); got == "—" || got == "" {
		t.Errorf("history cell should have data, got %q", got)
	}

	// Faults and log update paths.
	g.faults.renderHistory([]controller.FaultRecord{
		{Index: 0, Valid: true, Code: 1, CodeName: "Battery under-voltage", Time: "09-13 14:35:45"},
	})
	if g.faults.history.Text == "" {
		t.Error("fault history text not rendered")
	}
	g.control.appendLog("test line")
	if g.control.log.Text != "test line\n" {
		t.Errorf("log append = %q", g.control.log.Text)
	}
}

func fakeSnapshot() map[uint16][]uint16 {
	return map[uint16][]uint16{
		0x0100: {76},
		0x0101: {536},
		0x0102: {85},
		0x0103: {0x231E},
		0x010B: {0x8002},
		0x010C: {0, 0},
		0x0204: {0, 0, 0, 0},
		0x0210: {5},
		0x0212: {3600},
		0x0213: {2302},
		0x0215: {5000},
		0x0216: {2300},
		0x0218: {5001},
		0x0219: {87},
		0x021B: {2000},
		0x021C: {2020},
		0x021F: {40},
		0x0220: {412},
		0x0223: {320},
		0x0104: {0},
		0x0105: {0},
		0x0106: {0},
		0x0107: {782},
		0x0108: {63},
		0x0109: {492},
	}
}

func fakeSnapshotStats() map[uint16][]uint16 {
	values := map[uint16][]uint16{}
	for _, addr := range statisticsAddrs {
		reg := controller.FindRegisterByAddr(addr)
		if reg == nil {
			continue
		}
		w := make([]uint16, reg.Length)
		for i := range w {
			w[i] = uint16(40 - i) // plausible series/sample filler
		}
		values[addr] = w
	}
	return values
}

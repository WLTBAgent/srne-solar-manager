package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// statisticsAddrs covers the whole statistics area 0xF000–0xF04B; one
// merged block read fetches everything shown on this tab.
var statisticsAddrs = []uint16{
	0xF000, 0xF007, 0xF00E, 0xF015, 0xF01C, 0xF023, // 7-day series
	0xF02D, 0xF02E, 0xF02F, 0xF030, 0xF031, 0xF032, 0xF033,
	0xF034, 0xF036, 0xF038, 0xF03A, 0xF03C, 0xF03D, 0xF03E,
	0xF03F, 0xF040, 0xF043, 0xF046, 0xF048, 0xF04A, 0xF04B,
}

type statsPanel struct {
	app   *App
	rows  map[uint16]*widget.Label
	last  map[uint16][]uint16
	table *widget.Table
}

func newStats(g *App) *statsPanel {
	return &statsPanel{app: g, rows: map[uint16]*widget.Label{}, last: map[uint16][]uint16{}}
}

func (p *statsPanel) canvas() fyne.CanvasObject {
	today := formGrid(&p.rows,
		kv("Battery charge (today)", 0xF02D), kv("Battery discharge (today)", 0xF02E),
		kv("PV generation (today)", 0xF02F), kv("Load consumption (today)", 0xF030),
		kv("Mains charge (today)", 0xF03C), kv("Load from mains (today)", 0xF03D),
		kv("Inverter run time (today)", 0xF03E), kv("Bypass run time (today)", 0xF03F),
	)
	accumulated := formGrid(&p.rows,
		kv("Total running days", 0xF031), kv("Battery full charges", 0xF033),
		kv("Battery over-discharges", 0xF032), kv("Accumulated charge (Ah)", 0xF034),
		kv("Accumulated discharge (Ah)", 0xF036), kv("Accumulated PV generation", 0xF038),
		kv("Accumulated load consumption", 0xF03A), kv("Accumulated mains charge", 0xF046),
		kv("Accumulated load from battery", 0xF048),
		kv("Inverter working hours", 0xF04A), kv("Bypass working hours", 0xF04B),
		kv("Power-on time", 0xF040), kv("Last equalize done", 0xF043),
	)

	p.table = widget.NewTable(
		func() (int, int) { return 8, 7 },
		func() fyne.CanvasObject { return widget.NewLabel("00000.00") },
		func(i widget.TableCellID, obj fyne.CanvasObject) {
			lbl := obj.(*widget.Label)
			lbl.SetText(p.cell(i))
			if i.Row == 0 {
				lbl.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				lbl.TextStyle = fyne.TextStyle{Monospace: true}
			}
		},
	)
	p.table.ShowHeaderRow = true
	p.table.SetColumnWidth(0, 110)
	for c := 1; c < 7; c++ {
		p.table.SetColumnWidth(c, 100)
	}

	toolbar := container.NewHBox(
		widget.NewButton("Read statistics", p.readAsync),
		widget.NewLabel("7-day history: yesterday → 7 days ago"),
	)

	return container.NewPadded(container.NewVScroll(container.NewVBox(
		toolbar,
		widget.NewCard("Today", "", today),
		widget.NewCard("Accumulated", "", accumulated),
		widget.NewCard("Last 7 days", "", p.table),
		layout.NewSpacer(),
	)))
}

var historyCols = []struct {
	addr uint16
	name string
}{
	{0xF000, "PV (Ah)"},
	{0xF007, "Batt in (Ah)"},
	{0xF00E, "Batt out (Ah)"},
	{0xF015, "Mains (Ah)"},
	{0xF01C, "Load (kWh)"},
	{0xF023, "Mains load (kWh)"},
}

var dayNames = []string{"Day", "Yesterday", "2 days ago", "3 days ago", "4 days ago", "5 days ago", "6 days ago", "7 days ago"}

// cell renders one table cell. Main goroutine only.
func (p *statsPanel) cell(i widget.TableCellID) string {
	if i.Row == 0 {
		if i.Col == 0 {
			return "Day"
		}
		return historyCols[i.Col-1].name
	}
	if i.Col == 0 {
		return dayNames[i.Row]
	}
	col := historyCols[i.Col-1]
	words, ok := p.last[col.addr]
	if !ok || len(words) < i.Row {
		return "—"
	}
	reg := controller.FindRegisterByAddr(col.addr)
	if reg == nil {
		return "—"
	}
	return controller.DecodeValue(singleWord(reg), []uint16{words[i.Row-1]})
}

// singleWord copies a series register as a 1-word register so its scale
// and unit formatting apply to individual history samples.
func singleWord(reg *controller.Register) *controller.Register {
	c := *reg
	c.Length = 1
	return &c
}

// apply updates all values from a block-read snapshot. Main goroutine.
func (p *statsPanel) apply(values map[uint16][]uint16) {
	for addr, words := range values {
		p.last[addr] = words
		if lbl, ok := p.rows[addr]; ok {
			if reg := controller.FindRegisterByAddr(addr); reg != nil {
				lbl.SetText(controller.DecodeValue(reg, words))
			}
		}
	}
	p.table.Refresh()
}

func (p *statsPanel) readAsync() {
	g := p.app
	if g.getState() != stateConnected {
		dialog.ShowInformation("Not connected", "Connect to the controller first.", g.window)
		return
	}
	g.async(func() { p.load(g.getConn()) })
}

// load reads the statistics area and updates the tab. Blocks until
// done; safe to call from any goroutine.
func (p *statsPanel) load(conn *controller.Controller) {
	g := p.app
	if conn == nil {
		return
	}
	{
		values, errs := conn.ReadPlanned(regsByAddrs(statisticsAddrs))
		ui(func() {
			p.apply(values)
			if len(errs) > 0 {
				g.reportError("Statistics read failed: %v", errs[0])
			} else {
				g.setStatus("Statistics updated")
			}
		})
	}
}

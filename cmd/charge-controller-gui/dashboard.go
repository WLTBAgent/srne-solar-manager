package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// dashboardAddrs are the registers shown on the dashboard, read as
// merged block transactions by the monitor loop.
var dashboardAddrs = []uint16{
	0x0100, 0x0101, 0x0102, 0x0103, 0x0104, 0x0105, 0x0106,
	0x0107, 0x0108, 0x0109, 0x010B, 0x010C,
	0x0204,
	0x0210, 0x0212, 0x0213, 0x0215, 0x0216, 0x0217, 0x0218,
	0x0219, 0x021A, 0x021B, 0x021C, 0x021F, 0x0220, 0x0223,
}

func dashboardRegs() []*controller.Register {
	return regsByAddrs(dashboardAddrs)
}

type dashboardPanel struct {
	app *App

	socText  *widget.RichText
	socBar   *widget.ProgressBar
	battRows map[uint16]*widget.Label
	dcRows   map[uint16]*widget.Label
	acRows   map[uint16]*widget.Label
	faults   *widget.Label
	pauseBtn *widget.Button
	showRaw  *widget.Check
	last     map[uint16][]uint16
}

func newDashboard(g *App) *dashboardPanel {
	return &dashboardPanel{
		app:      g,
		battRows: map[uint16]*widget.Label{},
		dcRows:   map[uint16]*widget.Label{},
		acRows:   map[uint16]*widget.Label{},
		last:     map[uint16][]uint16{},
	}
}

// canvas builds the dashboard UI. Main goroutine, once at startup.
func (p *dashboardPanel) canvas() fyne.CanvasObject {
	p.socText = widget.NewRichText(&widget.TextSegment{
		Text:  "—",
		Style: widget.RichTextStyle{Alignment: fyne.TextAlignCenter, SizeName: theme.SizeNameHeadingText, TextStyle: fyne.TextStyle{Bold: true}},
	})
	p.socBar = widget.NewProgressBar()

	p.faults = widget.NewLabel("")
	p.faults.Wrapping = fyne.TextWrapWord
	p.faults.Importance = widget.DangerImportance
	p.faults.Hide()

	battContent := container.NewVBox(
		p.socText,
		p.socBar,
		formGrid(&p.battRows,
			kv("Battery", 0x0101), kv("Device temp", 0x0103),
			kv("Charge stage", 0x010B),
		),
	)
	dcContent := formGrid(&p.dcRows,
		kv("PV voltage", 0x0107), kv("PV current", 0x0108),
		kv("PV charge power", 0x0109), kv("Charge current", 0x0102),
		kv("Load voltage", 0x0104), kv("Load current", 0x0105),
		kv("Load power", 0x0106),
	)
	acContent := formGrid(&p.acRows,
		kv("Machine state", 0x0210), kv("Bus voltage", 0x0212),
		kv("Grid voltage", 0x0213), kv("Grid frequency", 0x0215),
		kv("Output voltage", 0x0216), kv("Output frequency", 0x0218),
		kv("Load current", 0x0219), kv("Load active power", 0x021B),
		kv("Load apparent power", 0x021C), kv("Load ratio", 0x021F),
		kv("Heatsink temp", 0x0220), kv("Ambient temp", 0x0223),
	)

	battCard := widget.NewCard("Battery", "", battContent)
	dcCard := widget.NewCard("Solar / DC controller", "", dcContent)
	acCard := widget.NewCard("AC inverter", "", acContent)

	left := container.NewVBox(battCard, layout.NewSpacer())
	right := container.NewVBox(dcCard, acCard)

	split := container.NewHSplit(left, right)
	split.SetOffset(0.32)

	p.pauseBtn = widget.NewButton("Pause updates", p.togglePause)
	p.showRaw = widget.NewCheck("Show raw values", func(bool) { p.apply(nil) })

	return container.NewPadded(container.NewVScroll(container.NewVBox(
		p.faults,
		split,
		container.NewHBox(p.pauseBtn, p.showRaw),
	)))
}

func (p *dashboardPanel) togglePause() {
	if p.app.getState() != stateConnected {
		return
	}
	paused := !p.app.paused.Load()
	p.app.paused.Store(paused)
	if paused {
		p.pauseBtn.SetText("Resume updates")
	} else {
		p.pauseBtn.SetText("Pause updates")
	}
}

// formRow is one label/value pair for formGrid.
type formRow struct {
	title string
	addr  uint16
}

func kv(title string, addr uint16) formRow { return formRow{title, addr} }

// formGrid lays out label/value pairs using FormLayout (label leading,
// value trailing) and records the value labels for live updates.
func formGrid(into *map[uint16]*widget.Label, rows ...formRow) fyne.CanvasObject {
	var cells []fyne.CanvasObject
	for _, row := range rows {
		name := widget.NewLabel(row.title)
		val := widget.NewLabel("—")
		val.TextStyle = fyne.TextStyle{Monospace: true}
		val.Alignment = fyne.TextAlignTrailing

		(*into)[row.addr] = val
		cells = append(cells, name, val)
	}
	return container.New(layout.NewFormLayout(), cells...)
}

// apply merges a block-read snapshot into the panel and re-renders all
// values. Passing nil just re-renders from the cached values (used when
// toggling the raw display). Main goroutine only.
func (p *dashboardPanel) apply(values map[uint16][]uint16) {
	for addr, words := range values {
		p.last[addr] = words
	}

	decode := func(addr uint16) (string, bool) {
		reg := controller.FindRegisterByAddr(addr)
		if reg == nil {
			return "", false
		}
		words, ok := p.last[addr]
		if !ok {
			return "", false
		}
		text := controller.DecodeValue(reg, words)
		if p.showRaw != nil && p.showRaw.Checked {
			hex := make([]string, len(words))
			for i, w := range words {
				hex[i] = fmt.Sprintf("0x%04X", w)
			}
			text += "  (" + strings.Join(hex, " ") + ")"
		}
		return text, true
	}
	set := func(rows map[uint16]*widget.Label, addr uint16) {
		if lbl, ok := rows[addr]; ok {
			if s, ok := decode(addr); ok {
				lbl.SetText(s)
			}
		}
	}

	if soc, ok := decode(0x0100); ok {
		p.socText.Segments[0].(*widget.TextSegment).Text = soc
		p.socText.Refresh()
		// Ignore impossible SOC readings (0xFFFF "n/a" marker).
		if socPct := controller.RawToFloat(controller.FindRegisterByAddr(0x0100), p.last[0x0100]); socPct >= 0 && socPct <= 100 {
			p.socBar.SetValue(socPct / 100.0)
		}
	}
	for addr := range p.battRows {
		set(p.battRows, addr)
	}
	for addr := range p.dcRows {
		set(p.dcRows, addr)
	}
	for addr := range p.acRows {
		set(p.acRows, addr)
	}

	// Fault banner: controller fault bits + inverter fault codes.
	var faults []string
	if reg := controller.FindRegisterByAddr(0x010C); reg != nil {
		if words, ok := p.last[0x010C]; ok {
			faults = append(faults, controller.DecodeControllerFaults(words)...)
		}
	}
	if words, ok := p.last[0x0204]; ok {
		for _, code := range words {
			if code != 0 {
				faults = append(faults, controller.DecodeFaultCode(code))
			}
		}
	}
	if len(faults) > 0 {
		p.faults.SetText("⚠ " + joinFaults(faults))
		p.faults.Show()
	} else {
		p.faults.Hide()
	}
}

func (p *dashboardPanel) markDisconnected() {
	p.pauseBtn.SetText("Pause updates")
	p.app.paused.Store(false)
}

func joinFaults(list []string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += " · "
		}
		out += s
	}
	return out
}

package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// fault history: 16 records × 16 words starting at 0xF800.
const (
	faultHistoryBase = 0xF800
	faultRecordLen   = 16
	faultRecordCount = 16
)

type faultsPanel struct {
	app      *App
	current  *widget.Label
	history  *widget.Label
	readBtn  *widget.Button
	lastRecs []controller.FaultRecord
}

func newFaults(g *App) *faultsPanel {
	return &faultsPanel{app: g}
}

func (p *faultsPanel) canvas() fyne.CanvasObject {
	p.current = widget.NewLabel("Not read")
	p.current.Wrapping = fyne.TextWrapWord

	p.history = widget.NewLabel("Not read")
	p.history.Wrapping = fyne.TextWrapWord

	p.readBtn = widget.NewButton("Read faults", p.readAsync)

	currentCard := widget.NewCard("Current faults", "", p.current)
	historyCard := widget.NewCard("Fault history", "", p.history)

	return container.NewPadded(container.NewVScroll(container.NewVBox(
		container.NewHBox(p.readBtn),
		currentCard,
		historyCard,
	)))
}

func (p *faultsPanel) readAsync() {
	g := p.app
	if g.getState() != stateConnected {
		dialog.ShowInformation("Not connected", "Connect to the controller first.", g.window)
		return
	}
	g.async(func() { p.load(g.getConn()) })
}

// load reads current faults plus the history area and updates the tab.
// Blocks until done; safe to call from any goroutine.
func (p *faultsPanel) load(conn *controller.Controller) {
	g := p.app
	if conn == nil {
		return
	}
	{
		// Current controller fault bits + inverter fault codes.
		cur, curErrs := conn.ReadPlanned(regsByAddrs([]uint16{0x010C, 0x0204}))

		// History: ReadPlanned splits the 256-word area into legal blocks.
		histRegs := make([]*controller.Register, faultRecordCount)
		for i := range histRegs {
			histRegs[i] = &controller.Register{
				Address: faultHistoryBase + uint16(i)*faultRecordLen,
				Length:  faultRecordLen,
				Name:    fmt.Sprintf("Fault record %d", i),
			}
		}
		hist, histErrs := conn.ReadPlanned(histRegs)

		recs := make([]controller.FaultRecord, faultRecordCount)
		for i := range histRegs {
			recs[i] = controller.DecodeFaultRecord(i, hist[histRegs[i].Address])
		}

		ui(func() {
			p.current.SetText(p.currentText(cur))
			p.renderHistory(recs)
			switch {
			case len(curErrs) > 0 && len(histErrs) > 0:
				g.reportError("Faults: read failed (%v)", curErrs[0])
			case len(curErrs) > 0:
				g.reportError("Faults: current-fault read failed (%v); history loaded", curErrs[0])
			case len(histErrs) > 0:
				g.reportError("Faults: history read incomplete (%v)", histErrs[0])
			default:
				g.setStatus("Fault information updated")
			}
		})
	}
}

func (p *faultsPanel) currentText(values map[uint16][]uint16) string {
	text := "No active faults"
	if words, ok := values[0x010C]; ok {
		if faults := controller.DecodeControllerFaults(words); len(faults) > 0 {
			text = "Controller: " + fmt.Sprint(faults)
		}
	}
	if words, ok := values[0x0204]; ok {
		for _, code := range words {
			if code != 0 {
				text += "\nInverter: " + controller.DecodeFaultCode(code)
			}
		}
	}
	return text
}

func (p *faultsPanel) renderHistory(recs []controller.FaultRecord) {
	p.lastRecs = recs
	valid := 0
	text := ""
	for _, rec := range recs {
		if !rec.Valid {
			continue
		}
		valid++
		line := fmt.Sprintf("#%d  %s", rec.Index, rec.CodeName)
		if rec.Time != "" {
			line += "  @ " + rec.Time
		}
		line += fmt.Sprintf("  (code 0x%02X, %d data words)", rec.Code, len(rec.Data))
		text += line + "\n"
	}
	if valid == 0 {
		p.history.SetText("No fault records stored.")
	} else {
		p.history.SetText(text)
	}
}

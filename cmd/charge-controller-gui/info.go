package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

// infoAddrs spans the whole product info area 0x000A–0x0048; one merged
// block read fetches everything shown on this tab.
var infoAddrs = []uint16{
	0x000A, 0x000B, 0x000C, 0x0014, 0x0016, 0x0018, 0x001A,
	0x001B, 0x001C, 0x001E, 0x0020, 0x0021, 0x0035,
}

type infoPanel struct {
	app  *App
	rows map[uint16]*widget.Label
}

func newInfo(g *App) *infoPanel {
	return &infoPanel{app: g, rows: map[uint16]*widget.Label{}}
}

func (p *infoPanel) canvas() fyne.CanvasObject {
	var rows []formRow
	for _, r := range controller.AllRegisters {
		if r.Category == "Product Info" {
			rows = append(rows, kv(r.Name, r.Address))
		}
	}
	grid := formGrid(&p.rows, rows...)

	return container.NewPadded(container.NewVScroll(container.NewVBox(
		container.NewHBox(widget.NewButton("Read device info", p.readAsync)),
		widget.NewCard("Product information", "", grid),
	)))
}

// apply updates all fields from a block-read snapshot. Main goroutine.
func (p *infoPanel) apply(values map[uint16][]uint16) {
	for addr, lbl := range p.rows {
		reg := controller.FindRegisterByAddr(addr)
		words, ok := values[addr]
		if reg == nil || !ok {
			lbl.SetText("—")
			continue
		}
		lbl.SetText(controller.DecodeValue(reg, words))
	}
}

func (p *infoPanel) readAsync() {
	g := p.app
	if g.getState() != stateConnected {
		dialog.ShowInformation("Not connected", "Connect to the controller first.", g.window)
		return
	}
	g.async(func() { p.load(g.getConn()) })
}

// load reads the product info area and updates the tab. Blocks until
// done; safe to call from any goroutine.
func (p *infoPanel) load(conn *controller.Controller) {
	g := p.app
	if conn == nil {
		return
	}
	{
		values, errs := conn.ReadPlanned(regsByAddrs(infoAddrs))
		ui(func() {
			p.apply(values)
			if len(errs) > 0 {
				g.reportError("Device info read failed: %v", errs[0])
			} else {
				g.setStatus("Device info updated")
			}
		})
	}
}

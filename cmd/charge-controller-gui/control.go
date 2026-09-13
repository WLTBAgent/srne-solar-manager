package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

type controlPanel struct {
	app *App
	log *widget.Entry
}

func newControl(g *App) *controlPanel {
	return &controlPanel{app: g}
}

// canvas builds the Control tab: grouped device commands plus a
// read-only activity log. Main goroutine, once at startup.
func (p *controlPanel) canvas() fyne.CanvasObject {
	powerCard := widget.NewCard("Power", "", p.grid(
		p.action("Power on", "power-on", widget.MediumImportance, false),
		p.action("Power off", "power-off", widget.DangerImportance, true),
		p.action("Reboot controller", "reset", widget.DangerImportance, true),
		p.action("Restore factory defaults", "restore-defaults", widget.DangerImportance, true),
	))
	alarmsCard := widget.NewCard("Alarms & data", "", p.grid(
		p.action("Clear alarm", "clear-alarm", widget.MediumImportance, false),
		p.action("Clear statistics", "clear-stats", widget.DangerImportance, true),
		p.action("Clear history", "clear-history", widget.DangerImportance, true),
	))
	chargingCard := widget.NewCard("Charging", "", p.grid(
		p.action("Equalizing charge on", "equalize-on", widget.MediumImportance, false),
		p.action("Equalizing charge off", "equalize-off", widget.MediumImportance, false),
		p.action("Sync device clock", "", widget.MediumImportance, false), // handled specially
	))
	outputCard := widget.NewCard("DC output", "", p.grid(
		p.action("DC load on", "dc-load-on", widget.MediumImportance, false),
		p.action("DC load off", "dc-load-off", widget.MediumImportance, false),
	))

	p.log = widget.NewMultiLineEntry()
	p.log.SetPlaceHolder("Activity log…")
	p.log.SetMinRowsVisible(10)
	p.log.Disable() // read-only; still programmatically updated

	logCard := widget.NewCard("Activity log", "", container.NewVBox(
		p.log,
		widget.NewButton("Clear log", func() { p.log.SetText("") }),
	))

	cards := container.NewVScroll(container.NewVBox(powerCard, alarmsCard, chargingCard, outputCard))

	// Cards take the Border center (all remaining space); the log is
	// docked at the bottom with its natural height.
	return container.NewBorder(nil, logCard, nil, nil, cards)
}

func (p *controlPanel) grid(buttons ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewGridLayout(2), buttons...)
}

// action builds one command button. Empty actionName marks the clock-sync
// button, which needs a multi-register write rather than a ControlAction.
func (p *controlPanel) action(label, actionName string, importance widget.Importance, confirm bool) fyne.CanvasObject {
	btn := widget.NewButton(label, func() {
		if actionName == "" {
			p.syncClock()
			return
		}
		p.exec(actionName, confirm)
	})
	btn.Importance = importance
	return btn
}

func (p *controlPanel) requireConn() *controller.Controller {
	conn := p.app.getConn()
	if conn == nil {
		dialog.ShowInformation("Not connected", "Connect to the controller first.", p.app.window)
	}
	return conn
}

// exec confirms (optionally) and runs a write-only control action in the
// background.
func (p *controlPanel) exec(actionName string, confirm bool) {
	g := p.app
	conn := p.requireConn()
	if conn == nil {
		return
	}
	act, ok := controller.ControlActions[actionName]
	if !ok {
		return
	}

	run := func() {
		g.async(func() {
			c := g.getConn()
			if c == nil {
				return
			}
			err := c.WriteRegister(act.Addr, act.Val)
			ui(func() {
				if err != nil {
					g.log("✗ %s failed: %v", act.Desc, err)
					g.reportError("%s failed: %v", act.Desc, err)
					return
				}
				g.log("✓ %s (0x%04X ← 0x%04X)", act.Desc, act.Addr, act.Val)
				g.setStatus("%s: done", act.Desc)
			})
		})
	}

	if confirm {
		dialog.NewConfirm("Confirm action",
			fmt.Sprintf("Execute: %s?\nThis cannot be undone from this tool.", act.Desc),
			func(ok bool) {
				if ok {
					run()
				}
			}, g.window).Show()
		return
	}
	run()
}

// syncClock writes the host time to the device clock registers (0x020C).
func (p *controlPanel) syncClock() {
	g := p.app
	if p.requireConn() == nil {
		return
	}
	now := time.Now()
	vals := controller.EncodeTime(now.Year()%100, int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())

	g.async(func() {
		c := g.getConn()
		if c == nil {
			return
		}
		err := c.WriteRegisters(0x020C, vals)
		ui(func() {
			if err != nil {
				g.log("✗ Clock sync failed: %v", err)
				g.reportError("Clock sync failed: %v", err)
				return
			}
			g.log("✓ Device clock set to %s", now.Format("2006-01-02 15:04:05"))
			g.setStatus("Device clock synced")
		})
	})
}

// appendLog adds a line to the read-only activity log (thread-safe).
func (p *controlPanel) appendLog(line string) {
	ui(func() {
		text := p.log.Text
		if len(text) > 8000 {
			text = text[len(text)-8000:]
			if i := strings.Index(text, "\n"); i >= 0 {
				text = text[i+1:]
			}
		}
		p.log.SetText(text + line + "\n")
		p.log.CursorRow = len(p.log.Text)
	})
}

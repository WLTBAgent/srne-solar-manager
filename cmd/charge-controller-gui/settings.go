package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/wltechblog/charge-controller/internal/controller"
)

type settingRow struct {
	reg      *controller.Register
	value    string // last read value ("—" when unknown, "write-only" for W regs)
	note     string // last event (e.g. "written 14:22:01")
	category string
}

type settingsPanel struct {
	app *App

	rows     []settingRow
	view     []int // indices into rows currently shown
	filter   *widget.Entry
	catSel   *widget.Select
	list     *widget.List
	readVals *widget.Button

	// editor
	editIdx    int
	editTitle  *widget.Label
	editEntry  *widget.Entry
	editSlider *widget.Slider
	editSelect *widget.Select
	writeBtn   *widget.Button
	writeBusy  bool
}

func newSettings(g *App) *settingsPanel {
	p := &settingsPanel{app: g, editIdx: -1}

	regs := controller.ConfigurableRegisters()
	for i := range regs {
		r := regs[i]
		if r.Category == "Device Control" {
			// Write-only command registers belong on the Control tab;
			// exposing them here invites raw single-word misuse.
			continue
		}
		if r.Category == "Fault History" {
			// A rolling data log, not a parameter; shown on the Faults tab.
			continue
		}
		if r.Address == 0x020C {
			// Packed 3-register clock: writing one word here would
			// corrupt it; use Control → "Sync device clock".
			continue
		}
		value := "—"
		if r.Access == "W" {
			value = "write-only"
		}
		p.rows = append(p.rows, settingRow{reg: &regs[i], value: value, category: r.Category})
	}
	return p
}

func (p *settingsPanel) canvas() fyne.CanvasObject {
	p.filter = widget.NewEntry()
	p.filter.SetPlaceHolder("Filter by name / address (e.g. E006 or \"boost\")")
	p.filter.OnChanged = func(string) { p.rebuildView() }

	cats := []string{"All categories"}
	seen := map[string]bool{}
	for i := range p.rows {
		if !seen[p.rows[i].category] {
			seen[p.rows[i].category] = true
			cats = append(cats, p.rows[i].category)
		}
	}
	sort.Strings(cats[1:])
	// Selection is set before attaching OnChanged: SetSelected invokes
	// the callback immediately and the list does not exist yet.
	p.catSel = widget.NewSelect(cats, nil)
	p.catSel.SetSelected("All categories")
	p.catSel.OnChanged = func(string) { p.rebuildView() }

	p.readVals = widget.NewButton("Read values", p.readValuesAsync)

	toolbar := container.NewBorder(nil, nil, nil,
		container.NewHBox(p.readVals),
		p.filter,
	)

	// ── list ──
	p.list = widget.NewList(
		func() int { return len(p.view) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("Setting")
			value := widget.NewLabel("—")
			value.TextStyle = fyne.TextStyle{Monospace: true}
			value.Alignment = fyne.TextAlignTrailing
			// Border stores [center, right]: Objects[0]=name, Objects[1]=value.
			return container.NewBorder(nil, nil, nil, value, name)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			border := obj.(*fyne.Container)
			name := border.Objects[0].(*widget.Label)
			value := border.Objects[1].(*widget.Label)
			row := p.rows[p.view[i]]
			name.SetText(fmt.Sprintf("0x%04X  %s", row.reg.Address, row.reg.Name))
			value.SetText(row.value)
		},
	)
	p.list.OnSelected = func(id widget.ListItemID) {
		p.editIdx = p.view[id]
		p.showEditor()
	}

	// ── editor ──
	p.editTitle = widget.NewLabel("Select a setting")
	p.editTitle.Wrapping = fyne.TextWrapWord
	p.editTitle.TextStyle = fyne.TextStyle{Bold: true}

	p.editEntry = widget.NewEntry()
	p.editEntry.SetPlaceHolder("Value")

	p.editSlider = widget.NewSlider(0, 100)
	p.editSlider.Hide()
	p.editSlider.OnChanged = func(v float64) {
		reg := p.currentReg()
		if reg == nil {
			return
		}
		p.editEntry.SetText(formatEditorValue(v, reg))
	}

	p.editSelect = widget.NewSelect([]string{}, nil)
	p.editSelect.Hide()

	p.writeBtn = widget.NewButton("Write", p.writeSetting)

	editorCard := widget.NewCard("Edit setting", "", container.NewVBox(
		p.editTitle,
		p.editEntry,
		p.editSlider,
		p.editSelect,
		container.NewHBox(p.writeBtn),
	))

	split := container.NewHSplit(
		container.NewBorder(toolbar, nil, nil, nil, p.list),
		container.NewVScroll(editorCard),
	)
	split.SetOffset(0.55)
	p.rebuildView() // populate the list before the first paint
	return container.NewPadded(split)
}

func (p *settingsPanel) currentReg() *controller.Register {
	if p.editIdx < 0 || p.editIdx >= len(p.rows) {
		return nil
	}
	return p.rows[p.editIdx].reg
}

// rebuildView reapplies the filter and category selection. Main goroutine.
func (p *settingsPanel) rebuildView() {
	if p.list == nil {
		return
	}
	q := strings.ToLower(strings.TrimSpace(p.filter.Text))
	cat := p.catSel.Selected
	p.view = p.view[:0]
	for i := range p.rows {
		row := &p.rows[i]
		if cat != "All categories" && row.category != cat {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(row.reg.Name), q) &&
			!strings.Contains(strings.ToLower(fmt.Sprintf("0x%04X", row.reg.Address)), q) {
			continue
		}
		p.view = append(p.view, i)
	}
	p.list.Refresh()
}

// showEditor populates the editor for the selected row. Main goroutine.
func (p *settingsPanel) showEditor() {
	reg := p.currentReg()
	if reg == nil {
		return
	}
	row := p.rows[p.editIdx]

	desc := fmt.Sprintf("%s\nAddress 0x%04X · access %s", reg.Name, reg.Address, reg.Access)
	if reg.Unit != "" {
		desc += fmt.Sprintf(" · unit %s", reg.Unit)
	}
	if reg.Min != 0 || reg.Max != 0 {
		desc += fmt.Sprintf("\nRange %.1f – %.1f", reg.Min, reg.Max)
		if reg.Default != 0 {
			desc += fmt.Sprintf(" · default %.1f", reg.Default)
		}
	}
	if reg.Description != "" {
		desc += "\n" + reg.Description
	}
	if row.value == "write-only" {
		desc += "\nWrite-only: the device will not report this value back."
	}
	if row.note != "" {
		desc += "\n" + row.note
	}
	p.editTitle.SetText(desc)

	switch {
	case len(reg.Enum) > 0:
		p.editSlider.Hide()
		p.editEntry.Hide()
		options := enumOptions(reg)
		p.editSelect.SetOptions(options)
		if len(options) > 0 {
			p.editSelect.SetSelectedIndex(0)
		}
		p.editSelect.Show()
	case reg.Min != 0 || reg.Max != 0:
		p.editSelect.Hide()
		p.editSlider.Min = reg.Min
		p.editSlider.Max = reg.Max
		p.editSlider.Step = sliderStep(reg)
		p.editSlider.Value = editorDefault(reg)
		p.editEntry.SetText(formatEditorValue(p.editSlider.Value, reg))
		p.editSlider.Show()
		p.editEntry.Show()
	default:
		p.editSlider.Hide()
		p.editSelect.Hide()
		p.editEntry.SetText("")
		p.editEntry.SetPlaceHolder("Raw register value (decimal or 0x hex)")
		p.editEntry.Show()
	}
}

func enumOptions(reg *controller.Register) []string {
	var out []string
	for _, k := range controller.EnumSortedKeys(reg.Enum) {
		out = append(out, fmt.Sprintf("%d = %s", k, reg.Enum[k]))
	}
	return out
}

func sliderStep(reg *controller.Register) float64 {
	switch reg.Scale {
	case 0.1:
		return 0.1
	case 0.01:
		return 0.01
	default:
		return 1
	}
}

func editorDefault(reg *controller.Register) float64 {
	if reg.Default >= reg.Min && reg.Default <= reg.Max && reg.Default != 0 {
		return reg.Default
	}
	return reg.Min
}

func formatEditorValue(v float64, reg *controller.Register) string {
	switch reg.Scale {
	case 0.1:
		return fmt.Sprintf("%.1f", v)
	case 0.01:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// refreshValuesAsync block-reads all readable (RW) settings in the
// background. Safe to call from any goroutine.
func (p *settingsPanel) refreshValuesAsync() {
	g := p.app
	g.async(func() { p.loadValues(g.getConn()) })
}

// loadValues reads the RW setting rows and refreshes the list. Blocks
// until done; safe to call from any goroutine.
func (p *settingsPanel) loadValues(conn *controller.Controller) {
	g := p.app
	if conn == nil {
		return
	}

	// Read exactly what the list displays (RW rows only).
	var regs []*controller.Register
	for i := range p.rows {
		if p.rows[i].reg.Access == "RW" {
			regs = append(regs, p.rows[i].reg)
		}
	}
	values, errs := conn.ReadPlanned(regs)

	ui(func() {
		for i := range p.rows {
			row := &p.rows[i]
			if words, ok := values[row.reg.Address]; ok {
				row.value = controller.DecodeValue(row.reg, words)
				row.note = "read " + time.Now().Format("15:04:05")
			} else if row.reg.Access == "W" {
				row.value = "write-only"
			} else if len(errs) > 0 {
				row.value = "read error"
			}
		}
		p.list.Refresh()
		if p.editIdx >= 0 {
			p.showEditor() // refresh description with new read note
		}
		if len(errs) > 0 {
			msgs := []string{fmt.Sprintf("Settings: %d/%d registers read", len(values), len(regs))}
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			g.reportErrors(msgs)
		} else {
			g.setStatus("Settings loaded (%d registers)", len(values))
		}
	})
}

// readValuesAsync guards the manual "Read values" button.
func (p *settingsPanel) readValuesAsync() {
	if p.app.getState() != stateConnected {
		dialog.ShowInformation("Not connected", "Connect to the controller first.", p.app.window)
		return
	}
	p.refreshValuesAsync()
}

// writeSetting validates and writes the edited value. Main goroutine
// gathers input; the bus work happens in a goroutine.
func (p *settingsPanel) writeSetting() {
	g := p.app
	if g.getState() != stateConnected {
		dialog.ShowInformation("Not connected", "Connect to the controller first.", g.window)
		return
	}
	if p.writeBusy {
		return
	}
	reg := p.currentReg()
	if reg == nil {
		dialog.ShowInformation("No selection", "Select a setting on the left first.", g.window)
		return
	}

	var raw uint16
	var err error
	switch {
	case len(reg.Enum) > 0:
		key := strings.SplitN(p.editSelect.Selected, " = ", 2)[0]
		v, parseErr := parseEnumKey(key)
		if parseErr != nil {
			dialog.ShowError(fmt.Errorf("pick a value from the dropdown"), g.window)
			return
		}
		raw = v
	default:
		raw, err = controller.ParseValue(p.editEntry.Text, reg)
		if err != nil {
			dialog.ShowError(fmt.Errorf("invalid value: %w", err), g.window)
			return
		}
	}
	if err := controller.ValidateValue(reg, raw); err != nil {
		dialog.ShowError(err, g.window)
		return
	}

	shown := controller.DecodeValue(reg, []uint16{raw})
	dialog.NewConfirm("Confirm write",
		fmt.Sprintf("Write %s to\n%s (0x%04X)?", shown, reg.Name, reg.Address),
		func(ok bool) {
			if !ok {
				return
			}
			p.doWrite(raw, shown)
		}, g.window).Show()
}

func parseEnumKey(key string) (uint16, error) {
	var v uint16
	_, err := fmt.Sscanf(key, "%d", &v)
	return v, err
}

func (p *settingsPanel) doWrite(raw uint16, shown string) {
	g := p.app
	reg := p.currentReg()
	if reg == nil {
		return
	}
	p.writeBusy = true
	p.writeBtn.Disable()
	g.setStatus("Writing %s to 0x%04X…", shown, reg.Address)

	g.async(func() {
		conn := g.getConn()
		if conn == nil {
			ui(func() { p.writeDone() })
			g.setStatus("Write aborted: disconnected")
			return
		}

		err := conn.WriteRegister(reg.Address, raw)
		var readback string
		var readErr error
		if err == nil && reg.Access == "RW" {
			time.Sleep(60 * time.Millisecond) // device settle before readback
			var words []uint16
			words, readErr = conn.ReadRegister(reg)
			if readErr == nil {
				readback = controller.DecodeValue(reg, words)
			}
		}

		ui(func() {
			p.writeDone()
			if err != nil {
				g.log("✗ Write 0x%04X = 0x%04X failed: %v", reg.Address, raw, err)
				g.setStatus("Write failed")
				dialog.ShowError(fmt.Errorf("write to %s (0x%04X) failed:\n%v", reg.Name, reg.Address, err), g.window)
				return
			}
			g.log("✓ Wrote %s → %s (0x%04X)", shown, reg.Name, reg.Address)
			for i := range p.rows {
				if p.rows[i].reg.Address == reg.Address {
					p.rows[i].note = "written " + time.Now().Format("15:04:05")
					if readErr == nil && readback != "" {
						p.rows[i].value = readback
					}
				}
			}
			p.list.Refresh()
			p.showEditor()
			if readErr != nil {
				g.reportError("Written to 0x%04X; readback failed: %v", reg.Address, readErr)
			} else if readback != "" {
				g.setStatus("Written; device reports %s", readback)
				if readback != shown {
					dialog.ShowInformation("Write verified",
						fmt.Sprintf("Written %s.\nDevice reports: %s", shown, readback), g.window)
				}
			} else {
				g.setStatus("Written (%s)", shown)
			}
		})
	})
}

func (p *settingsPanel) writeDone() {
	p.writeBusy = false
	p.writeBtn.Enable()
}

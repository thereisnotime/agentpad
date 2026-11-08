package ui

import (
	"fmt"
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/pad"
)

// ledPanel is the lighting section of the Pad screen for one pad model.
type ledPanel struct {
	u       *appUI
	pid     uint16
	caps    pad.LEDCaps
	content fyne.CanvasObject

	// 8890: one global effect by index.
	mode       int
	tiles      []*modeTile
	labelEntry *widget.Entry
	labelHint  *widget.Label
	// 884x: one setting per layer.
	layers []*ledEditor

	policyEditors []*ledEditor
	follow        *widget.Check
	applyBtn      *widget.Button
	message       *widget.Label
}

func newLEDPanel(u *appUI, pid uint16) *ledPanel {
	p := &ledPanel{u: u, pid: pid, caps: pad.CapsFor(pid), mode: 1}
	p.message = secondary("")
	p.message.Hide()
	p.applyBtn = widget.NewButtonWithIcon("Apply LED", theme.ColorPaletteIcon(), p.apply)
	p.applyBtn.Importance = widget.HighImportance

	var manual fyne.CanvasObject
	if p.caps.PerLayer {
		manual = p.buildLayers()
	} else {
		manual = p.buildModes()
	}
	note := secondary(p.caps.Note)
	lighting := newCard("Lighting", p.subtitle(), container.NewVBox(manual, note, p.message, actionRow(p.applyBtn)))
	p.content = container.NewVBox(lighting, p.buildPolicy())
	return p
}

func (p *ledPanel) subtitle() string {
	if p.caps.PerLayer {
		return fmt.Sprintf("Board %04x · effect and colour per layer", p.caps.ProductID)
	}
	return fmt.Sprintf("Board %04x · built-in effects by number, 0 is off", p.caps.ProductID)
}

// modeLabel is the user's name for 8890 effect n, or the caps default.
func (p *ledPanel) modeLabel(n int) string {
	def := ""
	if n >= 0 && n < len(p.caps.Modes) {
		def = p.caps.Modes[n].Label
	}
	if s := p.u.prefs.LEDLabel(n); s != "" {
		return s
	}
	return def
}

func (p *ledPanel) buildModes() fyne.CanvasObject {
	grid := newGrid(112)
	for _, m := range p.caps.Modes {
		n := m.Index
		tile := newModeTile(n, p.modeLabel(n), func() { p.selectMode(n) })
		p.tiles = append(p.tiles, tile)
		grid.Add(tile)
	}
	p.labelEntry = widget.NewEntry()
	p.labelEntry.SetPlaceHolder("e.g. Breathing")
	p.labelHint = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.labelEntry.OnChanged = func(s string) {
		p.u.prefs.SetLEDLabel(p.mode, s)
		p.refreshLabels()
	}
	p.selectMode(p.mode)
	rename := container.NewBorder(nil, nil, vcenter(p.labelHint), nil, p.labelEntry)
	return container.NewVBox(grid, rename)
}

func (p *ledPanel) selectMode(n int) {
	p.mode = n
	for i, t := range p.tiles {
		t.setSelected(i == n)
	}
	p.labelHint.SetText(fmt.Sprintf("Name for %d", n))
	cb := p.labelEntry.OnChanged
	p.labelEntry.OnChanged = nil
	p.labelEntry.SetText(p.u.prefs.LEDLabel(n))
	p.labelEntry.OnChanged = cb
	p.message.Hide()
}

func (p *ledPanel) buildLayers() fyne.CanvasObject {
	rows := newGrid(240)
	for layer := range 3 {
		e := newLEDEditor(p, pad.LEDSetting{Layer: layer, Effect: "backlight", Color: "purple"}, func(pad.LEDSetting) { p.message.Hide() })
		p.layers = append(p.layers, e)
		title := text(fmt.Sprintf("Layer %d", layer+1), 13, colText2, true)
		rows.Add(container.NewVBox(title, e.content))
	}
	return rows
}

func (p *ledPanel) buildPolicy() fyne.CanvasObject {
	follow := widget.NewCheck("LED follows agents", nil)
	follow.SetChecked(p.u.ledFollow.Load())
	follow.OnChanged = p.u.setLEDFollow
	p.follow = follow

	cur := defaultLEDPolicy(p.pid)
	if pol := p.u.policy.Load(); pol != nil {
		cur = *pol
	}
	attention := newLEDEditor(p, cur.Attention, func(s pad.LEDSetting) {
		pol := *p.u.policy.Load()
		pol.Attention = s
		p.u.setLEDPolicy(pol)
	})
	otherwise := newLEDEditor(p, cur.Otherwise, func(s pad.LEDSetting) {
		pol := *p.u.policy.Load()
		pol.Otherwise = s
		p.u.setLEDPolicy(pol)
	})
	p.policyEditors = []*ledEditor{attention, otherwise}

	row := func(dot color.NRGBA, title string, e *ledEditor) fyne.CanvasObject {
		d := canvas.NewCircle(dot)
		head := container.NewHBox(vcenter(fixed(10, 10, d)), text(title, 13, colText, true))
		return container.NewVBox(head, e.content)
	}
	where := "layer 1"
	if !p.caps.PerLayer {
		where = "the whole pad"
	}
	return newCard("Agent lighting", "What the LED shows on "+where+" while following your agents",
		container.NewVBox(follow,
			newGrid(260, row(colorAttention, "When an agent needs you", attention), row(colorIdle, "Otherwise", otherwise))))
}

// refreshLabels re-reads the user's 8890 effect names into tiles and selects.
func (p *ledPanel) refreshLabels() {
	for i, t := range p.tiles {
		if l := p.modeLabel(i); l != t.label.Text {
			t.setLabel(l)
		}
	}
	for _, e := range p.policyEditors {
		e.refreshOptions()
	}
}

// apply writes the manual setting(s) through the device worker.
func (p *ledPanel) apply() {
	var settings []pad.LEDSetting
	if p.caps.PerLayer {
		for _, e := range p.layers {
			settings = append(settings, e.value)
		}
	} else {
		settings = []pad.LEDSetting{{Mode: p.mode}}
	}
	for _, s := range settings {
		if err := pad.ValidateLED(p.caps, s); err != nil {
			p.showError(err.Error())
			return
		}
	}
	if !p.u.padPresent.Load() {
		p.showError(writeErrMessage(pad.ErrNotConnected))
		return
	}
	p.applyBtn.Disable()
	p.say(widget.LowImportance, "Writing LED settings…")
	remaining := len(settings)
	var firstErr error
	for _, s := range settings {
		p.u.dev.SetLED(s, func(err error) {
			fyne.Do(func() {
				if err != nil && firstErr == nil {
					firstErr = err
				}
				remaining--
				if remaining > 0 {
					return
				}
				p.applyBtn.Enable()
				if firstErr != nil {
					p.showError("LED write failed: " + writeErrMessage(firstErr))
					p.u.showWriteError("LED write failed", firstErr)
					return
				}
				p.say(widget.SuccessImportance, "LED settings written.")
				p.u.toasts.show(toastSuccess, "LED settings written to the pad.")
			})
		})
	}
}

func (p *ledPanel) showError(s string) { p.say(widget.DangerImportance, s) }

func (p *ledPanel) say(imp widget.Importance, s string) {
	p.message.Importance = imp
	p.message.SetText(s)
	p.message.Show()
}

// ledEditor edits one pad.LEDSetting for the panel's model: an effect select
// for 8890; effect select plus colour swatches for 884x.
type ledEditor struct {
	p        *ledPanel
	value    pad.LEDSetting
	onChange func(pad.LEDSetting)
	content  fyne.CanvasObject

	sel      *widget.Select
	options  []string
	swatches []*swatch
	swRow    *fyne.Container
}

func newLEDEditor(p *ledPanel, initial pad.LEDSetting, onChange func(pad.LEDSetting)) *ledEditor {
	e := &ledEditor{p: p, value: initial}
	e.sel = widget.NewSelect(nil, nil)
	e.refreshOptions()
	e.sel.OnChanged = func(string) { e.changed() }
	if !p.caps.PerLayer {
		e.content = e.sel
		e.onChange = onChange
		return e
	}
	e.swRow = chipRow()
	for _, c := range p.caps.BacklightColors {
		name := c
		s := newSwatch(name, func() {
			e.value.Color = name
			e.syncSwatches()
			e.emit()
		})
		e.swatches = append(e.swatches, s)
		e.swRow.Add(s)
	}
	e.syncSwatches()
	e.content = container.NewVBox(e.sel, e.swRow)
	e.onChange = onChange
	return e
}

// refreshOptions rebuilds the select's labels (8890 names can be edited).
func (e *ledEditor) refreshOptions() {
	caps := e.p.caps
	e.options = e.options[:0]
	sel := 0
	for i, m := range caps.Modes {
		if caps.PerLayer {
			e.options = append(e.options, m.Label)
			if m.ID == e.value.Effect {
				sel = i
			}
		} else {
			e.options = append(e.options, fmt.Sprintf("%d · %s", m.Index, e.p.modeLabel(m.Index)))
			if m.Index == e.value.Mode {
				sel = i
			}
		}
	}
	cb := e.sel.OnChanged
	e.sel.OnChanged = nil
	e.sel.SetOptions(slices.Clone(e.options))
	e.sel.SetSelectedIndex(sel)
	e.sel.OnChanged = cb
}

func (e *ledEditor) changed() {
	i := e.sel.SelectedIndex()
	if i < 0 {
		return
	}
	m := e.p.caps.Modes[i]
	if e.p.caps.PerLayer {
		e.value.Effect = m.ID
		e.value.Mode = 0
		if !m.Color {
			e.value.Color = ""
		} else if !e.colorAllowed(e.value.Color) {
			e.value.Color = e.p.caps.Colors[0]
		}
		e.syncSwatches()
	} else {
		e.value = pad.LEDSetting{Mode: m.Index}
	}
	e.emit()
}

func (e *ledEditor) colorAllowed(c string) bool {
	allowed := e.p.caps.Colors
	if e.value.Effect == "backlight" {
		allowed = e.p.caps.BacklightColors
	}
	return slices.Contains(allowed, c)
}

func (e *ledEditor) syncSwatches() {
	if e.swRow == nil {
		return
	}
	needsColor := false
	for _, m := range e.p.caps.Modes {
		if m.ID == e.value.Effect {
			needsColor = m.Color
		}
	}
	if needsColor {
		e.swRow.Show()
	} else {
		e.swRow.Hide()
	}
	for _, s := range e.swatches {
		if s.name == "white" && e.value.Effect != "backlight" {
			s.Hide()
		} else {
			s.Show()
		}
		s.setSelected(s.name == e.value.Color)
	}
	e.swRow.Refresh()
}

func (e *ledEditor) emit() {
	if e.onChange != nil {
		e.onChange(e.value)
	}
}

// modeTile is a selectable card for one 8890 effect.
type modeTile struct {
	*tapBox
	bg       *canvas.Rectangle
	index    *canvas.Text
	label    *canvas.Text
	selected bool
	hovered  bool
}

func newModeTile(n int, label string, onTap func()) *modeTile {
	t := &modeTile{bg: roundRect(colSurface2, 12), index: text(fmt.Sprint(n), 24, colText, true), label: text(label, 12, colText2, false)}
	t.bg.StrokeWidth = 1.5
	col := container.NewVBox(t.index, t.label)
	t.tapBox = newTapBox(container.NewStack(t.bg, inset(10, 12, 14, 14, col)), onTap, func(h bool) { t.hovered = h; t.restyle() })
	t.restyle()
	return t
}

func (t *modeTile) setLabel(s string) {
	t.label.Text = s
	t.label.Refresh()
}

func (t *modeTile) setSelected(on bool) {
	t.selected = on
	t.restyle()
}

func (t *modeTile) restyle() {
	switch {
	case t.selected:
		t.bg.FillColor, t.bg.StrokeColor = rgb(0x262044), colAccent
		t.index.Color = rgb(0xc9bcff)
	case t.hovered:
		t.bg.FillColor, t.bg.StrokeColor = colSurface3, rgb(0x3a3f4c)
		t.index.Color = colText
	default:
		t.bg.FillColor, t.bg.StrokeColor = colSurface2, colBorder
		t.index.Color = colText
	}
	t.bg.Refresh()
	t.index.Refresh()
}

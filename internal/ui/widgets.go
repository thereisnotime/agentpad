package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---- layouts ---------------------------------------------------------------

// fixedLayout gives its children a fixed min size and stacks them.
type fixedLayout struct{ size fyne.Size }

func (l fixedLayout) MinSize([]fyne.CanvasObject) fyne.Size { return l.size }
func (l fixedLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func fixed(w, h float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(fixedLayout{fyne.NewSize(w, h)}, objs...)
}

// gridLayout is a responsive grid: as many columns of at least minCol as fit.
type gridLayout struct {
	minCol, gap float32
	lastW       float32
}

func newGrid(minCol float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(&gridLayout{minCol: minCol, gap: 12}, objs...)
}

func (l *gridLayout) visible(objs []fyne.CanvasObject) []fyne.CanvasObject {
	out := objs[:0:0]
	for _, o := range objs {
		if o.Visible() {
			out = append(out, o)
		}
	}
	return out
}

func (l *gridLayout) rows(objs []fyne.CanvasObject, cols int) []float32 {
	var hs []float32
	for i, o := range objs {
		if i%cols == 0 {
			hs = append(hs, 0)
		}
		hs[len(hs)-1] = max(hs[len(hs)-1], o.MinSize().Height)
	}
	return hs
}

func (l *gridLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	objs = l.visible(objs)
	var w float32
	for _, o := range objs {
		w = max(w, o.MinSize().Width)
	}
	cols := 1
	if l.lastW > 0 {
		cols = columnsFor(l.lastW, max(l.minCol, w), l.gap)
	}
	var h float32
	for i, rh := range l.rows(objs, cols) {
		if i > 0 {
			h += l.gap
		}
		h += rh
	}
	return fyne.NewSize(w, h)
}

func (l *gridLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	l.lastW = size.Width
	objs = l.visible(objs)
	var minW float32
	for _, o := range objs {
		minW = max(minW, o.MinSize().Width)
	}
	cols := columnsFor(size.Width, max(l.minCol, minW), l.gap)
	cw := (size.Width - l.gap*float32(cols-1)) / float32(cols)
	hs := l.rows(objs, cols)
	var y float32
	for i, o := range objs {
		r, c := i/cols, i%cols
		if c == 0 && r > 0 {
			y += hs[r-1] + l.gap
		}
		o.Move(fyne.NewPos(float32(c)*(cw+l.gap), y))
		o.Resize(fyne.NewSize(cw, hs[r]))
	}
}

// twoPaneLayout puts [main, side] side by side when wide enough, else stacks
// them. Both panes keep their own height (top aligned, no stretching).
type twoPaneLayout struct {
	side, gap float32
	lastW     float32
}

func newTwoPane(main, side fyne.CanvasObject) *fyne.Container {
	return container.New(&twoPaneLayout{side: inspectorWidth, gap: 16}, main, side)
}

func (l *twoPaneLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	if l.lastW > 0 && sideBySide(l.lastW) {
		return fyne.NewSize(max(a.Width, b.Width), max(a.Height, b.Height))
	}
	return fyne.NewSize(max(a.Width, b.Width), a.Height+l.gap+b.Height)
}

func (l *twoPaneLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	l.lastW = size.Width
	main, side := objs[0], objs[1]
	if sideBySide(size.Width) {
		sw := min(l.side, size.Width/2)
		main.Move(fyne.NewPos(0, 0))
		main.Resize(fyne.NewSize(size.Width-sw-l.gap, main.MinSize().Height))
		side.Move(fyne.NewPos(size.Width-sw, 0))
		side.Resize(fyne.NewSize(sw, side.MinSize().Height))
		return
	}
	mh := main.MinSize().Height
	main.Move(fyne.NewPos(0, 0))
	main.Resize(fyne.NewSize(size.Width, mh))
	side.Move(fyne.NewPos(0, mh+l.gap))
	side.Resize(fyne.NewSize(size.Width, side.MinSize().Height))
}

// columnsLayout lays its children out in cols() equal columns, row by row.
type columnsLayout struct {
	cols func() int
	gap  float32
}

func (l *columnsLayout) n(objs []fyne.CanvasObject) int { return max(1, min(l.cols(), len(objs))) }

func (l *columnsLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	cols := l.n(objs)
	var w, h, rowH float32
	for i, o := range objs {
		m := o.MinSize()
		w = max(w, m.Width)
		rowH = max(rowH, m.Height)
		if i%cols == cols-1 || i == len(objs)-1 {
			if h > 0 {
				h += l.gap
			}
			h += rowH
			rowH = 0
		}
	}
	return fyne.NewSize(w*float32(cols)+l.gap*float32(cols-1), h)
}

func (l *columnsLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	cols := l.n(objs)
	cw := (size.Width - l.gap*float32(cols-1)) / float32(cols)
	var y float32
	for r := 0; r*cols < len(objs); r++ {
		row := objs[r*cols : min(len(objs), (r+1)*cols)]
		var rh float32
		for _, o := range row {
			rh = max(rh, o.MinSize().Height)
		}
		for c, o := range row {
			o.Move(fyne.NewPos(float32(c)*(cw+l.gap), y))
			o.Resize(fyne.NewSize(cw, rh))
		}
		y += rh + l.gap
	}
}

// cappedLayout sizes a single scroll to its content, but no taller than
// limit() (0 = no limit), so it scrolls inside itself beyond that.
type cappedLayout struct {
	scroll *container.Scroll
	limit  func() float32
}

func (l *cappedLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	c := l.scroll.Content.MinSize()
	h := c.Height
	if m := l.limit(); m > 0 && h > m {
		h = m
	}
	return fyne.NewSize(c.Width, h)
}

func (l *cappedLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func cappedScroll(content fyne.CanvasObject, limit func() float32) *fyne.Container {
	s := container.NewVScroll(content)
	return container.New(&cappedLayout{scroll: s, limit: limit}, s)
}

// collapsible is a section with a header that shows or hides its body.
type collapsible struct {
	title   string
	open    bool
	head    *widget.Button
	body    *fyne.Container
	content *fyne.Container
}

func newCollapsible(title string, open bool, body ...fyne.CanvasObject) *collapsible {
	c := &collapsible{title: title, body: container.NewVBox(body...)}
	c.head = widget.NewButton(title, func() { c.setOpen(!c.open) })
	c.head.Alignment = widget.ButtonAlignLeading
	c.head.Importance = widget.LowImportance
	c.content = container.NewVBox(c.head, c.body)
	c.open = !open
	c.setOpen(open)
	return c
}

func (c *collapsible) setOpen(on bool) {
	if c.open == on {
		return
	}
	c.open = on
	if on {
		c.head.SetIcon(theme.MenuDropDownIcon())
		c.body.Show()
	} else {
		c.head.SetIcon(theme.NavigateNextIcon())
		c.body.Hide()
	}
	c.content.Refresh()
}

// setBody replaces the body's children.
func (c *collapsible) setBody(objs ...fyne.CanvasObject) {
	c.body.Objects = objs
	c.body.Refresh()
}

// ---- text helpers ------------------------------------------------------------

func wrapLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// secondary is readable secondary text that wraps.
func secondary(s string) *widget.Label {
	l := wrapLabel(s)
	l.Importance = widget.LowImportance
	return l
}

func boldLabel(s string) *widget.Label {
	l := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

func text(s string, size float32, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold}
	return t
}

func monoText(s string, size float32, c color.Color) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Monospace: true}
	return t
}

func inset(t, b, l, r float32, o fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewCustomPaddedLayout(t, b, l, r), o)
}

// tight stacks title-like labels without the usual gap between them.
func tight(objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewCustomPaddedVBoxLayout(-8), objs...)
}

// vcenter centres o vertically without stretching it.
func vcenter(o fyne.CanvasObject) *fyne.Container {
	return container.NewVBox(layout.NewSpacer(), o, layout.NewSpacer())
}

// ---- surfaces ------------------------------------------------------------------

func roundRect(fill color.Color, radius float32) *canvas.Rectangle {
	r := canvas.NewRectangle(fill)
	r.CornerRadius = radius
	return r
}

// surface puts content on a raised card background.
func surface(content fyne.CanvasObject) *fyne.Container {
	bg := roundRect(colSurface, 14)
	bg.StrokeColor = colBorder
	bg.StrokeWidth = 1
	return container.NewStack(bg, inset(14, 16, 16, 16, content))
}

// newCard is a titled card; subtitle may be empty.
func newCard(title, subtitle string, body fyne.CanvasObject) *fyne.Container {
	head := tight(boldLabel(title))
	if subtitle != "" {
		head.Add(secondary(subtitle))
	}
	return surface(container.NewBorder(head, nil, nil, nil, body))
}

// screen wraps a screen body: padded and vertically scrolling inside itself.
func screen(body fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVScroll(inset(18, 24, 20, 20, body))
}

// screenTitle is the large heading at the top of a screen.
func screenTitle(title, sub string) fyne.CanvasObject {
	t := text(title, 22, colText, true)
	if sub == "" {
		return t
	}
	return container.NewVBox(t, inset(-6, 0, -8, 0, secondary(sub)))
}

// ---- tapBox: a tappable, hoverable wrapper around plain canvas content ---------

type tapBox struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
	onHover func(bool)
}

func newTapBox(content fyne.CanvasObject, onTap func(), onHover func(bool)) *tapBox {
	t := &tapBox{content: content, onTap: onTap, onHover: onHover}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapBox) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(t.content) }

func (t *tapBox) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapBox) MouseIn(*desktop.MouseEvent) {
	if t.onHover != nil {
		t.onHover(true)
	}
}
func (t *tapBox) MouseMoved(*desktop.MouseEvent) {}
func (t *tapBox) MouseOut() {
	if t.onHover != nil {
		t.onHover(false)
	}
}

func (t *tapBox) Cursor() desktop.Cursor {
	if t.onTap == nil {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

var (
	_ fyne.Tappable      = (*tapBox)(nil)
	_ desktop.Hoverable  = (*tapBox)(nil)
	_ desktop.Cursorable = (*tapBox)(nil)
)

// ---- chip ------------------------------------------------------------------------

// chip is a rounded, selectable label with an optional muted text tag
// (e.g. "edited").
type chip struct {
	*tapBox
	bg       *canvas.Rectangle
	label    *canvas.Text
	tag      *canvas.Text
	selected bool
	hovered  bool
}

func newChip(label string, mono bool, onTap func()) *chip {
	c := &chip{bg: roundRect(colSurface2, 15), label: text(label, 13, colText, false)}
	c.label.TextStyle.Monospace = mono
	c.bg.StrokeWidth = 1
	c.tag = text("", 11, colText3, false)
	c.tag.Hide()
	row := container.NewHBox(vcenter(c.label), vcenter(c.tag))
	c.tapBox = newTapBox(container.NewStack(c.bg, inset(5, 5, 12, 12, row)), onTap, func(h bool) {
		c.hovered = h
		c.restyle()
	})
	c.restyle()
	return c
}

func (c *chip) setSelected(on bool) {
	if c.selected != on {
		c.selected = on
		c.restyle()
	}
}

// setTag shows a small muted tag after the label ("" hides it).
func (c *chip) setTag(s string) {
	c.tag.Text = s
	if s == "" {
		c.tag.Hide()
	} else {
		c.tag.Show()
	}
	c.restyle()
}

func (c *chip) restyle() {
	switch {
	case c.selected:
		c.bg.FillColor, c.bg.StrokeColor = colAccent, colAccent
		c.label.Color = color.White
	case c.hovered:
		c.bg.FillColor, c.bg.StrokeColor = colSurface3, rgb(0x3a3f4c)
		c.label.Color = colText
	default:
		c.bg.FillColor, c.bg.StrokeColor = colSurface2, colBorder
		c.label.Color = colText
	}
	c.label.TextStyle.Bold = c.selected
	c.tag.Color = colText3
	if c.selected {
		c.tag.Color = withAlpha(rgb(0xffffff), 0xb0)
	}
	c.bg.Refresh()
	c.label.Refresh()
	c.tag.Refresh()
}

// chipRow wraps chips onto as many lines as needed.
func chipRow(objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewRowWrapLayoutWithCustomPadding(6, 6), objs...)
}

// ---- pill (header status) ----------------------------------------------------------

type pill struct {
	*tapBox
	bg    *canvas.Rectangle
	dot   *canvas.Circle
	text  *canvas.Text
	col   color.NRGBA
	pulse *fyne.Animation
}

// setPulsing makes the pill breathe (used while an agent waits).
func (p *pill) setPulsing(on bool) {
	if on == (p.pulse != nil) {
		return
	}
	if !on {
		p.pulse.Stop()
		p.pulse = nil
		p.set(p.col, p.text.Text)
		return
	}
	p.pulse = fyne.NewAnimation(900*time.Millisecond, func(f float32) {
		p.bg.FillColor = withAlpha(p.col, uint8(0x24+float32(0x50)*f))
		p.dot.FillColor = mix(p.col, colText, f*0.5)
		p.bg.Refresh()
		p.dot.Refresh()
	})
	p.pulse.AutoReverse = true
	p.pulse.RepeatCount = fyne.AnimationRepeatForever
	p.pulse.Curve = fyne.AnimationEaseInOut
	p.pulse.Start()
}

func newPill(onTap func()) *pill {
	p := &pill{bg: roundRect(colSurface2, 13), dot: canvas.NewCircle(colorIdle), text: text("", 12, colText, false)}
	row := container.NewHBox(vcenter(fixed(8, 8, p.dot)), vcenter(p.text))
	p.tapBox = newTapBox(container.NewStack(p.bg, inset(4, 4, 10, 12, row)), onTap, nil)
	return p
}

func (p *pill) set(c color.NRGBA, s string) {
	p.col = c
	p.dot.FillColor = c
	p.bg.FillColor = withAlpha(c, 0x24)
	p.bg.StrokeColor = withAlpha(c, 0x55)
	p.bg.StrokeWidth = 1
	p.text.Text = s
	p.dot.Refresh()
	p.bg.Refresh()
	p.text.Refresh()
	p.Refresh()
}

// ---- swatch --------------------------------------------------------------------------

type swatch struct {
	*tapBox
	ring *canvas.Circle
	name string
}

func newSwatch(name string, onTap func()) *swatch {
	s := &swatch{ring: canvas.NewCircle(color.Transparent), name: name}
	s.ring.StrokeWidth = 2
	fill := canvas.NewCircle(ledColor(name))
	s.tapBox = newTapBox(fixed(30, 30, s.ring, inset(4, 4, 4, 4, fill)), onTap, nil)
	s.setSelected(false)
	return s
}

func (s *swatch) setSelected(on bool) {
	if on {
		s.ring.StrokeColor = colText
	} else {
		s.ring.StrokeColor = color.Transparent
	}
	s.ring.Refresh()
}

// ---- status dot + text -------------------------------------------------------------------

// statusIcon is a coloured circle with a theme icon glyph inside.
func statusIcon(res fyne.Resource, c color.NRGBA) (*fyne.Container, *canvas.Circle, *widget.Icon) {
	bg := canvas.NewCircle(withAlpha(c, 0x30))
	ic := widget.NewIcon(res)
	return fixed(40, 40, bg, inset(9, 9, 9, 9, ic)), bg, ic
}

func themedIcon(res fyne.Resource, primary bool) fyne.Resource {
	if primary {
		return theme.NewPrimaryThemedResource(res)
	}
	return res
}

package ui

import (
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/profile"
)

type controlShape int

const (
	shapeKey controlShape = iota
	shapeKnob
	shapeTurn
)

// padControl is one clickable part of the drawn pad: a keycap, the knob, or
// one of the knob's turn chips.
type padControl struct {
	widget.BaseWidget
	shape    controlShape
	glyph    string // small marker: key number, ↺ ● ↻
	action   string
	unit     float32 // key size from the pad layout, drives font size
	selected bool
	hovered  bool
	onTap    func()

	// live feedback state, driven by animations (UI goroutine)
	press     float32 // 0..1 how far the cap is pushed in
	angle     float64 // knob indicator tick in degrees
	pulse     float32 // knob ring pulse 0..1
	pressAnim *fyne.Animation
	tickAnim  *fyne.Animation
	pulseAnim *fyne.Animation
}

func stopAnim(a *fyne.Animation) {
	if a != nil {
		a.Stop()
	}
}

// animatePress sinks the control (down) or lets it spring back (up), ~120 ms ease-out.
func (c *padControl) animatePress(down bool) {
	stopAnim(c.pressAnim)
	from, to := c.press, float32(0)
	if down {
		to = 1
	}
	c.pressAnim = fyne.NewAnimation(120*time.Millisecond, func(p float32) {
		c.press = from + (to-from)*p
		c.Refresh()
	})
	c.pressAnim.Curve = fyne.AnimationEaseOut
	c.pressAnim.Start()
}

// animateTick turns the knob indicator by full degrees and eases it back.
func (c *padControl) animateTick(full float64) {
	stopAnim(c.tickAnim)
	c.tickAnim = fyne.NewAnimation(360*time.Millisecond, func(p float32) {
		c.angle = tickAngle(full, p)
		c.Refresh()
	})
	c.tickAnim.Curve = fyne.AnimationLinear
	c.tickAnim.Start()
}

// animatePulse flashes the knob ring.
func (c *padControl) animatePulse() {
	stopAnim(c.pulseAnim)
	c.pulseAnim = fyne.NewAnimation(450*time.Millisecond, func(p float32) {
		c.pulse = 1 - p
		c.Refresh()
	})
	c.pulseAnim.Curve = fyne.AnimationEaseOut
	c.pulseAnim.Start()
}

func newPadControl(shape controlShape, glyph string, onTap func()) *padControl {
	c := &padControl{shape: shape, glyph: glyph, onTap: onTap, unit: 60}
	c.ExtendBaseWidget(c)
	return c
}

func (c *padControl) setAction(a string) {
	c.action = a
	c.Refresh()
}

func (c *padControl) setSelected(on bool) {
	if c.selected != on {
		c.selected = on
		c.Refresh()
	}
}

func (c *padControl) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}
func (c *padControl) MouseIn(*desktop.MouseEvent)    { c.hovered = true; c.Refresh() }
func (c *padControl) MouseMoved(*desktop.MouseEvent) {}
func (c *padControl) MouseOut()                      { c.hovered = false; c.Refresh() }
func (c *padControl) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (c *padControl) CreateRenderer() fyne.WidgetRenderer {
	r := &padControlRenderer{c: c,
		shadowRect: roundRect(rgb(0x0b0c10), 8), faceRect: roundRect(colSurface3, 8),
		shadowCirc: canvas.NewCircle(rgb(0x0b0c10)), faceCirc: canvas.NewCircle(colSurface3),
		ring:   canvas.NewCircle(color.Transparent),
		marker: canvas.NewLine(colText2),
		glyph:  monoText("", 10, colText3),
		action: monoText("", 14, colText),
	}
	r.action.TextStyle.Bold = true
	r.Refresh()
	return r
}

type padControlRenderer struct {
	c                    *padControl
	shadowRect, faceRect *canvas.Rectangle
	shadowCirc, faceCirc *canvas.Circle
	ring                 *canvas.Circle
	marker               *canvas.Line
	glyph, action        *canvas.Text
}

func (r *padControlRenderer) Objects() []fyne.CanvasObject {
	switch r.c.shape {
	case shapeKnob:
		return []fyne.CanvasObject{r.shadowCirc, r.faceCirc, r.ring, r.marker, r.action}
	default:
		return []fyne.CanvasObject{r.shadowRect, r.faceRect, r.glyph, r.action}
	}
}

func (r *padControlRenderer) Destroy() {}

func (r *padControlRenderer) MinSize() fyne.Size { return fyne.NewSize(10, 10) }

func (r *padControlRenderer) Layout(size fyne.Size) {
	u := r.c.unit
	w, h := size.Width, size.Height
	switch r.c.shape {
	case shapeKey:
		lip := max(3, u*0.08)
		r.shadowRect.CornerRadius = u * 0.16
		r.faceRect.CornerRadius = u * 0.14
		r.shadowRect.Move(fyne.NewPos(0, 0))
		r.shadowRect.Resize(size)
		sink := lip * 0.9 * r.c.press
		r.faceRect.Move(fyne.NewPos(lip*0.6, lip*0.4+sink))
		r.faceRect.Resize(fyne.NewSize(w-lip*1.2, h-lip*1.6))
		r.glyph.TextSize = max(9, u*0.13)
		r.glyph.Move(fyne.NewPos(u*0.13, u*0.08+sink))
		r.fitAction(w-u*0.2, max(11, u*0.2))
		as := r.action.MinSize()
		r.action.Move(fyne.NewPos((w-as.Width)/2, (h-lip-as.Height)/2+sink))
	case shapeKnob:
		r.shadowCirc.Move(fyne.NewPos(0, 0))
		r.shadowCirc.Resize(size)
		in := w * 0.05
		r.faceCirc.Move(fyne.NewPos(in, in*0.6))
		r.faceCirc.Resize(fyne.NewSize(w-2*in, h-2*in))
		ri := w * 0.17
		r.ring.Move(fyne.NewPos(ri, ri*0.9))
		r.ring.Resize(fyne.NewSize(w-2*ri, h-2*ri))
		// indicator line from near the rim towards the centre, rotated by angle
		rad := r.c.angle * math.Pi / 180
		cx, cy := w/2, h/2-in*0.2
		sin, cos := float32(math.Sin(rad)), float32(math.Cos(rad))
		r1, r2 := w*0.42, w*0.3
		r.marker.Position1 = fyne.NewPos(cx+sin*r1, cy-cos*r1)
		r.marker.Position2 = fyne.NewPos(cx+sin*r2, cy-cos*r2)
		r.marker.StrokeWidth = max(2, u*0.04)
		r.fitAction(w*0.62, max(11, u*0.18))
		as := r.action.MinSize()
		r.action.Move(fyne.NewPos((w-as.Width)/2, (h-as.Height)/2+h*0.06))
	case shapeTurn:
		r.shadowRect.CornerRadius = h / 2
		r.faceRect.CornerRadius = h / 2
		r.shadowRect.Resize(size)
		r.faceRect.Move(fyne.NewPos(1, 1))
		r.faceRect.Resize(fyne.NewSize(w-2, h-2))
		r.glyph.TextSize = max(11, u*0.17)
		gs := r.glyph.MinSize()
		r.glyph.Move(fyne.NewPos(h*0.35, (h-gs.Height)/2))
		left := h*0.35 + gs.Width + 4
		r.fitAction(w-left-h*0.3, max(10, u*0.15))
		as := r.action.MinSize()
		r.action.Move(fyne.NewPos(left, (h-as.Height)/2))
	}
}

// fitAction sets the action text, shrinking then truncating it to fit width.
func (r *padControlRenderer) fitAction(width, size float32) {
	label := r.c.action
	style := r.action.TextStyle
	for s := size; s >= 9; s-- {
		if fyne.MeasureText(label, s, style).Width <= width {
			r.action.Text, r.action.TextSize = label, s
			return
		}
	}
	r.action.TextSize = 9
	cw := fyne.MeasureText("m", 9, style).Width
	r.action.Text = capLabel(label, int(width/max(cw, 1)))
}

func (r *padControlRenderer) Refresh() {
	c := r.c
	face, stroke, strokeW := colSurface3, rgb(0x3b404e), float32(1)
	switch {
	case c.selected:
		face, stroke, strokeW = rgb(0x30295a), colAccent, 2.5
	case c.hovered:
		face, stroke = rgb(0x343946), rgb(0x4a5060)
	}
	if c.press > 0 {
		face = mix(face, rgb(0x1b1d24), c.press*0.6)
		stroke = mix(stroke, colAccent, c.press)
		strokeW = max(strokeW, 1+2*c.press)
	}
	r.faceRect.FillColor, r.faceRect.StrokeColor, r.faceRect.StrokeWidth = face, stroke, strokeW
	r.shadowRect.StrokeColor, r.shadowRect.StrokeWidth = withAlpha(colAccent, uint8(0x90*c.press)), 3*c.press
	r.shadowRect.FillColor = rgb(0x0b0c10)
	r.shadowCirc.StrokeColor, r.shadowCirc.StrokeWidth = color.Transparent, 0
	r.faceCirc.FillColor, r.faceCirc.StrokeColor, r.faceCirc.StrokeWidth = rgb(0x2e323d), rgb(0x434958), 1
	switch {
	case c.selected:
		r.shadowCirc.StrokeColor, r.shadowCirc.StrokeWidth = colAccent, 3
		r.faceCirc.FillColor, r.faceCirc.StrokeColor = rgb(0x30295a), colAccent
	case c.hovered:
		r.faceCirc.FillColor = rgb(0x373c49)
	}
	if c.press > 0 {
		r.faceCirc.FillColor = mix(r.faceCirc.FillColor.(color.NRGBA), rgb(0x1b1d24), c.press*0.5)
	}
	r.marker.StrokeColor = mix(colText2, colAccent, max(c.press, float32(math.Abs(c.angle)/detentDegrees)))
	r.ring.StrokeColor, r.ring.StrokeWidth = withAlpha(colText, 0x18), 1.5
	if c.pulse > 0 {
		r.ring.StrokeColor = mix(withAlpha(colText, 0x18), colAccent, c.pulse)
		r.ring.StrokeWidth = 1.5 + 4*c.pulse
	}
	r.glyph.Text = c.glyph
	r.glyph.Color = colText2
	if c.selected {
		r.glyph.Color = rgb(0xc9bcff)
	}
	r.action.Color = colText
	r.Layout(c.Size())
	for _, o := range r.Objects() {
		o.Refresh()
	}
}

// padView draws the pad (body, three keycaps, knob and turn chips) laid out
// for an orientation and scaled to the space it gets.
type padView struct {
	body     *canvas.Rectangle
	controls map[controlID]*padControl
	layout   *padLayout
	content  *fyne.Container
}

func newPadView(o profile.Orientation, onSelect func(controlID)) *padView {
	v := &padView{controls: map[controlID]*padControl{}}
	v.body = roundRect(rgb(0x1e2129), 20)
	v.body.StrokeColor = rgb(0x30343f)
	v.body.StrokeWidth = 1.5
	glyphs := map[controlID]string{ctlKey1: "1", ctlKey2: "2", ctlKey3: "3", ctlKnobCCW: "↺", ctlKnobPress: "●", ctlKnobCW: "↻"}
	objs := []fyne.CanvasObject{v.body}
	for _, id := range allControls {
		shape := shapeKey
		switch id {
		case ctlKnobPress:
			shape = shapeKnob
		case ctlKnobCCW, ctlKnobCW:
			shape = shapeTurn
		}
		c := newPadControl(shape, glyphs[id], func() { onSelect(id) })
		v.controls[id] = c
		objs = append(objs, c)
	}
	v.layout = &padLayout{v: v, orient: o}
	v.content = container.New(v.layout, objs...)
	return v
}

func (v *padView) setOrientation(o profile.Orientation) {
	v.layout.orient = o
	v.content.Refresh()
}

func (v *padView) setSelected(id controlID) {
	for cid, c := range v.controls {
		c.setSelected(cid == id)
	}
}

// padLayout positions the pad parts with padGeometry.
type padLayout struct {
	v      *padView
	orient profile.Orientation
	lastW  float32
}

func (l *padLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	// Height follows the width we were last given, so a narrow, stacked
	// screen still gets a big drawing instead of the absolute minimum.
	u := float32(geoMinUnit)
	if l.lastW > 0 {
		long := l.lastW
		if !l.orient.Vertical() {
			u = max(u, min(long/padLength, geoMaxUnit*0.8))
		} else {
			u = max(u, min(long/padThick, 70))
		}
	}
	w, h := padSizeFor(geoMinUnit, l.orient)
	_, hh := padSizeFor(u, l.orient)
	return fyne.NewSize(w, max(h, hh))
}

func (l *padLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	l.lastW = size.Width
	g := padGeometry(size.Width, size.Height, l.orient)
	place := func(o fyne.CanvasObject, r rect) {
		o.Move(fyne.NewPos(r.X, r.Y))
		o.Resize(fyne.NewSize(r.W, r.H))
	}
	place(l.v.body, g.Body)
	l.v.body.CornerRadius = g.Unit * 0.3
	rects := map[controlID]rect{ctlKey1: g.Keys[0], ctlKey2: g.Keys[1], ctlKey3: g.Keys[2],
		ctlKnobPress: g.Knob, ctlKnobCCW: g.CCW, ctlKnobCW: g.CW}
	for id, c := range l.v.controls {
		c.unit = g.Unit
		place(c, rects[id])
		c.Refresh()
	}
}

package ui

import "github.com/thereisnotime/agentpad/internal/profile"

// rect is an axis-aligned rectangle in container coordinates.
type rect struct{ X, Y, W, H float32 }

func (r rect) right() float32  { return r.X + r.W }
func (r rect) bottom() float32 { return r.Y + r.H }

// overlaps reports whether r and o share any area.
func (r rect) overlaps(o rect) bool {
	return r.X < o.right() && o.X < r.right() && r.Y < o.bottom() && o.Y < r.bottom()
}

// padGeom is where each part of the drawn pad goes.
type padGeom struct {
	Unit    float32 // key size; text scales with it
	Body    rect
	Keys    [3]rect // as seen: left->right, or top->bottom when vertical
	Knob    rect    // knob circle bounds (press target)
	CCW, CW rect    // turn targets under the knob
}

// Proportions in units of the key size.
const (
	geoKeyGap   = 0.2  // between keys
	geoKnobGap  = 0.4  // between the key row and the knob block
	geoMargin   = 0.32 // body padding
	geoKnobSide = 1.7  // knob block is a square of this side
	geoKnobDia  = 1.06 // knob circle diameter
	geoTurnH    = 0.42 // turn chip height
	geoTurnGap  = 0.12 // gap between knob circle and turn chips / between chips
	geoMaxUnit  = 112
	geoMinUnit  = 34
)

// padLength is the body's long side and padThick its short side, in units.
const (
	padLength = 2*geoMargin + 3 + 2*geoKeyGap + geoKnobGap + geoKnobSide
	padThick  = 2*geoMargin + geoKnobSide
)

// padUnitFor returns the key size that makes the pad fit w x h.
func padUnitFor(w, h float32, o profile.Orientation) float32 {
	long, short := w, h
	if o.Vertical() {
		long, short = h, w
	}
	u := min(long/padLength, short/padThick, geoMaxUnit)
	return max(u, 0)
}

// padSizeFor returns the body size for key size u.
func padSizeFor(u float32, o profile.Orientation) (w, h float32) {
	if o.Vertical() {
		return padThick * u, padLength * u
	}
	return padLength * u, padThick * u
}

// padGeometry lays the pad out centred in w x h for orientation o.
// Knob side as seen: normal=right, upsidedown=left, clockwise=bottom,
// counterclockwise=top.
func padGeometry(w, h float32, o profile.Orientation) padGeom {
	u := padUnitFor(w, h, o)
	bw, bh := padSizeFor(u, o)
	g := padGeom{Unit: u, Body: rect{(w - bw) / 2, (h - bh) / 2, bw, bh}}

	m, kb := geoMargin*u, geoKnobSide*u
	keysLen := (3 + 2*geoKeyGap) * u
	vertical := o.Vertical()
	side := o.KnobSide()
	knobFirst := side == profile.SideLeft || side == profile.SideTop

	// Offsets along the long axis, from the body's start.
	keysAt, knobAt := m, m+keysLen+geoKnobGap*u
	if knobFirst {
		knobAt, keysAt = m, m+kb+geoKnobGap*u
	}
	cross := m + (kb-u)/2 // keys centred on the short axis
	for i := range g.Keys {
		along := keysAt + float32(i)*(1+geoKeyGap)*u
		if vertical {
			g.Keys[i] = rect{g.Body.X + cross, g.Body.Y + along, u, u}
		} else {
			g.Keys[i] = rect{g.Body.X + along, g.Body.Y + cross, u, u}
		}
	}

	block := rect{g.Body.X + knobAt, g.Body.Y + m, kb, kb}
	if vertical {
		block = rect{g.Body.X + m, g.Body.Y + knobAt, kb, kb}
	}
	d := geoKnobDia * u
	th, tg := geoTurnH*u, geoTurnGap*u
	top := block.Y + (kb-(d+tg+th))/2
	g.Knob = rect{block.X + (kb-d)/2, top, d, d}
	tw := (kb - tg) / 2
	g.CCW = rect{block.X, top + d + tg, tw, th}
	g.CW = rect{block.X + tw + tg, top + d + tg, tw, th}
	return g
}

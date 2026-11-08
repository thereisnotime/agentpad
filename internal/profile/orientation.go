package profile

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Orientation is how the pad physically sits on the desk.
// Profile keys are always given in the order they are seen:
// left->right when horizontal, top->bottom when vertical.
type Orientation string

const (
	OrientNormal           Orientation = "normal"           // keys left, knob right
	OrientUpsideDown       Orientation = "upsidedown"       // knob left, keys right
	OrientClockwise        Orientation = "clockwise"        // keys top, knob bottom
	OrientCounterClockwise Orientation = "counterclockwise" // knob top, keys bottom
)

// Side is where the knob appears as seen by the user.
type Side string

const (
	SideRight  Side = "right"
	SideLeft   Side = "left"
	SideTop    Side = "top"
	SideBottom Side = "bottom"
)

var orientations = []Orientation{OrientNormal, OrientClockwise, OrientUpsideDown, OrientCounterClockwise}

// Orientations returns all orientations in rotation order (normal, clockwise, upsidedown, counterclockwise).
func Orientations() []Orientation { return append([]Orientation(nil), orientations...) }

// ParseOrientation returns the orientation for s, or OrientNormal and false if unknown.
func ParseOrientation(s string) (Orientation, bool) {
	for _, o := range orientations {
		if string(o) == s {
			return o, true
		}
	}
	return OrientNormal, false
}

func (o Orientation) index() int {
	for i, x := range orientations {
		if x == o {
			return i
		}
	}
	return -1
}

// Vertical reports whether keys are stacked vertically.
func (o Orientation) Vertical() bool {
	return o == OrientClockwise || o == OrientCounterClockwise
}

// KnobSide returns where the knob sits as seen.
func (o Orientation) KnobSide() Side {
	switch o {
	case OrientUpsideDown:
		return SideLeft
	case OrientClockwise:
		return SideBottom
	case OrientCounterClockwise:
		return SideTop
	default:
		return SideRight
	}
}

// RotateCW / RotateCCW return the next orientation when the pad is turned by 90°.
// Unknown orientations are treated as normal.
func (o Orientation) RotateCW() Orientation {
	return orientations[(max(o.index(), 0)+1)%len(orientations)]
}

func (o Orientation) RotateCCW() Orientation {
	n := len(orientations)
	return orientations[(max(o.index(), 0)+n-1)%n]
}

// Label is a short human label, e.g. "Horizontal · knob right".
func (o Orientation) Label() string {
	dir := "Horizontal"
	if o.Vertical() {
		dir = "Vertical"
	}
	return dir + " · knob " + string(o.KnobSide())
}

// MappingYAMLOriented is MappingYAML with an orientation; it emits `orientation:`
// and the button grid shape the tool expects (1x3 horizontal, 3x1 vertical).
// The rows/columns header stays 1x3 (the physical board) for every orientation.
// Keys stay in "as seen" order; the tool remaps them to the physical buttons.
// run:<name> actions are written as F19..F24, see PlainLayers.
func MappingYAMLOriented(layers []Profile, o Orientation) ([]byte, error) {
	if o.index() < 0 {
		return nil, fmt.Errorf("unknown orientation %q", string(o))
	}
	if len(layers) < 1 || len(layers) > 3 {
		return nil, fmt.Errorf("mapping needs 1-3 layers, got %d", len(layers))
	}
	// rows/columns describe the physical board (always 1x3); only the
	// buttons grid is written as seen, i.e. 3 rows of 1 when vertical.
	doc := mappingDoc{Orientation: string(o), Rows: 1, Columns: 3, Knobs: 1}
	for _, p := range layers {
		if err := Validate(p); err != nil {
			return nil, fmt.Errorf("profile %s: %w", p.Name, err)
		}
	}
	// run:<name> controls send spare keys agentpad watches for
	for _, p := range PlainLayers(layers) {
		k := [3]quoted{quoted(p.Keys[0]), quoted(p.Keys[1]), quoted(p.Keys[2])}
		buttons := [][]quoted{{k[0], k[1], k[2]}}
		if o.Vertical() {
			buttons = [][]quoted{{k[0]}, {k[1]}, {k[2]}}
		}
		doc.Layers = append(doc.Layers, mappingLayer{
			Buttons: buttons,
			Knobs:   []mappingKnob{{CCW: quoted(p.Knob.CCW), Press: quoted(p.Knob.Press), CW: quoted(p.Knob.CW)}},
		})
	}
	return yaml.Marshal(doc)
}

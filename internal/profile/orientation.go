package profile

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

// Stubs: implemented by the backend work (replace bodies, keep signatures).

// Orientations returns all orientations in rotation order (normal, clockwise, upsidedown, counterclockwise).
func Orientations() []Orientation { panic("TODO") }

// ParseOrientation returns the orientation for s, or OrientNormal and false if unknown.
func ParseOrientation(s string) (Orientation, bool) { panic("TODO") }

// Vertical reports whether keys are stacked vertically.
func (o Orientation) Vertical() bool { panic("TODO") }

// KnobSide returns where the knob sits as seen.
func (o Orientation) KnobSide() Side { panic("TODO") }

// RotateCW / RotateCCW return the next orientation when the pad is turned by 90°.
func (o Orientation) RotateCW() Orientation  { panic("TODO") }
func (o Orientation) RotateCCW() Orientation { panic("TODO") }

// Label is a short human label, e.g. "Horizontal · knob right".
func (o Orientation) Label() string { panic("TODO") }

// MappingYAMLOriented is MappingYAML with an orientation; it emits `orientation:`
// and the button grid shape the tool expects (1x3 horizontal, 3x1 vertical).
func MappingYAMLOriented(layers []Profile, o Orientation) ([]byte, error) { panic("TODO") }

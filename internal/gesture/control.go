package gesture

import (
	"strings"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
)

// Control is a pad control as seen by the user (orientation is applied by the
// pad firmware via the gesture-mode mapping). Chords are "keyA+keyB".
type Control string

// Controls, with the same names profiles use in `gestures:`.
const (
	Key1      Control = profile.CtrlKey1
	Key2      Control = profile.CtrlKey2
	Key3      Control = profile.CtrlKey3
	KnobCCW   Control = profile.CtrlKnobCCW
	KnobPress Control = profile.CtrlKnobPress
	KnobCW    Control = profile.CtrlKnobCW
)

// Kind is the type of a recognised gesture.
type Kind string

// Gesture kinds, with the same names profiles use in `gestures:`.
const (
	Tap         Kind = profile.KindTap
	Hold        Kind = profile.KindHold
	LongHold    Kind = profile.KindLongHold
	DoubleTap   Kind = profile.KindDoubleTap
	Rotate      Kind = profile.KindRotate
	PressRotate Kind = profile.KindPressRotate
	Chord       Kind = profile.KindChord
)

// Linux keycodes (input-event-codes.h) sent by the pad in gesture mode.
const (
	KeyF13 uint16 = 183 + iota
	KeyF14
	KeyF15
	KeyF16
	KeyF17
	KeyF18
)

var keyControls = map[uint16]Control{
	KeyF13: Key1, KeyF14: Key2, KeyF15: Key3,
	KeyF16: KnobCCW, KeyF17: KnobPress, KeyF18: KnobCW,
}

// ControlForKey maps a Linux keycode from a pad in gesture mode (see
// profile.GestureModeMapping) to its control.
func ControlForKey(code uint16) (Control, bool) {
	c, ok := keyControls[code]
	return c, ok
}

// ChordOf returns the canonical chord control for two keys ("key1+key3"),
// or "" if they cannot form a chord.
func ChordOf(a, b Control) Control {
	return Control(profile.ChordName(string(a), string(b)))
}

// IsChord reports whether c is a chord control.
func (c Control) IsChord() bool { return strings.Contains(string(c), "+") }

func (c Control) isDetent() bool { return c == KnobCCW || c == KnobCW }

func (c Control) isPress() bool {
	return c == Key1 || c == Key2 || c == Key3 || c == KnobPress
}

func (c Control) isKey() bool { return c == Key1 || c == Key2 || c == Key3 }

// Input is one press or release of a control. Time is the kernel event time.
type Input struct {
	Control Control
	Down    bool
	Time    time.Time
}

// Gesture is a recognised gesture. Time is the moment it was decided (the
// input time, or the deadline that resolved it).
type Gesture struct {
	Control Control
	Kind    Kind
	Time    time.Time
}

func (g Gesture) String() string { return string(g.Control) + " " + string(g.Kind) }

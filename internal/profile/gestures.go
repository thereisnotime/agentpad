package profile

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Gesture control names (as seen by the user, after orientation).
const (
	CtrlKey1      = "key1"
	CtrlKey2      = "key2"
	CtrlKey3      = "key3"
	CtrlKnobCCW   = "knob-ccw"
	CtrlKnobPress = "knob-press"
	CtrlKnobCW    = "knob-cw"
)

// Gesture kind names.
const (
	KindTap         = "tap"
	KindHold        = "hold"
	KindLongHold    = "longhold"
	KindDoubleTap   = "doubletap"
	KindRotate      = "rotate"
	KindPressRotate = "pressrotate"
	KindChord       = "chord"
)

var (
	pressKinds  = []string{KindTap, KindHold, KindLongHold, KindDoubleTap}
	rotateKinds = []string{KindRotate, KindPressRotate}
	chordKeys   = []string{CtrlKey1, CtrlKey2, CtrlKey3}
)

// GestureControls returns the single-control names in display order.
// Chords use "keyA+keyB" (A < B), see ChordName.
func GestureControls() []string {
	return []string{CtrlKey1, CtrlKey2, CtrlKey3, CtrlKnobCCW, CtrlKnobPress, CtrlKnobCW}
}

// ChordNames returns every valid chord control name ("key1+key2", ...).
func ChordNames() []string {
	var out []string
	for i, a := range chordKeys {
		for _, b := range chordKeys[i+1:] {
			out = append(out, a+"+"+b)
		}
	}
	return out
}

// ChordName returns the canonical chord control for two keys, or "" if the
// pair cannot form a chord (only key1..key3, two different keys).
func ChordName(a, b string) string {
	if a == b || !slices.Contains(chordKeys, a) || !slices.Contains(chordKeys, b) {
		return ""
	}
	if b < a {
		a, b = b, a
	}
	return a + "+" + b
}

// GestureKinds returns the kinds valid for control (nil if control is unknown).
func GestureKinds(control string) []string {
	switch {
	case control == CtrlKnobCCW || control == CtrlKnobCW:
		return slices.Clone(rotateKinds)
	case control == CtrlKnobPress || slices.Contains(chordKeys, control):
		return slices.Clone(pressKinds)
	case slices.Contains(ChordNames(), control):
		return []string{KindChord}
	}
	return nil
}

// GestureAction returns the action for control/kind. Without an explicit
// gesture entry, a tap falls back to the plain key / knob press action and a
// rotate to the plain knob action, so plain profiles keep working in gesture
// mode. ok is false when nothing is mapped.
func (p Profile) GestureAction(control, kind string) (action string, ok bool) {
	if a := p.Gestures[control][kind]; a != "" {
		return a, true
	}
	switch {
	case kind == KindTap && control == CtrlKey1:
		action = p.Keys[0]
	case kind == KindTap && control == CtrlKey2:
		action = p.Keys[1]
	case kind == KindTap && control == CtrlKey3:
		action = p.Keys[2]
	case kind == KindTap && control == CtrlKnobPress:
		action = p.Knob.Press
	case kind == KindRotate && control == CtrlKnobCCW:
		action = p.Knob.CCW
	case kind == KindRotate && control == CtrlKnobCW:
		action = p.Knob.CW
	}
	return action, action != ""
}

func validateGestures(g map[string]map[string]string) error {
	controls := make([]string, 0, len(g))
	for c := range g {
		controls = append(controls, c)
	}
	sort.Strings(controls) // stable error messages
	for _, c := range controls {
		kinds := GestureKinds(c)
		if kinds == nil {
			return fmt.Errorf("gesture %q: unknown control (want one of %s or a chord like key1+key3)",
				c, strings.Join(GestureControls(), ", "))
		}
		names := make([]string, 0, len(g[c]))
		for k := range g[c] {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if !slices.Contains(kinds, k) {
				return fmt.Errorf("gesture %s %s: kind not valid here (want %s)", c, k, strings.Join(kinds, ", "))
			}
			if err := validateGestureAction(g[c][k]); err != nil {
				return fmt.Errorf("gesture %s %s: %w", c, k, err)
			}
		}
	}
	return nil
}

// ErrMouseInGesture is returned for mouse actions in gesture entries; the
// virtual keyboard used in gesture mode cannot emit them.
var ErrMouseInGesture = errors.New("mouse actions are not supported in gesture mode")

func validateGestureAction(a string) error {
	if err := validateAction(a); err != nil {
		return err
	}
	for _, part := range splitSequence(a) {
		if isMouse(stripModifiers(part)) {
			return ErrMouseInGesture
		}
	}
	return nil
}

// Gesture-mode keys: every control sends one otherwise unused key.
// f13..f18 are accepted by ch57x-keyboard-tool v1.7.0 (`show-keys`) and
// reach Linux as KEY_F13..KEY_F18.
const (
	GestureKey1      = "f13"
	GestureKey2      = "f14"
	GestureKey3      = "f15"
	GestureKnobCCW   = "f16"
	GestureKnobPress = "f17"
	GestureKnobCW    = "f18"
)

// GestureModeProfile is the profile programmed while gesture mode is on.
func GestureModeProfile() Profile {
	return Profile{
		Name:        "gesture-mode",
		Description: "agentpad gesture mode: each control sends a unique key that agentpad reads",
		Keys:        [3]string{GestureKey1, GestureKey2, GestureKey3},
		Knob:        Knob{CCW: GestureKnobCCW, Press: GestureKnobPress, CW: GestureKnobCW},
	}
}

// GestureModeMapping renders the gesture-mode mapping for orientation o.
// All three layers are identical so a layer switch on the pad cannot break
// gesture mode. Keys are "as seen", exactly like MappingYAMLOriented.
func GestureModeMapping(o Orientation) ([]byte, error) {
	g := GestureModeProfile()
	return MappingYAMLOriented([]Profile{g, g, g}, o)
}

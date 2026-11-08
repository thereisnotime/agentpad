package gesture

import (
	"fmt"
	"slices"

	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/vkbd"
)

// KeyMap maps Linux keycodes from the pad to controls.
type KeyMap map[uint16]Control

// Collision reports controls whose actions produce the same keycode. The
// first control in GestureControls order wins; Others are not observable.
type Collision struct {
	Code   uint16
	Winner Control
	Others []Control
}

func (c Collision) String() string {
	return fmt.Sprintf("keycode %d is sent by %s and %v; it is shown as %s", c.Code, c.Winner, c.Others, c.Winner)
}

// KeyMapReport explains what a plain-mode KeyMap cannot tell apart.
type KeyMapReport struct {
	Collisions []Collision
	// Unmapped controls send nothing a keyboard node reports as a key
	// (mouse actions, unknown custom codes, empty actions).
	Unmapped []Control
	// Expected holds every keycode the profile's actions can produce,
	// modifiers and sequence keys included. A key outside this set means the
	// pad holds a different mapping than the profile.
	Expected map[uint16]bool
}

// Ambiguous reports whether some controls cannot be observed reliably.
func (r KeyMapReport) Ambiguous() bool { return len(r.Collisions) > 0 || len(r.Unmapped) > 0 }

// GestureKeyMap is the unambiguous F13..F18 map used in gesture mode.
func GestureKeyMap() KeyMap {
	m := make(KeyMap, len(keyControls))
	for k, c := range keyControls {
		m[k] = c
	}
	return m
}

var plainOrder = []Control{Key1, Key2, Key3, KnobCCW, KnobPress, KnobCW}

// PlainKeyMap builds the keycode -> control map for a pad programmed in
// plain mode with p in orientation o. The mapping is uploaded "as seen"
// (profile.MappingYAMLOriented), so Keys[0] is key 1 as seen in every
// orientation and o only needs to be valid. Each control is identified by
// the main key of its action: the key of the first step ("ctrl-c" -> C,
// "a,b" -> A), or the first modifier of a bare modifier combo.
// Collisions resolve to the first control in key1, key2, key3, knob-ccw,
// knob-press, knob-cw order.
//
// run:<name> controls are mapped by the spare key plain mode programs for
// them (profile.PlainLayers) as if p were layer 1; for layers 2 and 3 pass
// the profiles already translated by profile.PlainLayers over all layers.
func PlainKeyMap(p profile.Profile, o profile.Orientation) (KeyMap, KeyMapReport, error) {
	if !slices.Contains(profile.Orientations(), o) {
		return nil, KeyMapReport{}, fmt.Errorf("unknown orientation %q", string(o))
	}
	p = profile.PlainLayers([]profile.Profile{p})[0]
	actions := map[Control]string{
		Key1: p.Keys[0], Key2: p.Keys[1], Key3: p.Keys[2],
		KnobCCW: p.Knob.CCW, KnobPress: p.Knob.Press, KnobCW: p.Knob.CW,
	}
	m := KeyMap{}
	rep := KeyMapReport{Expected: map[uint16]bool{}}
	for _, a := range actions {
		combos, err := vkbd.Translate(a)
		if err != nil {
			continue
		}
		for _, c := range combos {
			for _, mod := range c.Mods {
				rep.Expected[mod] = true
			}
			if c.Key != 0 {
				rep.Expected[c.Key] = true
			}
		}
	}
	byCode := map[uint16]int{} // code -> index in rep.Collisions, -1 if none yet
	for _, c := range plainOrder {
		code, ok := mainKey(actions[c])
		if !ok {
			rep.Unmapped = append(rep.Unmapped, c)
			continue
		}
		winner, taken := m[code]
		if !taken {
			m[code] = c
			byCode[code] = -1
			continue
		}
		if i := byCode[code]; i >= 0 {
			rep.Collisions[i].Others = append(rep.Collisions[i].Others, c)
		} else {
			byCode[code] = len(rep.Collisions)
			rep.Collisions = append(rep.Collisions, Collision{Code: code, Winner: winner, Others: []Control{c}})
		}
	}
	return m, rep, nil
}

func mainKey(action string) (uint16, bool) {
	combos, err := vkbd.Translate(action)
	if err != nil || len(combos) == 0 {
		return 0, false
	}
	c := combos[0]
	if c.Key != 0 {
		return c.Key, true
	}
	if len(c.Mods) > 0 {
		return c.Mods[0], true
	}
	return 0, false
}

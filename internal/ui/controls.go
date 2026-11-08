package ui

import (
	"slices"
	"strings"

	"github.com/thereisnotime/agentpad/internal/profile"
)

// controlID names one physical control of the pad.
type controlID int

const (
	ctlKey1 controlID = iota
	ctlKey2
	ctlKey3
	ctlKnobCCW
	ctlKnobPress
	ctlKnobCW
)

var allControls = []controlID{ctlKey1, ctlKey2, ctlKey3, ctlKnobCCW, ctlKnobPress, ctlKnobCW}

// title is the inspector heading for c.
func (c controlID) title() string {
	switch c {
	case ctlKey1:
		return "Key 1"
	case ctlKey2:
		return "Key 2"
	case ctlKey3:
		return "Key 3"
	case ctlKnobCCW:
		return "Knob · turn left"
	case ctlKnobPress:
		return "Knob · press"
	case ctlKnobCW:
		return "Knob · turn right"
	}
	return ""
}

// name is c's control name in profiles (key1, knob-ccw, ...).
func (c controlID) name() string {
	return [...]string{profile.CtrlKey1, profile.CtrlKey2, profile.CtrlKey3,
		profile.CtrlKnobCCW, profile.CtrlKnobPress, profile.CtrlKnobCW}[c]
}

// actionSlot is one editable action of a control: the plain action, or in
// gesture mode one gesture kind of the control or of a chord.
type actionSlot struct {
	ID          string // slotAction, or "g:<control>:<kind>"
	Label       string
	Placeholder string
	Section     string // heading shown above the first slot of a section
}

const slotAction = "action"

// Inspector sections.
const (
	sectionGestures = "Gestures (while agentpad runs)"
	sectionChords   = "Chords"
)

// gestureSlotID is the slot ID of kind on control (a control name or chord).
func gestureSlotID(control, kind string) string { return "g:" + control + ":" + kind }

// parseGestureSlot splits a gesture slot ID; ok is false for other slots.
func parseGestureSlot(id string) (control, kind string, ok bool) {
	rest, found := strings.CutPrefix(id, "g:")
	if !found {
		return "", "", false
	}
	control, kind, ok = strings.Cut(rest, ":")
	return control, kind, ok && control != "" && kind != ""
}

// kindLabel is the inspector label of a gesture kind.
func kindLabel(kind string) string {
	switch kind {
	case profile.KindTap:
		return "Tap"
	case profile.KindHold:
		return "Hold"
	case profile.KindLongHold:
		return "Long hold"
	case profile.KindDoubleTap:
		return "Double-tap"
	case profile.KindRotate:
		return "Turn"
	case profile.KindPressRotate:
		return "Press + turn"
	case profile.KindChord:
		return "Chord"
	}
	return kind
}

// controlLabel is a short human name for a control or chord name.
func controlLabel(name string) string {
	switch name {
	case profile.CtrlKey1, profile.CtrlKey2, profile.CtrlKey3:
		return "key " + strings.TrimPrefix(name, "key")
	case profile.CtrlKnobCCW:
		return "knob left"
	case profile.CtrlKnobPress:
		return "knob press"
	case profile.CtrlKnobCW:
		return "knob right"
	}
	if a, b, ok := strings.Cut(name, "+"); ok {
		return "keys " + strings.TrimPrefix(a, "key") + "+" + strings.TrimPrefix(b, "key")
	}
	return name
}

// notUsed is the placeholder of an empty gesture slot.
const notUsed = "not used"

// slotsFor lists the editable actions of c. Plain mode has one slot; gesture
// mode adds one per gesture kind of c and the three chords. Empty gesture
// slots mean "not used", except tap and turn, which fall back to the plain
// action (shown as the placeholder).
func slotsFor(c controlID, p profile.Profile, gestures bool) []actionSlot {
	out := []actionSlot{{ID: slotAction, Label: "Sends", Placeholder: "key, combo (ctrl-c) or media key"}}
	if !gestures {
		return out
	}
	out[0].Label = "Sends (plain mode)"
	plain := getAction(p, c, slotAction)
	for i, kind := range profile.GestureKinds(c.name()) {
		s := actionSlot{ID: gestureSlotID(c.name(), kind), Label: kindLabel(kind), Placeholder: notUsed}
		if (kind == profile.KindTap || kind == profile.KindRotate) && plain != "" {
			s.Placeholder = plain + " (plain action)"
		}
		if i == 0 {
			s.Section = sectionGestures
		}
		out = append(out, s)
	}
	for i, chord := range profile.ChordNames() {
		s := actionSlot{ID: gestureSlotID(chord, profile.KindChord), Label: "Keys " + strings.ReplaceAll(strings.ReplaceAll(chord, "key", ""), "+", " + "), Placeholder: notUsed}
		if i == 0 {
			s.Section = sectionChords
		}
		out = append(out, s)
	}
	return out
}

// gestureOptions are the suggestions for gesture slots: keys and media keys
// (no mouse actions) plus the reserved agentpad actions.
func gestureOptions() []string {
	var out []string
	for _, k := range profile.SupportedKeys() {
		if strings.ContainsAny(k, "(") || slices.Contains([]string{"click", "rclick", "mclick", "wheelup", "wheeldown"}, k) {
			continue
		}
		out = append(out, k)
	}
	return append(out, profile.AgentpadActions()...)
}

// getAction returns the action of slot on control c in p. Gesture slots
// return only explicit gesture actions (no fallback).
func getAction(p profile.Profile, c controlID, slot string) string {
	if control, kind, ok := parseGestureSlot(slot); ok {
		return p.Gestures[control][kind]
	}
	if slot != slotAction {
		return ""
	}
	switch c {
	case ctlKey1, ctlKey2, ctlKey3:
		return p.Keys[int(c-ctlKey1)]
	case ctlKnobCCW:
		return p.Knob.CCW
	case ctlKnobPress:
		return p.Knob.Press
	case ctlKnobCW:
		return p.Knob.CW
	}
	return ""
}

// setAction returns p with slot on control c set to v (trimmed).
func setAction(p profile.Profile, c controlID, slot, v string) profile.Profile {
	v = strings.TrimSpace(v)
	if control, kind, ok := parseGestureSlot(slot); ok {
		return setGesture(p, control, kind, v)
	}
	if slot != slotAction {
		return p
	}
	switch c {
	case ctlKey1, ctlKey2, ctlKey3:
		p.Keys[int(c-ctlKey1)] = v
	case ctlKnobCCW:
		p.Knob.CCW = v
	case ctlKnobPress:
		p.Knob.Press = v
	case ctlKnobCW:
		p.Knob.CW = v
	}
	return p
}

// setGesture returns p with the gesture action set (v == "" removes it). The
// gestures map is copied, so p's original map is never modified, and empty
// maps are dropped so an untouched profile compares equal to the stored one.
func setGesture(p profile.Profile, control, kind, v string) profile.Profile {
	g := make(map[string]map[string]string, len(p.Gestures)+1)
	for c, kinds := range p.Gestures {
		cp := make(map[string]string, len(kinds))
		for k, a := range kinds {
			cp[k] = a
		}
		g[c] = cp
	}
	if v == "" {
		delete(g[control], kind)
		if len(g[control]) == 0 {
			delete(g, control)
		}
	} else {
		if g[control] == nil {
			g[control] = map[string]string{}
		}
		g[control][kind] = v
	}
	if len(g) == 0 {
		g = nil
	}
	p.Gestures = g
	return p
}

// quickPicks are the common actions offered as one-tap chips.
var quickPicks = []string{"1", "2", "3", "y", "a", "n", "enter", "escape", "up", "down", "volumeup", "volumedown", "mute", "play"}

// capLabel shortens an action for a keycap: at most n runes, with an ellipsis.
func capLabel(action string, n int) string {
	if action == "" {
		return "—"
	}
	r := []rune(action)
	if n < 2 || len(r) <= n {
		return action
	}
	return string(r[:n-1]) + "…"
}

// editedNote explains the "edited" tag on a built-in profile.
const editedNote = "Edited · differs from the built-in, Reset restores it"

// editedBuiltin reports whether a profile gets the "edited" tag: a built-in
// with a stored override. Profiles only the user has get no tag.
func editedBuiltin(name string, overridden bool) bool {
	if !overridden {
		return false
	}
	for _, b := range profile.Builtins() {
		if b.Name == name {
			return true
		}
	}
	return false
}

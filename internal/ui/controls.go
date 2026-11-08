package ui

import (
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

// actionSlot is one editable action of a control. Today every control has a
// single "action" slot; gestures (tap / hold / double tap) can add more
// slots per control without changing the inspector.
type actionSlot struct {
	ID    string
	Label string
}

const slotAction = "action"

// slotsFor lists the editable actions of c.
func slotsFor(controlID) []actionSlot {
	return []actionSlot{{ID: slotAction, Label: "Sends"}}
}

// getAction returns the action of slot on control c in p.
func getAction(p profile.Profile, c controlID, slot string) string {
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
	if slot != slotAction {
		return p
	}
	v = strings.TrimSpace(v)
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

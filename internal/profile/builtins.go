package profile

// Builtins returns the built-in profiles in display order:
// claude-code, codex, opencode, media.
func Builtins() []Profile {
	nav := Knob{CCW: "up", Press: "enter", CW: "down"}
	return []Profile{
		{
			Name:        "claude-code",
			Description: "Claude Code permission prompt: 1 yes, 2 yes and don't ask again, Esc no",
			Keys:        [3]string{"1", "2", "escape"},
			Knob:        nav,
			// Gesture mode only (docs/gestures.md): hold key 3 to interrupt,
			// press+rotate the knob to cycle the permission mode, long-press
			// the knob to jump to the waiting agent.
			Gestures: map[string]map[string]string{
				CtrlKey3:      {KindHold: "ctrl-c"},
				CtrlKnobPress: {KindLongHold: ActionJump},
				CtrlKnobCCW:   {KindPressRotate: "shift-tab"},
				CtrlKnobCW:    {KindPressRotate: "shift-tab"},
			},
		},
		{
			Name:        "codex",
			Description: "Codex CLI approval: y approve, a always, Esc deny",
			Keys:        [3]string{"y", "a", "escape"},
			Knob:        nav,
		},
		{
			Name:        "opencode",
			Description: "OpenCode permission: Enter accept, a always, Esc reject",
			Keys:        [3]string{"enter", "a", "escape"},
			Knob:        nav,
		},
		{
			Name:        "media",
			Description: "Media controls: previous, play/pause, next; knob is volume",
			Keys:        [3]string{"prev", "play", "next"},
			Knob:        Knob{CCW: "volumedown", Press: "mute", CW: "volumeup"},
		},
	}
}

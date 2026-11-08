package profile

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Action grammar mirrors ch57x-keyboard-tool v1.7.0 for a 1-row board with one knob.

var modifierNames = []string{
	"ctrl", "shift", "alt", "opt", "win", "cmd",
	"rctrl", "rshift", "ralt", "ropt", "rwin", "rcmd",
}

var keyNames = []string{
	"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m",
	"n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
	"1", "2", "3", "4", "5", "6", "7", "8", "9", "0",
	"enter", "escape", "backspace", "tab", "space", "minus", "equal",
	"leftbracket", "rightbracket", "backslash", "nonushash", "semicolon",
	"quote", "grave", "comma", "dot", "slash", "capslock",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
	"printscreen", "macbrightnessdown", "macbrightnessup", "insert", "home",
	"pageup", "delete", "end", "pagedown", "right", "left", "down", "up",
	"numlock", "numpadslash", "numpadasterisk", "numpadminus", "numpadplus",
	"numpadenter", "numpad1", "numpad2", "numpad3", "numpad4", "numpad5",
	"numpad6", "numpad7", "numpad8", "numpad9", "numpad0", "numpaddot",
	"nonusbackslash", "application", "power", "numpadequal",
	"f13", "f14", "f15", "f16", "f17", "f18", "f19", "f20", "f21", "f22", "f23", "f24",
}

var mediaNames = []string{
	"next", "previous", "prev", "stop", "play", "mute",
	"volumeup", "volumedown", "favorites", "calculator", "screenlock",
}

var mouseExamples = []string{
	"click", "rclick", "mclick", "click(left+right)",
	"wheelup", "wheeldown", "wheel(-100)", "move(5,0)", "drag(left,0,5)",
}

// maxChord caps comma-separated key sequences. The tool accepts longer ones,
// but a pad button is meant for short shortcuts.
const maxChord = 5

var (
	modifierSet = toSet(modifierNames)
	keySet      = toSet(keyNames)
	mediaSet    = toSet(mediaNames)

	customRe = regexp.MustCompile(`^<([0-9]{1,3})>$`)
	btnsRe   = `(?:left|right|middle)(?:\+(?:left|right|middle))*`
	numRe    = `(-?[0-9]{1,4})`
	clickRe  = regexp.MustCompile(`^(?:click|rclick|mclick|click\(` + btnsRe + `\))$`)
	wheelRe  = regexp.MustCompile(`^(?:wheelup|wheeldown|wheel\(` + numRe + `\))$`)
	moveRe   = regexp.MustCompile(`^move\(` + numRe + `,` + numRe + `\)$`)
	dragRe   = regexp.MustCompile(`^drag\(` + btnsRe + `,` + numRe + `,` + numRe + `\)$`)
)

func toSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// SupportedKeys returns action names accepted in profiles (for UI suggestions).
func SupportedKeys() []string {
	out := make([]string, 0, len(keyNames)+len(mediaNames)+len(modifierNames)+len(mouseExamples))
	out = append(out, keyNames...)
	out = append(out, mediaNames...)
	out = append(out, modifierNames...)
	out = append(out, mouseExamples...)
	return out
}

var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// Validate checks every action; the error names the control, e.g. `key 2: unknown key "foo"`.
func Validate(p Profile) error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("name %q: must be non-empty and contain only a-z, 0-9 and -", p.Name)
	}
	controls := []struct{ label, action string }{
		{"key 1", p.Keys[0]},
		{"key 2", p.Keys[1]},
		{"key 3", p.Keys[2]},
		{"knob ccw", p.Knob.CCW},
		{"knob press", p.Knob.Press},
		{"knob cw", p.Knob.CW},
	}
	for _, c := range controls {
		if err := validateAction(c.action); err != nil {
			return fmt.Errorf("%s: %w", c.label, err)
		}
	}
	return nil
}

func validateAction(a string) error {
	if a == "" {
		return errors.New("empty action")
	}
	if mediaSet[a] {
		return nil
	}
	if parts := splitSequence(a); len(parts) > 1 {
		if len(parts) > maxChord {
			return fmt.Errorf("sequence %q has more than %d keys", a, maxChord)
		}
		for i, part := range parts {
			if mediaSet[part] {
				return fmt.Errorf("media key %q cannot be part of a sequence", part)
			}
			if isMouse(stripModifiers(part)) {
				return fmt.Errorf("mouse action %q cannot be part of a sequence", part)
			}
			// Only the first key in a sequence can carry modifiers on 1-row boards.
			if err := validateKeyCombo(part, i == 0); err != nil {
				return err
			}
		}
		return nil
	}
	return validateKeyCombo(a, true)
}

// stripModifiers removes a leading "mod-mod-" prefix if every element is a modifier.
func stripModifiers(s string) string {
	mods, rest, err := splitModifiers(s)
	if err != nil || len(mods) == 0 {
		return s
	}
	return rest
}

// splitModifiers splits "ctrl-shift-x" into ([ctrl shift], "x"). A bare modifier
// combo ("ctrl-shift") returns all mods and an empty rest.
func splitModifiers(s string) ([]string, string, error) {
	// Mouse arguments may contain "-" (negative numbers); split only before "(".
	head, tail := s, ""
	if i := strings.IndexByte(s, '('); i >= 0 {
		head, tail = s[:i], s[i:]
	}
	fields := strings.Split(head, "-")
	var mods []string
	for i, f := range fields {
		if modifierSet[f] {
			mods = append(mods, f)
			continue
		}
		if f == "" {
			return nil, "", fmt.Errorf("malformed action %q", s)
		}
		rest := strings.Join(fields[i:], "-") + tail
		return mods, rest, nil
	}
	if tail != "" {
		return nil, "", fmt.Errorf("malformed action %q", s)
	}
	return mods, "", nil
}

func validateKeyCombo(s string, allowMods bool) error {
	if s == "" {
		return errors.New("empty key in sequence")
	}
	mods, rest, err := splitModifiers(s)
	if err != nil {
		return err
	}
	if len(mods) > 0 && !allowMods {
		return fmt.Errorf("only the first key in a sequence can have modifiers: %q", s)
	}
	if rest == "" {
		if !allowMods {
			return fmt.Errorf("bare modifier %q not allowed here", s)
		}
		return nil // bare modifier combo
	}
	if mediaSet[rest] {
		return fmt.Errorf("media key %q cannot be combined with modifiers", rest)
	}
	if keySet[rest] {
		return nil
	}
	if m := customRe.FindStringSubmatch(rest); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 255 {
			return fmt.Errorf("custom key code %q out of range 0-255", rest)
		}
		return nil
	}
	if isMouse(rest) {
		return checkMouseRange(rest)
	}
	if strings.ContainsAny(rest, "-") && len(mods) == 0 {
		return fmt.Errorf("unknown modifier in %q", s)
	}
	return fmt.Errorf("unknown key %q", rest)
}

func isMouse(s string) bool {
	return clickRe.MatchString(s) || wheelRe.MatchString(s) || moveRe.MatchString(s) || dragRe.MatchString(s)
}

// checkMouseRange enforces the tool's signed 8-bit range for mouse arguments.
func checkMouseRange(s string) error {
	var nums []string
	switch {
	case wheelRe.MatchString(s):
		nums = wheelRe.FindStringSubmatch(s)[1:]
	case moveRe.MatchString(s):
		nums = moveRe.FindStringSubmatch(s)[1:]
	case dragRe.MatchString(s):
		nums = dragRe.FindStringSubmatch(s)[1:]
	}
	for _, n := range nums {
		if n == "" {
			continue
		}
		v, _ := strconv.Atoi(n)
		if v < -128 || v > 127 {
			return fmt.Errorf("mouse value %d in %q out of range -128..127", v, s)
		}
	}
	return nil
}

// splitSequence splits on commas that are not inside parentheses, so mouse
// actions like "move(5,0)" stay intact.
func splitSequence(a string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range a {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, a[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, a[start:])
}

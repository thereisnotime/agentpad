package vkbd

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrUnsupported is returned for actions the virtual keyboard cannot emit
// (mouse actions, unknown custom codes).
var ErrUnsupported = errors.New("unsupported in gesture mode")

// key pairs a HID keyboard usage (what ch57x-keyboard-tool's <n> means) with
// the Linux keycode (input-event-codes.h) the kernel's hid-input maps it to.
type key struct {
	hid   uint8
	linux uint16
}

// keys mirrors profile's key names (ch57x-keyboard-tool v1.7.0 `show-keys`).
var keys = map[string]key{
	"a": {0x04, 30}, "b": {0x05, 48}, "c": {0x06, 46}, "d": {0x07, 32}, "e": {0x08, 18},
	"f": {0x09, 33}, "g": {0x0a, 34}, "h": {0x0b, 35}, "i": {0x0c, 23}, "j": {0x0d, 36},
	"k": {0x0e, 37}, "l": {0x0f, 38}, "m": {0x10, 50}, "n": {0x11, 49}, "o": {0x12, 24},
	"p": {0x13, 25}, "q": {0x14, 16}, "r": {0x15, 19}, "s": {0x16, 31}, "t": {0x17, 20},
	"u": {0x18, 22}, "v": {0x19, 47}, "w": {0x1a, 17}, "x": {0x1b, 45}, "y": {0x1c, 21},
	"z": {0x1d, 44},
	"1": {0x1e, 2}, "2": {0x1f, 3}, "3": {0x20, 4}, "4": {0x21, 5}, "5": {0x22, 6},
	"6": {0x23, 7}, "7": {0x24, 8}, "8": {0x25, 9}, "9": {0x26, 10}, "0": {0x27, 11},
	"enter": {0x28, 28}, "escape": {0x29, 1}, "backspace": {0x2a, 14}, "tab": {0x2b, 15},
	"space": {0x2c, 57}, "minus": {0x2d, 12}, "equal": {0x2e, 13},
	"leftbracket": {0x2f, 26}, "rightbracket": {0x30, 27}, "backslash": {0x31, 43},
	"nonushash": {0x32, 43}, "semicolon": {0x33, 39}, "quote": {0x34, 40},
	"grave": {0x35, 41}, "comma": {0x36, 51}, "dot": {0x37, 52}, "slash": {0x38, 53},
	"capslock": {0x39, 58},
	"f1":       {0x3a, 59}, "f2": {0x3b, 60}, "f3": {0x3c, 61}, "f4": {0x3d, 62},
	"f5": {0x3e, 63}, "f6": {0x3f, 64}, "f7": {0x40, 65}, "f8": {0x41, 66},
	"f9": {0x42, 67}, "f10": {0x43, 68}, "f11": {0x44, 87}, "f12": {0x45, 88},
	"printscreen":       {0x46, 99},  // KEY_SYSRQ
	"macbrightnessdown": {0x47, 70},  // HID scroll lock -> KEY_SCROLLLOCK
	"macbrightnessup":   {0x48, 119}, // HID pause -> KEY_PAUSE
	"insert":            {0x49, 110}, "home": {0x4a, 102}, "pageup": {0x4b, 104},
	"delete": {0x4c, 111}, "end": {0x4d, 107}, "pagedown": {0x4e, 109},
	"right": {0x4f, 106}, "left": {0x50, 105}, "down": {0x51, 108}, "up": {0x52, 103},
	"numlock": {0x53, 69}, "numpadslash": {0x54, 98}, "numpadasterisk": {0x55, 55},
	"numpadminus": {0x56, 74}, "numpadplus": {0x57, 78}, "numpadenter": {0x58, 96},
	"numpad1": {0x59, 79}, "numpad2": {0x5a, 80}, "numpad3": {0x5b, 81},
	"numpad4": {0x5c, 75}, "numpad5": {0x5d, 76}, "numpad6": {0x5e, 77},
	"numpad7": {0x5f, 71}, "numpad8": {0x60, 72}, "numpad9": {0x61, 73},
	"numpad0": {0x62, 82}, "numpaddot": {0x63, 83},
	"nonusbackslash": {0x64, 86},  // KEY_102ND
	"application":    {0x65, 127}, // KEY_COMPOSE
	"power":          {0x66, 116}, "numpadequal": {0x67, 117},
	"f13": {0x68, 183}, "f14": {0x69, 184}, "f15": {0x6a, 185}, "f16": {0x6b, 186},
	"f17": {0x6c, 187}, "f18": {0x6d, 188}, "f19": {0x6e, 189}, "f20": {0x6f, 190},
	"f21": {0x70, 191}, "f22": {0x71, 192}, "f23": {0x72, 193}, "f24": {0x73, 194},
}

var modifiers = map[string]uint16{
	"ctrl": 29, "shift": 42, "alt": 56, "opt": 56, "win": 125, "cmd": 125,
	"rctrl": 97, "rshift": 54, "ralt": 100, "ropt": 100, "rwin": 126, "rcmd": 126,
}

var media = map[string]uint16{
	"next": 163, "previous": 165, "prev": 165, "stop": 166, "play": 164, "mute": 113,
	"volumeup": 115, "volumedown": 114, "favorites": 364, "calculator": 140,
	"screenlock": 152, // KEY_SCREENLOCK (= KEY_COFFEE)
}

var hidToLinux = func() map[uint8]uint16 {
	m := make(map[uint8]uint16, len(keys))
	for _, k := range keys {
		m[k.hid] = k.linux
	}
	return m
}()

var (
	customRe = regexp.MustCompile(`^<([0-9]{1,3})>$`)
	mouseRe  = regexp.MustCompile(`^(?:click|rclick|mclick|wheelup|wheeldown)$|^(?:click|wheel|move|drag)\(`)
)

// Combo is one step of an action: modifiers held while Key is tapped.
// Key is 0 for a bare modifier combo ("ctrl-shift").
type Combo struct {
	Mods []uint16
	Key  uint16
}

// Translate turns a profile action ("ctrl-c", "ctrl-a,b", "volumeup",
// "<110>") into key combos with Linux keycodes. Mouse actions return
// ErrUnsupported.
func Translate(action string) ([]Combo, error) {
	if action == "" {
		return nil, errors.New("empty action")
	}
	if code, ok := media[action]; ok {
		return []Combo{{Key: code}}, nil
	}
	var out []Combo
	for _, part := range splitSequence(action) {
		c, err := translateCombo(part)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func translateCombo(s string) (Combo, error) {
	if mouseRe.MatchString(stripMods(s)) {
		return Combo{}, fmt.Errorf("mouse action %q: %w", s, ErrUnsupported)
	}
	fields := strings.Split(s, "-")
	var c Combo
	for i, f := range fields {
		if m, ok := modifiers[f]; ok {
			c.Mods = append(c.Mods, m)
			continue
		}
		rest := strings.Join(fields[i:], "-")
		code, err := keyCode(rest)
		if err != nil {
			return Combo{}, err
		}
		c.Key = code
		return c, nil
	}
	if len(c.Mods) == 0 {
		return Combo{}, fmt.Errorf("empty key in %q", s)
	}
	return c, nil
}

func keyCode(name string) (uint16, error) {
	if k, ok := keys[name]; ok {
		return k.linux, nil
	}
	if m := customRe.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n <= 255 {
			if code, ok := hidToLinux[uint8(n)]; ok {
				return code, nil
			}
		}
		return 0, fmt.Errorf("custom key %s has no known Linux keycode: %w", name, ErrUnsupported)
	}
	if _, ok := media[name]; ok {
		return 0, fmt.Errorf("media key %q cannot be combined", name)
	}
	return 0, fmt.Errorf("unknown key %q", name)
}

func stripMods(s string) string {
	for {
		i := strings.IndexByte(s, '-')
		if i < 0 {
			return s
		}
		if _, ok := modifiers[s[:i]]; !ok {
			return s
		}
		s = s[i+1:]
	}
}

// splitSequence splits on commas outside parentheses ("a,b", "move(5,0)").
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

// KeyEvent is one key press (Down) or release in emission order.
type KeyEvent struct {
	Code uint16
	Down bool
}

// Events expands combos into presses and releases: modifiers down, key
// down+up, modifiers up in reverse order.
func Events(combos []Combo) []KeyEvent {
	var out []KeyEvent
	for _, c := range combos {
		for _, m := range c.Mods {
			out = append(out, KeyEvent{m, true})
		}
		if c.Key != 0 {
			out = append(out, KeyEvent{c.Key, true}, KeyEvent{c.Key, false})
		}
		for i := len(c.Mods) - 1; i >= 0; i-- {
			out = append(out, KeyEvent{c.Mods[i], false})
		}
	}
	return out
}

// AllCodes returns every Linux keycode Translate can produce (for uinput
// UI_SET_KEYBIT).
func AllCodes() []uint16 {
	seen := map[uint16]bool{}
	var out []uint16
	add := func(c uint16) {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, k := range keys {
		add(k.linux)
	}
	for _, c := range modifiers {
		add(c)
	}
	for _, c := range media {
		add(c)
	}
	return out
}

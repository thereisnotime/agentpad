// Package settings owns agentpad's human-editable settings.yaml: typed
// settings with defaults and validation, a comment-preserving store with live
// reload, export/import bundles with backups, and single-profile sharing.
//
// Nothing here imports Fyne; the UI reads and writes settings through Store.
package settings

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"gopkg.in/yaml.v3"
)

// Schema is the settings.yaml / bundle schema version this build writes.
const Schema = 1

// FileName is the settings file name inside the config dir.
const FileName = "settings.yaml"

// Pad input modes.
const (
	ModePlain   = "plain"
	ModeGesture = "gesture"
)

// UI themes.
const (
	ThemeSystem = "system"
	ThemeDark   = "dark"
	ThemeLight  = "light"
)

// Gesture threshold limits in milliseconds (inclusive).
const (
	minHoldMs, maxHoldMs           = 100, 5000
	maxLongHoldMs                  = 10000
	minDoubleTapMs, maxDoubleTapMs = 100, 1000
	minChordMs, maxChordMs         = 10, 500
	maxLabelLen                    = 32
)

// Settings is the content of settings.yaml (schema 1).
type Settings struct {
	Schema        int                  `yaml:"schema"`
	Pad           PadSettings          `yaml:"pad"`
	LED           LEDSettings          `yaml:"led"`
	Gestures      GestureSettings      `yaml:"gestures"`
	Notifications NotificationSettings `yaml:"notifications"`
	UI            UISettings           `yaml:"ui"`
}

// PadSettings describes the pad itself.
type PadSettings struct {
	Orientation profile.Orientation `yaml:"orientation"`
	// ActiveProfile is the profile last written to the pad ("" = none yet).
	ActiveProfile string `yaml:"active_profile"`
	Mode          string `yaml:"mode"` // ModePlain | ModeGesture
}

// LEDSettings controls how the LEDs follow agent state. Policies are kept
// per board family because the two families describe LEDs differently.
type LEDSettings struct {
	FollowAgents bool `yaml:"follow_agents"`
	// Basic is the policy for 0x8890 boards (effects by index, no colours).
	Basic LEDPolicy `yaml:"8890"`
	// PerLayer is the policy for 0x8840/0x8842 boards (named effects, colours).
	PerLayer LEDPolicy `yaml:"884x"`
	// Labels are user names for 8890 effect indexes, keyed "0".."5".
	Labels map[string]string `yaml:"labels"`
}

// LEDPolicy is what the LED shows while following agents.
type LEDPolicy struct {
	Attention LED `yaml:"attention"` // an agent needs you
	Normal    LED `yaml:"normal"`    // everything else
}

// Board families for LED policies.
const (
	FamilyBasic    = "8890"
	FamilyPerLayer = "884x"
)

// LEDFamily returns the LED family of a USB product ID: FamilyPerLayer for
// 0x8840/0x8842, FamilyBasic for everything else.
func LEDFamily(productID uint16) string {
	if pad.CapsFor(productID).PerLayer {
		return FamilyPerLayer
	}
	return FamilyBasic
}

// Policy returns the LED policy for productID's family.
func (l LEDSettings) Policy(productID uint16) LEDPolicy {
	if LEDFamily(productID) == FamilyPerLayer {
		return l.PerLayer
	}
	return l.Basic
}

// SetPolicy stores p as the LED policy for productID's family.
func (l *LEDSettings) SetPolicy(productID uint16, p LEDPolicy) {
	if LEDFamily(productID) == FamilyPerLayer {
		l.PerLayer = p
	} else {
		l.Basic = p
	}
}

// Label returns the user's name for 8890 effect n, or "".
func (l LEDSettings) Label(n int) string { return l.Labels[strconv.Itoa(n)] }

// SetLabel names 8890 effect n; an empty (or blank) label removes it.
func (l *LEDSettings) SetLabel(n int, label string) {
	label = strings.TrimSpace(label)
	if l.Labels == nil {
		l.Labels = map[string]string{}
	}
	if label == "" {
		delete(l.Labels, strconv.Itoa(n))
		return
	}
	l.Labels[strconv.Itoa(n)] = label
}

// LED is a pad.LEDSetting as written in YAML: {mode: N} for 8890 boards,
// {layer, effect, color} for 884x boards. Convert with pad.LEDSetting(l) and
// LED(s).
type LED pad.LEDSetting

// Setting returns l as a pad.LEDSetting.
func (l LED) Setting() pad.LEDSetting { return pad.LEDSetting(l) }

// MarshalYAML writes the compact flow form, e.g. {mode: 1} or
// {layer: 0, effect: backlight, color: red}.
func (l LED) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	add := func(k, v, tag string) {
		val := &yaml.Node{Kind: yaml.ScalarNode, Value: v, Tag: tag}
		if tag == "!!str" && slices.Contains([]string{"off", "on", "yes", "no", "y", "n"}, v) {
			val.Style = yaml.DoubleQuotedStyle // YAML 1.1 parsers read these as booleans
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, val)
	}
	if l.Effect == "" && l.Color == "" && l.Layer == 0 {
		add("mode", strconv.Itoa(l.Mode), "!!int")
		return n, nil
	}
	add("layer", strconv.Itoa(l.Layer), "!!int")
	if l.Mode != 0 {
		add("mode", strconv.Itoa(l.Mode), "!!int")
	}
	add("effect", l.Effect, "!!str")
	if l.Color != "" {
		add("color", l.Color, "!!str")
	}
	return n, nil
}

// GestureSettings are the gesture recogniser thresholds in milliseconds.
type GestureSettings struct {
	HoldMs        int `yaml:"hold_ms"`
	LongHoldMs    int `yaml:"long_hold_ms"`
	DoubleTapMs   int `yaml:"double_tap_ms"`
	ChordWindowMs int `yaml:"chord_window_ms"`
}

// NotificationSettings controls desktop notifications.
type NotificationSettings struct {
	Attention bool `yaml:"attention"`
}

// UISettings controls the desktop UI.
type UISettings struct {
	Theme string `yaml:"theme"` // ThemeSystem | ThemeDark | ThemeLight
}

// Defaults returns the built-in settings.
func Defaults() Settings {
	return Settings{
		Schema: Schema,
		Pad: PadSettings{
			Orientation:   profile.OrientNormal,
			ActiveProfile: "",
			Mode:          ModePlain,
		},
		LED: LEDSettings{
			FollowAgents: true,
			Basic:        LEDPolicy{Attention: LED{Mode: 1}, Normal: LED{Mode: 0}},
			PerLayer:     LEDPolicy{Attention: LED{Effect: "backlight", Color: "orange"}, Normal: LED{Effect: "off"}},
			Labels:       map[string]string{},
		},
		Gestures:      GestureSettings{HoldMs: 450, LongHoldMs: 1500, DoubleTapMs: 300, ChordWindowMs: 80},
		Notifications: NotificationSettings{Attention: true},
		UI:            UISettings{Theme: ThemeSystem},
	}
}

// Clone returns a deep copy of s (Labels is not shared).
func (s Settings) Clone() Settings {
	s.LED.Labels = maps.Clone(s.LED.Labels)
	if s.LED.Labels == nil {
		s.LED.Labels = map[string]string{}
	}
	return s
}

// FieldError is one validation problem. Path is dotted (e.g. "pad.orientation",
// "profiles.codex"); Line and Column are 1-based and 0 when unknown.
type FieldError struct {
	Path    string
	Line    int
	Column  int
	Message string
}

func (e FieldError) Error() string {
	var b strings.Builder
	if e.Line > 0 {
		fmt.Fprintf(&b, "line %d", e.Line)
		if e.Column > 0 {
			fmt.Fprintf(&b, ", column %d", e.Column)
		}
		b.WriteString(": ")
	}
	if e.Path != "" {
		b.WriteString(e.Path + ": ")
	}
	b.WriteString(e.Message)
	return b.String()
}

// ValidationError collects the problems found in one file or bundle.
type ValidationError struct {
	File   string // path or a short description, may be empty
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		parts[i] = fe.Error()
	}
	msg := strings.Join(parts, "; ")
	if e.File != "" {
		return e.File + ": " + msg
	}
	return msg
}

func oneOf(v string, allowed ...string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%q is not allowed (want %s)", v, strings.Join(allowed, " | "))
}

// Validate checks s and returns every problem (nil when valid). LED policies
// are checked against their family's capabilities (pad.ValidateLED).
func Validate(s Settings) []FieldError {
	var errs []FieldError
	add := func(path string, err error) {
		if err != nil {
			errs = append(errs, FieldError{Path: path, Message: err.Error()})
		}
	}
	if s.Schema != Schema {
		add("schema", fmt.Errorf("unsupported schema %d (this agentpad understands %d)", s.Schema, Schema))
	}
	orients := make([]string, 0, 4)
	for _, o := range profile.Orientations() {
		orients = append(orients, string(o))
	}
	add("pad.orientation", oneOf(string(s.Pad.Orientation), orients...))
	if s.Pad.ActiveProfile != "" && !profile.ValidName(s.Pad.ActiveProfile) {
		add("pad.active_profile", fmt.Errorf("%q is not a profile name (a-z, 0-9 and -)", s.Pad.ActiveProfile))
	}
	add("pad.mode", oneOf(s.Pad.Mode, ModePlain, ModeGesture))

	for _, f := range []struct {
		key string
		pid uint16
		p   LEDPolicy
	}{{FamilyBasic, 0x8890, s.LED.Basic}, {FamilyPerLayer, 0x8840, s.LED.PerLayer}} {
		caps := pad.CapsFor(f.pid)
		add("led."+f.key+".attention", pad.ValidateLED(caps, f.p.Attention.Setting()))
		add("led."+f.key+".normal", pad.ValidateLED(caps, f.p.Normal.Setting()))
	}
	for _, k := range slices.Sorted(maps.Keys(s.LED.Labels)) {
		n, err := strconv.Atoi(k)
		switch {
		case err != nil || n < 0 || n > 5 || strconv.Itoa(n) != k:
			add("led.labels."+k, fmt.Errorf("label key must be an 8890 mode index 0-5"))
		case strings.TrimSpace(s.LED.Labels[k]) == "":
			add("led.labels."+k, fmt.Errorf("label is empty"))
		case len([]rune(s.LED.Labels[k])) > maxLabelLen:
			add("led.labels."+k, fmt.Errorf("label longer than %d characters", maxLabelLen))
		}
	}

	g := s.Gestures
	rng := func(path string, v, lo, hi int) {
		if v < lo || v > hi {
			add(path, fmt.Errorf("%d out of range %d-%d", v, lo, hi))
		}
	}
	rng("gestures.hold_ms", g.HoldMs, minHoldMs, maxHoldMs)
	if g.LongHoldMs <= g.HoldMs {
		add("gestures.long_hold_ms", fmt.Errorf("%d must be greater than hold_ms (%d)", g.LongHoldMs, g.HoldMs))
	} else {
		rng("gestures.long_hold_ms", g.LongHoldMs, minHoldMs, maxLongHoldMs)
	}
	rng("gestures.double_tap_ms", g.DoubleTapMs, minDoubleTapMs, maxDoubleTapMs)
	rng("gestures.chord_window_ms", g.ChordWindowMs, minChordMs, maxChordMs)

	add("ui.theme", oneOf(s.UI.Theme, ThemeSystem, ThemeDark, ThemeLight))
	return errs
}

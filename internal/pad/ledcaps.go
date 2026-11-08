package pad

import (
	"fmt"
	"slices"
)

// LEDSetting is one LED configuration. Comparable, so it can be used for
// change detection.
//   - 8890: Mode is the firmware effect index (0 = off); Layer and Color unused.
//   - 8840/8842: Layer 0..2, Effect one of off|backlight|shock|shock2|press,
//     Color one of LEDCaps.Colors (required unless Effect is "off").
type LEDSetting struct {
	Layer  int
	Mode   int
	Effect string
	Color  string
}

// LEDMode is a selectable effect.
type LEDMode struct {
	Index int    // 8890 mode index; -1 for named effects
	ID    string // effect name for 884x ("backlight"), "mode-N" for 8890
	Label string // human label
	Color bool   // whether a colour is required
}

// LEDCaps describes what a pad model's LEDs can do.
type LEDCaps struct {
	ProductID uint16
	PerLayer  bool // settings apply per layer (884x)
	Modes     []LEDMode
	Colors    []string // empty when the model has no colour control
	// BacklightColors is Colors plus "white" (884x backlight only).
	BacklightColors []string
	Note            string // short explanation shown in the UI
}

const (
	pid8890 uint16 = 0x8890
	pid8840 uint16 = 0x8840
	pid8842 uint16 = 0x8842
)

// modes8890 is how many firmware effect indexes are offered for 8890 (0..5).
const modes8890 = 6

// layers884x is the number of LED layers on 884x boards (tool: "Layer must be 0-2").
const layers884x = 3

var colors884x = []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple"}

const (
	note8890 = "This board's firmware only offers built-in effects; colours can't be chosen."
	// ch57x-keyboard-tool v1.7.0 packs the 884x LED code at the wrong byte
	// (upstream issue #180, fixed on master after v1.7.0).
	note884x = "Effects and colours are set per layer. ch57x-keyboard-tool v1.7.0 has a known " +
		"bug on some 0x8840 boards (issue #180): colours or effects may not match; a newer tool build fixes it."
)

// CapsFor returns LED capabilities for a product ID; unknown IDs get 8890 caps.
func CapsFor(productID uint16) LEDCaps {
	switch productID {
	case pid8840, pid8842:
		return LEDCaps{
			ProductID: productID,
			PerLayer:  true,
			Modes: []LEDMode{
				{Index: -1, ID: "off", Label: "Off"},
				{Index: -1, ID: "backlight", Label: "Backlight", Color: true},
				{Index: -1, ID: "shock", Label: "Shock", Color: true},
				{Index: -1, ID: "shock2", Label: "Shock 2", Color: true},
				{Index: -1, ID: "press", Label: "Light on press", Color: true},
			},
			Colors:          slices.Clone(colors884x),
			BacklightColors: append(slices.Clone(colors884x), "white"),
			Note:            note884x,
		}
	}
	modes := make([]LEDMode, modes8890)
	for i := range modes {
		label := "Off"
		if i > 0 {
			label = fmt.Sprintf("Effect %d", i)
		}
		modes[i] = LEDMode{Index: i, ID: fmt.Sprintf("mode-%d", i), Label: label}
	}
	if productID != pid8890 {
		productID = pid8890
	}
	return LEDCaps{ProductID: productID, Modes: modes, Note: note8890}
}

// ValidateLED checks s against caps and returns an error naming the problem.
func ValidateLED(caps LEDCaps, s LEDSetting) error {
	if !caps.PerLayer {
		if s.Color != "" {
			return fmt.Errorf("led: this board has no colour control (got %q)", s.Color)
		}
		if s.Effect != "" {
			return fmt.Errorf("led: this board selects effects by index, not name (got %q)", s.Effect)
		}
		if s.Layer != 0 {
			return fmt.Errorf("led: this board has no per-layer LEDs (got layer %d)", s.Layer)
		}
		if s.Mode < 0 || s.Mode >= len(caps.Modes) {
			return fmt.Errorf("led: mode %d out of range 0-%d", s.Mode, len(caps.Modes)-1)
		}
		return nil
	}
	if s.Layer < 0 || s.Layer >= layers884x {
		return fmt.Errorf("led: layer %d out of range 0-%d", s.Layer, layers884x-1)
	}
	if s.Mode != 0 {
		return fmt.Errorf("led: this board selects effects by name, not index (got mode %d)", s.Mode)
	}
	i := slices.IndexFunc(caps.Modes, func(m LEDMode) bool { return m.ID == s.Effect })
	if i < 0 {
		return fmt.Errorf("led: unknown effect %q", s.Effect)
	}
	mode := caps.Modes[i]
	if !mode.Color {
		if s.Color != "" {
			return fmt.Errorf("led: effect %q takes no colour (got %q)", s.Effect, s.Color)
		}
		return nil
	}
	if s.Color == "" {
		return fmt.Errorf("led: effect %q needs a colour", s.Effect)
	}
	allowed := caps.Colors
	if s.Effect == "backlight" {
		allowed = caps.BacklightColors
	}
	if !slices.Contains(allowed, s.Color) {
		if s.Color == "white" {
			return fmt.Errorf("led: white is only available for backlight")
		}
		return fmt.Errorf("led: unknown colour %q", s.Color)
	}
	return nil
}

// OffSetting returns the "LEDs off" setting for a model (layer 0 for 884x).
func OffSetting(productID uint16) LEDSetting {
	if CapsFor(productID).PerLayer {
		return LEDSetting{Layer: 0, Effect: "off"}
	}
	return LEDSetting{Mode: 0}
}

// ledArgs returns the tool arguments after "led" for s on a board with caps.
// 884x expects the effect and colour as ONE argument ("backlight red"):
// `led <layer> "<effect>[ <color>]"`.
func ledArgs(caps LEDCaps, s LEDSetting) []string {
	if !caps.PerLayer {
		return []string{fmt.Sprint(s.Mode)}
	}
	mode := s.Effect
	if s.Color != "" {
		mode += " " + s.Color
	}
	return []string{fmt.Sprint(s.Layer), mode}
}

package pad

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

// Stubs: implemented by the backend work (replace bodies, keep signatures).

// CapsFor returns LED capabilities for a product ID; unknown IDs get 8890 caps.
func CapsFor(productID uint16) LEDCaps { panic("TODO") }

// ValidateLED checks s against caps and returns an error naming the problem.
func ValidateLED(caps LEDCaps, s LEDSetting) error { panic("TODO") }

// OffSetting returns the "LEDs off" setting for a model (layer 0 for 884x).
func OffSetting(productID uint16) LEDSetting { panic("TODO") }

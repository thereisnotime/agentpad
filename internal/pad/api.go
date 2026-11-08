package pad

import "time"

// Stubs: implemented by the device work (replace bodies, keep signatures).

// Detect scans /sys/bus/usb/devices for a supported pad. Returns ErrNotConnected if none.
func Detect() (Device, error) { return DetectIn("/sys/bus/usb/devices") }

// DetectIn is Detect with a custom sysfs root (for tests).
func DetectIn(sysfsRoot string) (Device, error) { panic("TODO") }

// ToolWriter implements Writer by running ch57x-keyboard-tool.
type ToolWriter struct {
	// Path to the tool; empty means look up "ch57x-keyboard-tool" on PATH.
	Path string
	// ProductID, when non-zero and not 0x8890, is passed as --product-id.
	ProductID uint16
}

// NewToolWriter returns a ToolWriter using PATH lookup.
func NewToolWriter() *ToolWriter { return &ToolWriter{} }

func (w *ToolWriter) Available() error            { panic("TODO") }
func (w *ToolWriter) Upload(mapping []byte) error { panic("TODO") }
func (w *ToolWriter) SetLED(n int) error          { panic("TODO") }

// LEDController applies LED modes change-only and at most once per interval.
// A request arriving inside the interval is deferred, and only the latest
// pending mode is applied when the interval elapses.
type LEDController struct{}

// NewLEDController wraps w. Initial mode is unknown (first Set always writes).
func NewLEDController(w Writer, minInterval time.Duration) *LEDController { panic("TODO") }

// Set requests mode n. Returns the error of an immediate write, nil if deferred or unchanged.
func (c *LEDController) Set(n int) error { panic("TODO") }

// Current returns the last successfully applied mode, or -1 if none.
func (c *LEDController) Current() int { panic("TODO") }

// Close stops any pending deferred write.
func (c *LEDController) Close() {}

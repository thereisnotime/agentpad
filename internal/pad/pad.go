// Package pad detects CH57x macro pads and writes mappings / LED modes to them.
package pad

import "errors"

// Supported USB IDs (vendor 0x1189).
const VendorID = 0x1189

var ProductIDs = []uint16{0x8890, 0x8840, 0x8842}

var (
	// ErrNotConnected means no supported pad is enumerated.
	ErrNotConnected = errors.New("pad not connected")
	// ErrPermission means the user cannot open the USB device (udev rule missing).
	ErrPermission = errors.New("permission denied opening pad (install the udev rule)")
	// ErrToolMissing means the programming backend is not installed.
	ErrToolMissing = errors.New("ch57x-keyboard-tool not found on PATH")
)

// Device describes a detected pad.
type Device struct {
	VendorID  uint16
	ProductID uint16
	BusPath   string // sysfs name, e.g. "1-4"
	Product   string // product string if available
}

// Writer programs a pad.
type Writer interface {
	// Available reports whether the backend can be used (e.g. tool on PATH).
	Available() error
	// Upload writes a ch57x-keyboard-tool mapping YAML document to the pad.
	Upload(mappingYAML []byte) error
	// SetLED applies an LED setting. Settings are validated against CapsFor first.
	SetLED(s LEDSetting) error
}

package pad

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Detect scans /sys/bus/usb/devices for a supported pad. Returns ErrNotConnected if none.
func Detect() (Device, error) { return DetectIn("/sys/bus/usb/devices") }

// DetectIn is Detect with a custom sysfs root (for tests).
func DetectIn(sysfsRoot string) (Device, error) {
	entries, err := os.ReadDir(sysfsRoot) // sorted by name
	if err != nil {
		return Device{}, ErrNotConnected
	}
	for _, e := range entries {
		dir := filepath.Join(sysfsRoot, e.Name())
		vid, ok := readHex(filepath.Join(dir, "idVendor"))
		if !ok || vid != VendorID {
			continue
		}
		pid, ok := readHex(filepath.Join(dir, "idProduct"))
		if !ok || !slices.Contains(ProductIDs, pid) {
			continue
		}
		product, _ := os.ReadFile(filepath.Join(dir, "product"))
		return Device{
			VendorID:  vid,
			ProductID: pid,
			BusPath:   e.Name(),
			Product:   strings.TrimSpace(string(product)),
		}, nil
	}
	return Device{}, ErrNotConnected
}

func readHex(path string) (uint16, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 16, 16)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}

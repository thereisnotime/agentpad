package ui

// Pure helpers with no Fyne dependency, so they can be unit tested anywhere.

import (
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// LED mode choices offered in the UI.
var ledModes = []string{"0 (off)", "1", "2"}

// ledModeIndex parses a ledModes entry back to its index; -1 if unknown.
func ledModeIndex(s string) int {
	for i, m := range ledModes {
		if m == s {
			return i
		}
	}
	return -1
}

// ledPolicy maps the aggregate agent state to an LED mode.
func ledPolicy(s events.State, attentionMode int) int {
	if s == events.StateAttention {
		return attentionMode
	}
	return 0
}

var (
	colorAttention = color.NRGBA{R: 0xf5, G: 0x7c, B: 0x00, A: 0xff}
	colorBusy      = color.NRGBA{R: 0x1e, G: 0x88, B: 0xe5, A: 0xff}
	colorIdle      = color.NRGBA{R: 0x9e, G: 0x9e, B: 0x9e, A: 0xff}
)

// stateColor returns the indicator colour of a session or aggregate state.
func stateColor(s events.State) color.NRGBA {
	switch s {
	case events.StateAttention:
		return colorAttention
	case events.StateBusy:
		return colorBusy
	default:
		return colorIdle
	}
}

// stateTitle is the human headline for an aggregate state.
func stateTitle(s events.State) string {
	switch s {
	case events.StateAttention:
		return "An agent needs you"
	case events.StateBusy:
		return "Agents working"
	default:
		return "All quiet"
	}
}

// shortenPath replaces the home directory prefix with ~.
func shortenPath(p, home string) string {
	if p == "" || home == "" {
		return p
	}
	home = filepath.Clean(home)
	clean := filepath.Clean(p)
	if clean == home {
		return "~"
	}
	if strings.HasPrefix(clean, home+string(filepath.Separator)) {
		return "~" + clean[len(home):]
	}
	return p
}

// relTime formats how long ago t was relative to now.
func relTime(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%d s ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d d ago", int(d.Hours()/24))
	}
}

// shortID trims long session ids for display.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "…"
}

// oneLine collapses whitespace so multi-line messages fit a table cell.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// writeErrMessage turns a pad write error into a user-facing sentence.
func writeErrMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, pad.ErrPermission):
		return "No permission to open the pad. Install the udev rule and replug the pad."
	case errors.Is(err, pad.ErrToolMissing):
		return "ch57x-keyboard-tool is not installed. Run `just tool-install`."
	case errors.Is(err, pad.ErrNotConnected):
		return "No pad connected. Plug it into a USB-A port (or use a C->A adapter)."
	default:
		return err.Error()
	}
}

// deviceLabel describes a detected pad.
func deviceLabel(d pad.Device) string {
	name := d.Product
	if name == "" {
		name = "CH57x pad"
	}
	return fmt.Sprintf("%s  %04x:%04x  on bus %s", name, d.VendorID, d.ProductID, d.BusPath)
}

// logColumns are the headers of the Log tab.
var logColumns = []string{"Time", "Agent", "Kind", "Tool", "Session", "Message"}

// logCell returns the text for column col of event e.
func logCell(e events.Event, col int) string {
	switch col {
	case 0:
		return e.Time.Local().Format("15:04:05")
	case 1:
		return e.Agent
	case 2:
		return string(e.Kind)
	case 3:
		return e.Tool
	case 4:
		return shortID(e.SessionID)
	case 5:
		return oneLine(e.Message)
	}
	return ""
}

// ring keeps the most recent n events in arrival order.
type ring struct {
	max    int
	events []events.Event
}

func newRing(max int) *ring { return &ring{max: max} }

func (r *ring) add(e ...events.Event) {
	r.events = append(r.events, e...)
	if over := len(r.events) - r.max; over > 0 {
		r.events = append([]events.Event(nil), r.events[over:]...)
	}
}

func (r *ring) len() int { return len(r.events) }

// newest returns the i-th most recent event (0 = latest).
func (r *ring) newest(i int) events.Event { return r.events[len(r.events)-1-i] }

func (r *ring) clear() { r.events = nil }

// profileFromControls builds a profile from the six editor fields, keeping
// name and description from base.
func profileFromControls(base profile.Profile, keys [3]string, ccw, press, cw string) profile.Profile {
	p := base
	for i := range keys {
		p.Keys[i] = strings.TrimSpace(keys[i])
	}
	p.Knob = profile.Knob{CCW: strings.TrimSpace(ccw), Press: strings.TrimSpace(press), CW: strings.TrimSpace(cw)}
	return p
}

// replaceProfile returns a copy of list with the profile named p.Name replaced by p.
func replaceProfile(list []profile.Profile, p profile.Profile) []profile.Profile {
	out := make([]profile.Profile, len(list))
	copy(out, list)
	for i := range out {
		if out[i].Name == p.Name {
			out[i] = p
		}
	}
	return out
}

// circleSVG renders a filled circle icon used for tray state.
func circleSVG(c color.NRGBA) []byte {
	return fmt.Appendf(nil,
		`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">`+
			`<circle cx="32" cy="32" r="26" fill="#%02x%02x%02x" stroke="#ffffff" stroke-width="4"/></svg>`,
		c.R, c.G, c.B)
}

// deviceWriter is a pad.Writer that targets whichever pad was last detected.
// pad.ToolWriter has a plain ProductID field, so we never mutate a shared one
// from several goroutines; each call works on a fresh copy instead.
type deviceWriter struct {
	mu   sync.Mutex
	base pad.ToolWriter
	pid  uint16
}

func newDeviceWriter(base *pad.ToolWriter) *deviceWriter {
	w := &deviceWriter{}
	if base != nil {
		w.base = *base
	}
	return w
}

func (w *deviceWriter) setProduct(pid uint16) {
	w.mu.Lock()
	w.pid = pid
	w.mu.Unlock()
}

func (w *deviceWriter) tool() *pad.ToolWriter {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.base
	if w.pid != 0 {
		t.ProductID = w.pid
	}
	return &t
}

func (w *deviceWriter) Available() error              { return w.tool().Available() }
func (w *deviceWriter) Upload(mapping []byte) error   { return w.tool().Upload(mapping) }
func (w *deviceWriter) SetLED(s pad.LEDSetting) error { return w.tool().SetLED(s) }

var _ pad.Writer = (*deviceWriter)(nil)

// plural formats a count with a naive English plural.
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

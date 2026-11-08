package ui

// Pure helpers with no Fyne dependency, so they can be unit tested anywhere.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

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

// deviceWriter is a pad.Writer that targets whichever pad was last detected.
// pad.ToolWriter has a plain ProductID field, so we never mutate a shared one
// from several goroutines; each call works on a fresh copy instead.
type deviceWriter struct {
	mu   sync.Mutex
	io   sync.Mutex
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

func (w *deviceWriter) Available() error { return w.tool().Available() }

// Upload and SetLED hold io so the tool never runs twice at once (the LED
// controller's deferred writes fire on a timer goroutine).
func (w *deviceWriter) Upload(mapping []byte) error {
	w.io.Lock()
	defer w.io.Unlock()
	return w.tool().Upload(mapping)
}

func (w *deviceWriter) SetLED(s pad.LEDSetting) error {
	w.io.Lock()
	defer w.io.Unlock()
	return w.tool().SetLED(s)
}

// product returns the last detected product ID, or 0x8890 when none was seen.
func (w *deviceWriter) product() uint16 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pid == 0 {
		return 0x8890
	}
	return w.pid
}

var _ pad.Writer = (*deviceWriter)(nil)

// plural formats a count with a naive English plural.
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

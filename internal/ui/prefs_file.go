package ui

import (
	"context"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/settings"
)

// FileSettings implements Settings on settings.yaml (*settings.Store).
//
// OnChange listeners run on the UI goroutine after the adapter's own setters
// and after external edits of the file (once Watch runs). OnInvalid
// listeners hear about errors: an invalid external edit (the store keeps the
// last good settings), or a setter that could not be saved; nil means the
// file is fine again.
type FileSettings struct {
	store *settings.Store
	post  func(func()) // fyne.Do; tests run inline

	mu        sync.Mutex
	listeners []func()
	invalid   []func(error)
	lastErr   error
}

// NewFileSettings wraps store. openErr is the error settings.Open returned
// together with a usable store (an invalid file at start), or nil.
func NewFileSettings(store *settings.Store, openErr error) *FileSettings {
	return &FileSettings{store: store, post: fyne.Do, lastErr: openErr}
}

// Store returns the underlying store (export, import, backups, profiles).
func (f *FileSettings) Store() *settings.Store { return f.store }

// Watch reloads settings.yaml on external edits until ctx is done.
func (f *FileSettings) Watch(ctx context.Context) {
	f.store.Watch(ctx, func(_ settings.Settings, err error) { f.changed(err) })
}

// changed reports the outcome of a reload or a write to the listeners.
func (f *FileSettings) changed(err error) {
	f.mu.Lock()
	wasBad := f.lastErr != nil
	f.lastErr = err
	listeners, invalid := append([]func(){}, f.listeners...), append([]func(error){}, f.invalid...)
	f.mu.Unlock()
	f.post(func() {
		if err != nil || wasBad {
			for _, fn := range invalid {
				fn(err)
			}
		}
		if err == nil {
			for _, fn := range listeners {
				fn()
			}
		}
	})
}

// OnChange implements Settings.
func (f *FileSettings) OnChange(fn func()) {
	f.mu.Lock()
	f.listeners = append(f.listeners, fn)
	f.mu.Unlock()
}

// OnInvalid registers fn for settings errors (nil: fixed). A pending error
// (e.g. the file was invalid at start) is reported right away.
func (f *FileSettings) OnInvalid(fn func(error)) {
	f.mu.Lock()
	f.invalid = append(f.invalid, fn)
	err := f.lastErr
	f.mu.Unlock()
	if err != nil {
		f.post(func() { fn(err) })
	}
}

func (f *FileSettings) get() settings.Settings { return f.store.Get() }

// update writes one change and notifies the listeners.
func (f *FileSettings) update(fn func(*settings.Settings)) {
	f.changed(f.store.Update(fn))
}

// Orientation implements Settings.
func (f *FileSettings) Orientation() profile.Orientation { return f.get().Pad.Orientation }

// SetOrientation implements Settings.
func (f *FileSettings) SetOrientation(o profile.Orientation) {
	f.update(func(s *settings.Settings) { s.Pad.Orientation = o })
}

// LEDFollow implements Settings.
func (f *FileSettings) LEDFollow() bool { return f.get().LED.FollowAgents }

// SetLEDFollow implements Settings.
func (f *FileSettings) SetLEDFollow(on bool) {
	f.update(func(s *settings.Settings) { s.LED.FollowAgents = on })
}

// LEDPolicy implements Settings. settings.yaml always holds a policy for
// both families (defaults when the file leaves it out), so it reports true.
func (f *FileSettings) LEDPolicy(pid uint16) (LEDPolicy, bool) {
	return policyFromSettings(f.get().LED.Policy(pid)), true
}

// SetLEDPolicy implements Settings.
func (f *FileSettings) SetLEDPolicy(pid uint16, p LEDPolicy) {
	f.update(func(s *settings.Settings) { s.LED.SetPolicy(pid, policyToSettings(p)) })
}

// LEDLabel implements Settings.
func (f *FileSettings) LEDLabel(n int) string { return f.get().LED.Label(n) }

// SetLEDLabel implements Settings.
func (f *FileSettings) SetLEDLabel(n int, label string) {
	f.update(func(s *settings.Settings) { s.LED.SetLabel(n, label) })
}

// ActiveProfile implements Settings.
func (f *FileSettings) ActiveProfile() string { return f.get().Pad.ActiveProfile }

// SetActiveProfile implements Settings.
func (f *FileSettings) SetActiveProfile(name string) {
	f.update(func(s *settings.Settings) { s.Pad.ActiveProfile = name })
}

// Mode implements Settings.
func (f *FileSettings) Mode() Mode { return ParseMode(f.get().Pad.Mode) }

// SetMode implements Settings.
func (f *FileSettings) SetMode(m Mode) {
	f.update(func(s *settings.Settings) { s.Pad.Mode = string(ParseMode(string(m))) })
}

// NotifyAttention implements Settings.
func (f *FileSettings) NotifyAttention() bool { return f.get().Notifications.Attention }

// SetNotifyAttention implements Settings.
func (f *FileSettings) SetNotifyAttention(on bool) {
	f.update(func(s *settings.Settings) { s.Notifications.Attention = on })
}

// GestureConfig implements Settings.
func (f *FileSettings) GestureConfig() GestureTimings {
	g := f.get().Gestures
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	return GestureTimings{Hold: ms(g.HoldMs), LongHold: ms(g.LongHoldMs), DoubleTap: ms(g.DoubleTapMs), ChordWindow: ms(g.ChordWindowMs)}
}

func policyFromSettings(p settings.LEDPolicy) LEDPolicy {
	return LEDPolicy{Attention: p.Attention.Setting(), Otherwise: p.Normal.Setting()}
}

func policyToSettings(p LEDPolicy) settings.LEDPolicy {
	return settings.LEDPolicy{Attention: settings.LED(p.Attention), Normal: settings.LED(p.Otherwise)}
}

// LegacyPrefs reads the values the Fyne-preferences implementation stored,
// keyed as settings.Store.Migrate expects. Unset keys are left out.
func LegacyPrefs(p fyne.Preferences) map[string]string {
	out := map[string]string{}
	str := func(k string) {
		if v := p.String(k); v != "" {
			out[k] = v
		}
	}
	// A bool / int is set when two different fallbacks give the same value.
	boolean := func(k string) {
		if a, b := p.BoolWithFallback(k, true), p.BoolWithFallback(k, false); a == b {
			out[k] = strconv.FormatBool(a)
		}
	}
	integer := func(k string) {
		if a, b := p.IntWithFallback(k, -1), p.IntWithFallback(k, -2); a == b {
			out[k] = strconv.Itoa(a)
		}
	}
	for _, k := range []string{prefOrientation, prefActiveProfile, prefMode, ledPolicyKey(0x8890), ledPolicyKey(0x8840)} {
		str(k)
	}
	for n := range 6 {
		str(ledLabelKey(n))
	}
	boolean(prefLEDFollow)
	boolean(prefNotify)
	for _, k := range []string{prefAttentionMode, prefGestureHold, prefGestureLongHold, prefGestureDoubleTap} {
		integer(k)
	}
	return out
}

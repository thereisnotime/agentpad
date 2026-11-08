package ui

import (
	"strings"

	"fyne.io/fyne/v2"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// Settings is every user preference the UI reads or writes. The default
// implementation uses Fyne preferences; a file-backed store (settings.yaml)
// can be passed in Deps.Settings instead.
type Settings interface {
	Orientation() profile.Orientation
	SetOrientation(profile.Orientation)

	// LEDFollow is the "LED follows agents" toggle (default on).
	LEDFollow() bool
	SetLEDFollow(bool)

	// LEDPolicy returns the stored agent LED policy for pid's model, and
	// false when none is stored (the UI then uses defaults).
	LEDPolicy(pid uint16) (LEDPolicy, bool)
	SetLEDPolicy(pid uint16, p LEDPolicy)

	// LEDLabel is the user's name for 8890 effect n ("" when unset).
	LEDLabel(n int) string
	SetLEDLabel(n int, label string)

	// ActiveProfile is the profile last applied to the pad ("" when none).
	ActiveProfile() string
	SetActiveProfile(name string)

	// OnChange registers f to run (on the UI goroutine) after any change,
	// including changes made outside the app.
	OnChange(f func())
}

// fynePrefs implements Settings on fyne.Preferences.
type fynePrefs struct{ p fyne.Preferences }

func newFynePrefs(p fyne.Preferences) Settings { return fynePrefs{p} }

func (f fynePrefs) Orientation() profile.Orientation {
	o, _ := profile.ParseOrientation(f.p.StringWithFallback(prefOrientation, string(profile.OrientNormal)))
	return o
}

func (f fynePrefs) SetOrientation(o profile.Orientation) { f.p.SetString(prefOrientation, string(o)) }

func (f fynePrefs) LEDFollow() bool      { return f.p.BoolWithFallback(prefLEDFollow, true) }
func (f fynePrefs) SetLEDFollow(on bool) { f.p.SetBool(prefLEDFollow, on) }

func (f fynePrefs) LEDPolicy(pid uint16) (LEDPolicy, bool) {
	raw := f.p.String(ledPolicyKey(pid))
	if raw == "" {
		// migrate the pre-v2 single 8890 "attention mode" preference
		if !perLayerLED(pid) {
			if n := f.p.IntWithFallback(prefAttentionMode, -1); n > 0 {
				p := defaultLEDPolicy(pid)
				p.Attention = pad.LEDSetting{Mode: n}
				return p, true
			}
		}
		return defaultLEDPolicy(pid), false
	}
	return decodeLEDPolicy(raw, pid, nil), true
}

func (f fynePrefs) SetLEDPolicy(pid uint16, p LEDPolicy) {
	f.p.SetString(ledPolicyKey(pid), encodeLEDPolicy(p))
}

func (f fynePrefs) LEDLabel(n int) string { return strings.TrimSpace(f.p.String(ledLabelKey(n))) }
func (f fynePrefs) SetLEDLabel(n int, label string) {
	f.p.SetString(ledLabelKey(n), strings.TrimSpace(label))
}

func (f fynePrefs) ActiveProfile() string        { return f.p.String(prefActiveProfile) }
func (f fynePrefs) SetActiveProfile(name string) { f.p.SetString(prefActiveProfile, name) }

func (f fynePrefs) OnChange(fn func()) { f.p.AddChangeListener(func() { fyne.Do(fn) }) }

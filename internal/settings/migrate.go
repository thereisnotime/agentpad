package settings

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// Legacy preference keys understood by Migrate. Values are strings; booleans
// accept true/false/1/0, numbers are decimal.
//
//	orientation                         pad.orientation
//	profile.active, activeProfile       pad.active_profile
//	led.follow, ledFollow               led.follow_agents
//	led.policy.8890, led.policy.884x    led.8890 / led.884x, JSON {"attention":{...},"otherwise":{...}}
//	ledPolicy                           same JSON; family picked from the content
//	led.attentionMode, attentionMode    led.8890.attention.mode (ignored when a 8890 policy is given)
//	ledLabel.N                          led.labels.N
//	theme                               ui.theme
//	mode                                pad.mode
//	notify.attention                    notifications.attention
//	gesture.holdMs                      gestures.hold_ms (0 = default, kept)
//	gesture.longholdMs                  gestures.long_hold_ms (0 = default, kept)
//	gesture.doubletapMs                 gestures.double_tap_ms (0 = default, kept)
//
// Unknown keys are ignored.
var legacyKeys = []string{
	"orientation", "profile.active", "activeProfile", "led.follow", "ledFollow",
	"led.policy.8890", "led.policy.884x", "ledPolicy", "led.attentionMode", "attentionMode",
	"ledLabel.N", "theme", "mode", "notify.attention",
	"gesture.holdMs", "gesture.longholdMs", "gesture.doubletapMs",
}

// LegacyKeys returns the preference keys Migrate understands ("ledLabel.N"
// stands for ledLabel.0 .. ledLabel.5).
func LegacyKeys() []string { return slices.Clone(legacyKeys) }

// legacyPolicy is the JSON the old UI stored (pad.LEDSetting has no JSON tags,
// encoding/json matches Layer/Mode/Effect/Color case-insensitively).
type legacyPolicy struct {
	Attention pad.LEDSetting `json:"attention"`
	Otherwise pad.LEDSetting `json:"otherwise"`
}

// Migrate applies legacy preferences (e.g. Fyne preferences) once: only when
// Open created settings.yaml and Migrate has not run yet on this Store.
// Values that don't parse or validate are skipped and listed in Warnings.
// It reports whether anything was applied.
func (s *Store) Migrate(prefs map[string]string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.created || s.migrated || len(prefs) == 0 {
		s.migrated = true
		return false, nil
	}
	s.migrated = true
	next := s.cur.Clone()
	applied := false
	try := func(key string, fn func(*Settings) error) {
		raw, ok := prefs[key]
		if !ok {
			return
		}
		cand := next.Clone()
		err := fn(&cand)
		if err == nil {
			if errs := Validate(cand); len(errs) > 0 {
				err = errs[0]
			}
		}
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("migrate %s=%q skipped: %v", key, raw, err))
			return
		}
		next, applied = cand, true
	}
	str := func(key string, set func(*Settings, string)) {
		try(key, func(c *Settings) error { set(c, strings.TrimSpace(prefs[key])); return nil })
	}
	boolean := func(key string, set func(*Settings, bool)) {
		try(key, func(c *Settings) error {
			v, err := strconv.ParseBool(strings.TrimSpace(prefs[key]))
			if err == nil {
				set(c, v)
			}
			return err
		})
	}
	policy := func(key string, family string) {
		try(key, func(c *Settings) error {
			var lp legacyPolicy
			if err := json.Unmarshal([]byte(prefs[key]), &lp); err != nil {
				return err
			}
			p := LEDPolicy{Attention: LED(lp.Attention), Normal: LED(lp.Otherwise)}
			f := family
			if f == "" {
				f = FamilyBasic
				if lp.Attention.Effect != "" || lp.Otherwise.Effect != "" {
					f = FamilyPerLayer
				}
			}
			if f == FamilyPerLayer {
				c.LED.PerLayer = p
			} else {
				c.LED.Basic = p
			}
			return nil
		})
	}

	str("orientation", func(c *Settings, v string) { c.Pad.Orientation = profile.Orientation(v) })
	str("activeProfile", func(c *Settings, v string) { c.Pad.ActiveProfile = v })
	str("profile.active", func(c *Settings, v string) { c.Pad.ActiveProfile = v })
	boolean("ledFollow", func(c *Settings, v bool) { c.LED.FollowAgents = v })
	boolean("led.follow", func(c *Settings, v bool) { c.LED.FollowAgents = v })
	_, has8890 := prefs["led.policy.8890"]
	if !has8890 {
		for _, k := range []string{"attentionMode", "led.attentionMode"} {
			try(k, func(c *Settings) error {
				n, err := strconv.Atoi(strings.TrimSpace(prefs[k]))
				if err != nil {
					return err
				}
				if n > 0 { // the old UI stored -1 / 0 for "unset"
					c.LED.Basic.Attention = LED{Mode: n}
				}
				return nil
			})
		}
	}
	policy("ledPolicy", "")
	policy("led.policy.8890", FamilyBasic)
	policy("led.policy.884x", FamilyPerLayer)
	for n := range 6 {
		key := "ledLabel." + strconv.Itoa(n)
		str(key, func(c *Settings, v string) { c.LED.SetLabel(n, v) })
	}
	str("theme", func(c *Settings, v string) { c.UI.Theme = v })
	str("mode", func(c *Settings, v string) { c.Pad.Mode = v })
	boolean("notify.attention", func(c *Settings, v bool) { c.Notifications.Attention = v })
	s.migrateGestures(prefs, try)

	if !applied {
		return false, nil
	}
	return true, s.setLocked(next)
}

// migrateGestures applies the three gesture thresholds together, so a hold
// above the default long hold still validates when both are given. 0 (the
// old UI's "default") keeps the default.
func (s *Store) migrateGestures(prefs map[string]string, try func(string, func(*Settings) error)) {
	keys := []struct {
		key string
		set func(*GestureSettings, int)
	}{
		{"gesture.holdMs", func(g *GestureSettings, v int) { g.HoldMs = v }},
		{"gesture.longholdMs", func(g *GestureSettings, v int) { g.LongHoldMs = v }},
		{"gesture.doubletapMs", func(g *GestureSettings, v int) { g.DoubleTapMs = v }},
	}
	first := ""
	for _, k := range keys {
		if _, ok := prefs[k.key]; ok {
			first = k.key
			break
		}
	}
	if first == "" {
		return
	}
	try(first, func(c *Settings) error {
		for _, k := range keys {
			raw, ok := prefs[k.key]
			if !ok {
				continue
			}
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				return fmt.Errorf("%s: %w", k.key, err)
			}
			if n > 0 {
				k.set(&c.Gestures, n)
			}
		}
		return nil
	})
}

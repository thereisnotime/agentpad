package settings

import (
	"reflect"
	"strings"
	"testing"

	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestMigrate(t *testing.T) {
	tests := []struct {
		name  string
		prefs map[string]string
		check func(Settings) bool
		warn  string
	}{
		{"ui keys", map[string]string{
			"orientation":       "counterclockwise",
			"led.follow":        "false",
			"profile.active":    "codex",
			"led.policy.8890":   `{"attention":{"Layer":0,"Mode":3,"Effect":"","Color":""},"otherwise":{"Mode":0}}`,
			"led.policy.884x":   `{"attention":{"Layer":1,"Effect":"shock","Color":"cyan"},"otherwise":{"Effect":"off"}}`,
			"ledLabel.3":        "Rainbow",
			"led.attentionMode": "5", // ignored: a 8890 policy is present
		}, func(s Settings) bool {
			return s.Pad.Orientation == profile.OrientCounterClockwise && !s.LED.FollowAgents &&
				s.Pad.ActiveProfile == "codex" && s.LED.Basic.Attention == LED{Mode: 3} &&
				s.LED.PerLayer.Attention == LED{Layer: 1, Effect: "shock", Color: "cyan"} &&
				s.LED.PerLayer.Normal == LED{Effect: "off"} && s.LED.Label(3) == "Rainbow"
		}, ""},
		{"aliases", map[string]string{
			"ledFollow": "0", "attentionMode": "2", "activeProfile": "media", "theme": "dark",
			"ledPolicy": `{"attention":{"Effect":"backlight","Color":"white"},"otherwise":{"Effect":"off"}}`,
		}, func(s Settings) bool {
			return !s.LED.FollowAgents && s.LED.Basic.Attention == LED{Mode: 2} && s.Pad.ActiveProfile == "media" &&
				s.UI.Theme == ThemeDark && s.LED.PerLayer.Attention == LED{Effect: "backlight", Color: "white"}
		}, ""},
		{"invalid values skipped", map[string]string{
			"orientation": "sideways", "led.follow": "maybe", "ledLabel.2": "Wave",
			"led.policy.8890": `{"attention":{"Mode":42}}`, "unknown.key": "x",
		}, func(s Settings) bool {
			d := Defaults()
			return s.Pad.Orientation == d.Pad.Orientation && s.LED.FollowAgents && s.LED.Label(2) == "Wave" &&
				s.LED.Basic == d.LED.Basic
		}, "migrate orientation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			s := openT(t, dir)
			ok, err := s.Migrate(tt.prefs)
			if err != nil || !ok {
				t.Fatalf("Migrate = %v, %v", ok, err)
			}
			if !tt.check(s.Get()) {
				t.Fatalf("settings = %+v", s.Get())
			}
			if w := strings.Join(s.Warnings(), "\n"); !strings.Contains(w, tt.warn) {
				t.Fatalf("warnings = %v", s.Warnings())
			}
			if !reflect.DeepEqual(openT(t, dir).Get(), s.Get()) {
				t.Fatal("migration not persisted")
			}
		})
	}
}

func TestMigrateRunsOnce(t *testing.T) {
	dir := t.TempDir()
	s := openT(t, dir)
	if ok, err := s.Migrate(map[string]string{"orientation": "clockwise"}); !ok || err != nil {
		t.Fatalf("first = %v %v", ok, err)
	}
	if ok, _ := s.Migrate(map[string]string{"orientation": "upsidedown"}); ok {
		t.Fatal("second Migrate on the same store applied")
	}
	s2 := openT(t, dir) // next start: file exists
	if ok, _ := s2.Migrate(map[string]string{"orientation": "upsidedown"}); ok {
		t.Fatal("Migrate applied with an existing file")
	}
	if s2.Get().Pad.Orientation != profile.OrientClockwise {
		t.Fatalf("orientation = %s", s2.Get().Pad.Orientation)
	}
	if len(LegacyKeys()) == 0 {
		t.Fatal("no legacy keys")
	}
}

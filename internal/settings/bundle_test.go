package settings

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
)

var zellij = profile.Profile{
	Name: "zellij-claude", Description: "claude in zellij",
	Keys: [3]string{"1", "2", "escape"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"},
	Gestures: map[string]map[string]string{"key3": {"hold": "ctrl-c"}},
}

// clockT makes s.now tick one second per call so backups get distinct names.
func clockT(s *Store) {
	t0 := time.Date(2026, 11, 8, 23, 59, 0, 0, time.UTC)
	s.now = func() time.Time { t0 = t0.Add(time.Second); return t0 }
}

// snapshot returns every file under dir except backups/.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() && rel == BackupDir {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			out[rel] = readT(t, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBundleRoundTrip(t *testing.T) {
	// source machine
	src := openT(t, t.TempDir())
	if err := src.Update(func(x *Settings) {
		x.LED.Basic.Attention = LED{Mode: 1}
		x.Pad.Orientation = profile.OrientClockwise
	}); err != nil {
		t.Fatal(err)
	}
	codex, _ := src.Profiles().Get("codex")
	codex.Keys[0] = "enter"
	for _, p := range []profile.Profile{zellij, codex} {
		if err := src.Profiles().Save(p); err != nil {
			t.Fatal(err)
		}
	}
	writeT(t, filepath.Join(src.Dir(), RulesFile), "rules:\n  - app: konsole\n    profile: codex\n")
	var buf bytes.Buffer
	if err := src.Export(&buf, ExportOptions{ExportedBy: "agentpad test"}); err != nil {
		t.Fatal(err)
	}
	b, err := ParseBundle(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, buf.String())
	}
	if b.ExportedBy != "agentpad test" || b.ExportedAt.IsZero() || len(b.Profiles) != 2 || b.Rules == nil {
		t.Fatalf("bundle = %+v", b)
	}

	var all bytes.Buffer
	if err := src.Export(&all, ExportOptions{AllProfiles: true, SkipRules: true}); err != nil {
		t.Fatal(err)
	}
	if ab, _ := ParseBundle(&all); len(ab.Profiles) != 5 || ab.Rules != nil {
		t.Fatalf("all profiles bundle: %d profiles", len(ab.Profiles))
	}

	tests := []struct {
		mode    ImportMode
		adds    int
		changes int
		removes int
		media   bool // media override survives
	}{
		// settings (a full section): pad.orientation, led.8890.attention and
		// ui.theme back to system; profiles: zellij add, codex change; rules add
		{Merge, 2, 4, 0, true},
		// replace also drops the "work" override and resets media to built-in
		{Replace, 2, 5, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			dst := openT(t, t.TempDir())
			clockT(dst)
			media, _ := dst.Profiles().Get("media")
			media.Knob.Press = "play"
			work := zellij
			work.Name = "work"
			for _, p := range []profile.Profile{media, work} {
				if err := dst.Profiles().Save(p); err != nil {
					t.Fatal(err)
				}
			}
			if err := dst.Update(func(x *Settings) { x.UI.Theme = ThemeDark }); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dst.Dir())

			plan, err := dst.PlanImport(b, tt.mode)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Count(Add) != tt.adds || plan.Count(Modify) != tt.changes || plan.Count(Remove) != tt.removes {
				t.Fatalf("plan: %v", plan.Changes)
			}
			if !reflect.DeepEqual(snapshot(t, dst.Dir()), before) {
				t.Fatal("PlanImport wrote something")
			}
			name, err := dst.Apply(plan)
			if err != nil {
				t.Fatal(err)
			}
			got := dst.Get()
			if got.LED.Basic.Attention.Mode != 1 || got.Pad.Orientation != profile.OrientClockwise {
				t.Fatalf("settings not applied: %+v", got)
			}
			if got.UI.Theme != ThemeSystem {
				t.Fatalf("theme = %s", got.UI.Theme)
			}
			if p, _ := dst.Profiles().Get("zellij-claude"); !reflect.DeepEqual(p, zellij) {
				t.Fatalf("zellij = %+v", p)
			}
			if p, _ := dst.Profiles().Get("codex"); p.Keys[0] != "enter" {
				t.Fatal("codex not updated")
			}
			if dst.Profiles().IsOverridden("media") != tt.media || dst.Profiles().IsOverridden("work") != tt.media {
				t.Fatal("replace/merge removal wrong")
			}
			if !strings.Contains(readT(t, filepath.Join(dst.Dir(), RulesFile)), "konsole") {
				t.Fatal("rules not written")
			}

			backups, err := dst.Backups()
			if err != nil || len(backups) != 1 || backups[0].Name != name {
				t.Fatalf("backups = %+v, %v", backups, err)
			}
			if !reflect.DeepEqual(backups[0].Files, []string{"profiles/media.yaml", "profiles/work.yaml", "settings.yaml"}) {
				t.Fatalf("backup files = %v", backups[0].Files)
			}

			// re-planning the same bundle is a no-op
			if again, err := dst.PlanImport(b, tt.mode); err != nil || !again.Empty() {
				t.Fatalf("second plan = %v, %v", again.Changes, err)
			}

			safety, err := dst.Restore(name)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snapshot(t, dst.Dir()), before) {
				t.Fatalf("restore mismatch:\n%v\nwant\n%v", snapshot(t, dst.Dir()), before)
			}
			if dst.Get().UI.Theme != ThemeDark || dst.Get().LED.Basic.Attention.Mode != 2 {
				t.Fatalf("store not reloaded: %+v", dst.Get())
			}
			if backups, _ := dst.Backups(); len(backups) != 2 || backups[0].Name != safety {
				t.Fatalf("safety backup missing: %+v", backups)
			}
		})
	}
}

func TestPlanImportPartialSettingsMerge(t *testing.T) {
	s := openT(t, t.TempDir())
	b, err := ParseBundle(strings.NewReader(`kind: agentpad-bundle
schema: 1
settings:
  led:
    "8890": {attention: {mode: 1}}
  future_thing: 1
profiles: []
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.PlanImport(b, Merge)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Path != "led.8890.attention" ||
		plan.Changes[0].Old != "{mode: 2}" || plan.Changes[0].New != "{mode: 1}" {
		t.Fatalf("changes = %v", plan.Changes)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "settings.future_thing") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
	if s.Get().LED.Basic.Normal.Mode != 0 {
		t.Fatal("merge touched other fields")
	}
}

func TestInvalidBundleChangesNothing(t *testing.T) {
	tests := []struct {
		name, doc string
		wantErr   string
	}{
		{"wrong kind", "kind: something\nschema: 1\n", "kind"},
		{"profile kind", "kind: agentpad-profile\nschema: 1\n", "single profile"},
		{"newer schema", "kind: agentpad-bundle\nschema: 2\n", "newer"},
		{"no schema", "kind: agentpad-bundle\n", "schema"},
		{"bad key", "kind: agentpad-bundle\nschema: 1\nsettings:\n  pad: {orientation: clockwise}\nprofiles:\n  - name: zellij-claude\n    keys: [\"1\", \"foo\", escape]\n    knob: {ccw: up, press: enter, cw: down}\n",
			"line 6, column 5: profiles.zellij-claude: key 2: unknown key \"foo\""},
		{"bad setting", "kind: agentpad-bundle\nschema: 1\nsettings:\n  pad:\n    orientation: sideways\n", "line 5, column 18: settings.pad.orientation"},
		{"bad led", "kind: agentpad-bundle\nschema: 1\nsettings:\n  led:\n    884x: {attention: {effect: glitter, color: red}}\n", "settings.led.884x.attention"},
		{"type error", "kind: agentpad-bundle\nschema: 1\nsettings:\n  gestures: {hold_ms: soon}\n", "line 4"},
		{"dup profile", "kind: agentpad-bundle\nschema: 1\nprofiles:\n  - {name: a, keys: [a, b, c], knob: {ccw: up, press: enter, cw: down}}\n  - {name: a, keys: [a, b, c], knob: {ccw: up, press: enter, cw: down}}\n", "duplicate"},
		{"profiles not list", "kind: agentpad-bundle\nschema: 1\nprofiles: {a: 1}\n", "list"},
		{"not yaml", "kind: [\n", "line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openT(t, t.TempDir())
			before := snapshot(t, s.Dir())
			b, err := ParseBundle(strings.NewReader(tt.doc))
			if err == nil {
				_, err = s.PlanImport(b, Replace)
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if !reflect.DeepEqual(snapshot(t, s.Dir()), before) {
				t.Fatal("files changed")
			}
			if bs, _ := s.Backups(); len(bs) != 0 {
				t.Fatal("backup taken for an invalid bundle")
			}
		})
	}
}

func TestApplyEmptyAndRestoreErrors(t *testing.T) {
	s := openT(t, t.TempDir())
	if name, err := s.Apply(&Plan{}); name != "" || err != nil {
		t.Fatalf("empty apply = %q, %v", name, err)
	}
	if bs, _ := s.Backups(); len(bs) != 0 {
		t.Fatal("empty apply made a backup")
	}
	for _, n := range []string{"", "../x", ".", "nope"} {
		if _, err := s.Restore(n); err == nil {
			t.Errorf("Restore(%q) succeeded", n)
		}
	}
	// a broken backup is refused before anything is touched
	writeT(t, filepath.Join(s.Dir(), BackupDir, "20260101T000000Z", FileName), "pad: {orientation: sideways}\n")
	before := snapshot(t, s.Dir())
	var ve *ValidationError
	if _, err := s.Restore("20260101T000000Z"); !errors.As(err, &ve) {
		t.Fatalf("restore broken = %v", err)
	}
	if !reflect.DeepEqual(snapshot(t, s.Dir()), before) {
		t.Fatal("broken restore changed files")
	}
}

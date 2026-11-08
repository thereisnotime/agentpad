package profile

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func runProfile(name string, keys [3]string, knob Knob) Profile {
	return Profile{Name: name, Keys: keys, Knob: knob}
}

func TestRunActionValidation(t *testing.T) {
	p := runProfile("x", [3]string{"run:deploy", "a", "b"}, Knob{CCW: "up", Press: "enter", CW: "down"})
	p.Gestures = map[string]map[string]string{CtrlKey2: {KindHold: "run:mic-off"}}

	SetScriptResolver(nil)
	if err := Validate(p); err != nil {
		t.Fatalf("no resolver: %v", err)
	}
	for _, bad := range []string{"run:", "run:Bad", "run:a b", "run:x,a"} {
		q := p
		q.Keys[0] = bad
		if Validate(q) == nil {
			t.Errorf("%q accepted", bad)
		}
	}

	known := map[string]bool{"deploy": true}
	SetScriptResolver(func(n string) bool { return known[n] })
	defer SetScriptResolver(nil)
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), `gesture key2 hold: unknown script "mic-off"`) {
		t.Fatalf("resolver: err = %v", err)
	}
	known["mic-off"] = true
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	// ValidateWith overrides the global resolver
	if err := ValidateWith(p, func(string) bool { return false }); err == nil || !strings.Contains(err.Error(), "key 1") {
		t.Errorf("ValidateWith: %v", err)
	}
	// mouse actions stay invalid in gestures
	p.Gestures[CtrlKey1] = map[string]string{KindHold: "click"}
	if err := Validate(p); err == nil {
		t.Error("mouse gesture accepted")
	}
}

func TestPlainLayersAssignsF19ToF24(t *testing.T) {
	l1 := runProfile("one", [3]string{"run:a", "x", "run:b"}, Knob{CCW: "up", Press: "run:a", CW: "down"})
	l2 := runProfile("two", [3]string{"run:c", "y", "z"}, Knob{CCW: "up", Press: "enter", CW: "down"})
	l3 := runProfile("three", [3]string{"run:d", "run:e", "1"}, Knob{CCW: "run:f", Press: "enter", CW: "down"})
	got := PlainLayers([]Profile{l1, l2, l3})
	want := [][6]string{
		{"f19", "x", "f20", "up", "f21", "down"},    // per control, first come first served
		{"f22", "y", "z", "up", "enter", "down"},    // continues over layers: unique keys
		{"f19", "f20", "1", "f21", "enter", "down"}, // 3 more don't fit: starts over
	}
	for i, p := range got {
		have := [6]string{p.Keys[0], p.Keys[1], p.Keys[2], p.Knob.CCW, p.Knob.Press, p.Knob.CW}
		if have != want[i] {
			t.Errorf("layer %d = %v, want %v", i+1, have, want[i])
		}
	}
	if l1.Keys[0] != "run:a" {
		t.Error("input modified")
	}
	sc := ScriptControls([]Profile{l1, l2})
	if sc[0][CtrlKey1] != "a" || sc[0][CtrlKey3] != "b" || sc[0][CtrlKnobPress] != "a" || sc[1][CtrlKey1] != "c" || len(sc[1]) != 1 {
		t.Errorf("ScriptControls = %v", sc)
	}
}

func TestMappingWritesScriptKeys(t *testing.T) {
	p := runProfile("one", [3]string{"run:deploy", "a", "b"}, Knob{CCW: "up", Press: "enter", CW: "run:next"})
	doc, err := MappingYAML([]Profile{p})
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Layers []struct {
			Buttons [][]string          `yaml:"buttons"`
			Knobs   []map[string]string `yaml:"knobs"`
		} `yaml:"layers"`
	}
	if err := yaml.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	if b := m.Layers[0].Buttons[0]; b[0] != "f19" || b[1] != "a" {
		t.Errorf("buttons %v", b)
	}
	if k := m.Layers[0].Knobs[0]; k["cw"] != "f20" {
		t.Errorf("knob %v", k)
	}
	if strings.Contains(string(doc), "run:") {
		t.Error("run: leaked into the mapping")
	}
}

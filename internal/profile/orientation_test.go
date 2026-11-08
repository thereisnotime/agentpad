package profile

import (
	"bytes"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestOrientationRotation(t *testing.T) {
	want := []Orientation{OrientNormal, OrientClockwise, OrientUpsideDown, OrientCounterClockwise}
	if got := Orientations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Orientations() = %v", got)
	}
	// Mutating the returned slice must not affect later calls.
	Orientations()[0] = "x"
	if Orientations()[0] != OrientNormal {
		t.Fatal("Orientations() exposes internal slice")
	}
	o := OrientNormal
	for i := range 4 {
		next := o.RotateCW()
		if next != want[(i+1)%4] {
			t.Fatalf("%s.RotateCW() = %s", o, next)
		}
		if next.RotateCCW() != o {
			t.Fatalf("%s.RotateCCW() = %s, want %s", next, next.RotateCCW(), o)
		}
		o = next
	}
	if o != OrientNormal {
		t.Fatalf("four CW turns ended at %s", o)
	}
	if OrientNormal.RotateCCW() != OrientCounterClockwise {
		t.Fatal("normal CCW should wrap to counterclockwise")
	}
	if Orientation("bogus").RotateCW() != OrientClockwise {
		t.Fatal("unknown orientation should rotate like normal")
	}
}

func TestParseOrientation(t *testing.T) {
	for _, o := range Orientations() {
		if got, ok := ParseOrientation(string(o)); !ok || got != o {
			t.Errorf("ParseOrientation(%q) = %v %v", o, got, ok)
		}
	}
	for _, s := range []string{"", "Normal", "cw", "upside-down"} {
		if got, ok := ParseOrientation(s); ok || got != OrientNormal {
			t.Errorf("ParseOrientation(%q) = %v %v", s, got, ok)
		}
	}
}

func TestOrientationPlacement(t *testing.T) {
	tests := []struct {
		o        Orientation
		vertical bool
		side     Side
		label    string
	}{
		{OrientNormal, false, SideRight, "Horizontal · knob right"},
		{OrientClockwise, true, SideBottom, "Vertical · knob bottom"},
		{OrientUpsideDown, false, SideLeft, "Horizontal · knob left"},
		{OrientCounterClockwise, true, SideTop, "Vertical · knob top"},
	}
	for _, tt := range tests {
		if tt.o.Vertical() != tt.vertical || tt.o.KnobSide() != tt.side || tt.o.Label() != tt.label {
			t.Errorf("%s: vertical=%v side=%s label=%q", tt.o, tt.o.Vertical(), tt.o.KnobSide(), tt.o.Label())
		}
	}
}

func TestMappingYAMLOriented(t *testing.T) {
	b := Builtins()
	for _, o := range Orientations() {
		got, err := MappingYAMLOriented(b[:1], o)
		if err != nil {
			t.Fatal(err)
		}
		head := "orientation: " + string(o) + "\nrows: 1\ncolumns: 3\n"
		buttons := "    - buttons:\n        - - \"1\"\n          - \"2\"\n          - \"escape\"\n"
		if o.Vertical() {
			buttons = "    - buttons:\n        - - \"1\"\n        - - \"2\"\n        - - \"escape\"\n"
		}
		want := head + "knobs: 1\nlayers:\n" + buttons +
			"      knobs:\n        - ccw: \"up\"\n          press: \"enter\"\n          cw: \"down\"\n"
		if string(got) != want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", o, got, want)
		}
	}
	normal, _ := MappingYAMLOriented(b[:3], OrientNormal)
	plain, _ := MappingYAML(b[:3])
	if !bytes.Equal(normal, plain) {
		t.Fatal("MappingYAML must equal MappingYAMLOriented(normal)")
	}
	if _, err := MappingYAMLOriented(b[:1], "sideways"); err == nil || !strings.Contains(err.Error(), "sideways") {
		t.Fatalf("unknown orientation: %v", err)
	}
	if _, err := MappingYAMLOriented(nil, OrientClockwise); err == nil {
		t.Fatal("0 layers: expected error")
	}
}

func TestMappingYAMLOrientedWithTool(t *testing.T) {
	tool, err := exec.LookPath("ch57x-keyboard-tool")
	if err != nil {
		t.Skip("ch57x-keyboard-tool not on PATH")
	}
	b := Builtins()
	for _, o := range Orientations() {
		doc, err := MappingYAMLOriented(b[:3], o)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(tool, "validate")
		cmd.Stdin = bytes.NewReader(doc)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: tool rejected mapping: %v\n%s\n%s", o, err, out, doc)
		}
	}
}

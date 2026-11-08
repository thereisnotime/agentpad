package pad

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestCapsFor(t *testing.T) {
	for _, pid := range []uint16{0, 0x8890, 0x1234} {
		c := CapsFor(pid)
		if c.ProductID != 0x8890 || c.PerLayer || len(c.Colors) != 0 || len(c.BacklightColors) != 0 || c.Note == "" {
			t.Fatalf("%#x: %+v", pid, c)
		}
		if len(c.Modes) != 3 {
			t.Fatalf("%#x: %d modes", pid, len(c.Modes))
		}
		for i, m := range c.Modes {
			if m.Index != i || m.Color || m.ID == "" || m.Label == "" {
				t.Fatalf("%#x mode %d: %+v", pid, i, m)
			}
		}
		if c.Modes[0].Label != "Off" || c.Modes[1].Label != "Key glow" || c.Modes[2].Label != "Rainbow chase" || c.Modes[1].ID != "mode-1" {
			t.Fatalf("labels: %+v", c.Modes)
		}
	}
	for _, pid := range []uint16{0x8840, 0x8842} {
		c := CapsFor(pid)
		if c.ProductID != pid || !c.PerLayer || c.Note == "" {
			t.Fatalf("%#x: %+v", pid, c)
		}
		var ids []string
		for _, m := range c.Modes {
			ids = append(ids, m.ID)
			if m.Index != -1 || m.Color != (m.ID != "off") {
				t.Fatalf("%#x: %+v", pid, m)
			}
		}
		if !slices.Equal(ids, []string{"off", "backlight", "shock", "shock2", "press"}) {
			t.Fatalf("effects = %v", ids)
		}
		if !slices.Equal(c.Colors, []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple"}) {
			t.Fatalf("colors = %v", c.Colors)
		}
		if !slices.Equal(c.BacklightColors, append(slices.Clone(c.Colors), "white")) {
			t.Fatalf("backlight colors = %v", c.BacklightColors)
		}
		// Callers must not be able to corrupt shared state.
		c.Colors[0] = "x"
		if CapsFor(pid).Colors[0] != "red" {
			t.Fatal("CapsFor shares its colour slice")
		}
	}
}

func TestValidateLED(t *testing.T) {
	c8890, c884x := CapsFor(0x8890), CapsFor(0x8842)
	tests := []struct {
		name    string
		caps    LEDCaps
		s       LEDSetting
		wantErr string // "" means valid
	}{
		{"8890 off", c8890, LEDSetting{}, ""},
		{"8890 mode 2", c8890, LEDSetting{Mode: 2}, ""},
		{"8890 mode 3", c8890, LEDSetting{Mode: 3}, "out of range"},
		{"8890 negative", c8890, LEDSetting{Mode: -1}, "out of range"},
		{"8890 colour", c8890, LEDSetting{Mode: 1, Color: "red"}, "no colour"},
		{"8890 effect", c8890, LEDSetting{Effect: "backlight"}, "by index"},
		{"8890 layer", c8890, LEDSetting{Layer: 1, Mode: 1}, "per-layer"},
		{"884x off", c884x, LEDSetting{Effect: "off"}, ""},
		{"884x off layer 2", c884x, LEDSetting{Layer: 2, Effect: "off"}, ""},
		{"884x backlight red", c884x, LEDSetting{Effect: "backlight", Color: "red"}, ""},
		{"884x backlight white", c884x, LEDSetting{Effect: "backlight", Color: "white"}, ""},
		{"884x press purple", c884x, LEDSetting{Layer: 1, Effect: "press", Color: "purple"}, ""},
		{"884x layer 3", c884x, LEDSetting{Layer: 3, Effect: "off"}, "layer 3 out of range"},
		{"884x layer -1", c884x, LEDSetting{Layer: -1, Effect: "off"}, "out of range"},
		{"884x empty effect", c884x, LEDSetting{}, "unknown effect"},
		{"884x unknown effect", c884x, LEDSetting{Effect: "rainbow"}, "unknown effect"},
		{"884x mode index", c884x, LEDSetting{Mode: 1, Effect: "off"}, "by name"},
		{"884x missing colour", c884x, LEDSetting{Effect: "shock"}, "needs a colour"},
		{"884x unknown colour", c884x, LEDSetting{Effect: "shock", Color: "black"}, "unknown colour"},
		{"884x white shock", c884x, LEDSetting{Effect: "shock", Color: "white"}, "only available for backlight"},
		{"884x off with colour", c884x, LEDSetting{Effect: "off", Color: "red"}, "takes no colour"},
	}
	for _, tt := range tests {
		err := ValidateLED(tt.caps, tt.s)
		switch {
		case tt.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected %v", tt.name, err)
		case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
			t.Errorf("%s: got %v, want %q", tt.name, err, tt.wantErr)
		}
	}
}

func TestOffSetting(t *testing.T) {
	for pid, want := range map[uint16]LEDSetting{
		0:      {},
		0x8890: {},
		0x8840: {Effect: "off"},
		0x8842: {Effect: "off"},
	} {
		got := OffSetting(pid)
		if got != want {
			t.Errorf("%#x: %+v", pid, got)
		}
		if err := ValidateLED(CapsFor(pid), got); err != nil {
			t.Errorf("%#x: off setting invalid: %v", pid, err)
		}
	}
}

func TestSetLEDRejectsBeforeExec(t *testing.T) {
	bad := map[uint16]LEDSetting{
		0:      {Mode: 1, Color: "red"},
		0x8890: {Mode: 9},
		0x8840: {Effect: "backlight"},
		0x8842: {Layer: 3, Effect: "off"},
	}
	for pid, s := range bad {
		var calls []call
		w := fakeWriter("", nil, &calls)
		w.ProductID = pid
		if err := w.SetLED(s); err == nil {
			t.Errorf("%#x %+v: expected error", pid, s)
		}
		if len(calls) != 0 {
			t.Errorf("%#x: tool ran for invalid setting: %v", pid, calls)
		}
	}
}

// TestLEDArgsWithTool checks the 884x argument grammar against the real tool's
// hidden `test-led` command, which only parses and encodes (no USB access).
func TestLEDArgsWithTool(t *testing.T) {
	tool, err := exec.LookPath(toolName)
	if err != nil {
		t.Skip("tool not installed")
	}
	caps := CapsFor(0x8842)
	var good []LEDSetting
	for layer := range 3 {
		good = append(good, LEDSetting{Layer: layer, Effect: "off"})
	}
	for _, m := range caps.Modes[1:] {
		colors := caps.Colors
		if m.ID == "backlight" {
			colors = caps.BacklightColors
		}
		for _, c := range colors {
			good = append(good, LEDSetting{Effect: m.ID, Color: c})
		}
	}
	for _, s := range good {
		args := append([]string{"--product-id", "34882", "test-led"}, ledArgs(caps, s)...)
		if out, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
			t.Errorf("%+v: tool rejected %v: %s", s, args, out)
		}
	}
	// The tool's own range check agrees with ours.
	if err := exec.Command(tool, "--product-id", "34882", "test-led", "3", "off").Run(); err == nil {
		t.Error("tool accepted layer 3")
	}
}

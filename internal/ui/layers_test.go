package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/vkbd"
)

func code(t *testing.T, key string) uint16 {
	t.Helper()
	c, err := vkbd.Translate(key)
	if err != nil || len(c) == 0 || c[0].Key == 0 {
		t.Fatalf("translate %q: %v", key, err)
	}
	return c[0].Key
}

// The built-ins as Apply writes them for claude-code: claude-code, codex, opencode.
func builtinLayers(t *testing.T) []layerMap {
	t.Helper()
	ls := profile.Layers(profile.Builtins(), "claude-code")
	if ls[0].Name != "claude-code" || ls[1].Name != "codex" || ls[2].Name != "opencode" {
		t.Fatalf("layers = %v %v %v", ls[0].Name, ls[1].Name, ls[2].Name)
	}
	return buildLayerMaps(ls, profile.OrientNormal)
}

func TestLayerGuess(t *testing.T) {
	maps := builtinLayers(t)
	g := newLayerGuess()

	// keys of layer 1 keep the guess
	g, ok := g.press(maps, code(t, "1"))
	if !ok || g.Cur != 0 || !slices.Equal(g.Cands, []int{0}) {
		t.Fatalf("1: %+v %v", g, ok)
	}

	// "a" is key 2 on codex and on opencode: ambiguous
	g, ok = g.press(maps, code(t, "a"))
	if !ok || !slices.Equal(g.Cands, []int{1, 2}) || g.Cur != 1 {
		t.Fatalf("a: %+v", g)
	}
	if li := g.info(maps); li.Text != "Pad on layer 2 or 3 · codex or opencode" || li.Short != "L2/3" {
		t.Errorf("info = %+v", li)
	}
	if c, _ := g.control(maps, code(t, "a")); c != gesture.Key2 {
		t.Errorf("a drawn on %s", c)
	}
	// escape is on every layer: still ambiguous
	if g2, _ := g.press(maps, code(t, "escape")); !slices.Equal(g2.Cands, []int{1, 2}) {
		t.Errorf("escape narrowed to %+v", g2)
	}
	// "y" only exists on codex
	g, ok = g.press(maps, code(t, "y"))
	if !ok || g.Cur != 1 || !slices.Equal(g.Cands, []int{1}) {
		t.Fatalf("y: %+v", g)
	}
	if li := g.info(maps); li.Text != "Pad on layer 2 · codex" || li.Short != "L2" {
		t.Errorf("info = %+v", li)
	}

	// enter: knob press on claude-code and codex, but key 1 on opencode
	enter := code(t, "enter")
	if c, _ := g.control(maps, enter); c != gesture.KnobPress {
		t.Errorf("codex enter drawn on %s", c)
	}
	op := layerGuess{Cur: 2, Cands: []int{2}}
	if c, _ := op.control(maps, enter); c != gesture.Key1 {
		t.Errorf("opencode enter drawn on %s", c)
	}
	// enter is explained by the current layer, so it never moves the guess
	if g2, ok := g.press(maps, enter); !ok || g2.Cur != 1 {
		t.Errorf("enter moved the guess: %+v", g2)
	}

	// a key no layer sends is unexplained and changes nothing
	g2, ok := g.press(maps, code(t, "f5"))
	if ok || g2.Cur != g.Cur {
		t.Errorf("f5: %+v %v", g2, ok)
	}

	// back on layer 1
	g, _ = g.press(maps, code(t, "2"))
	if g.Cur != 0 || g.info(maps).Short != "" || g.info(maps).Text != "Pad on layer 1 · claude-code" {
		t.Errorf("2: %+v %+v", g, g.info(maps))
	}
}

func TestLiveControllerLayers(t *testing.T) {
	var infos []layerInfo
	c := newLiveController(LiveDevices{Disabled: true}, nil, liveCallbacks{Layer: func(li layerInfo) { infos = append(infos, li) }})
	all := profile.Builtins()
	if _, err := c.SetProfile(all[0], profile.OrientNormal); err != nil {
		t.Fatal(err)
	}
	c.SetLayers(profile.Layers(all, all[0].Name))
	if ctl, ok, _ := c.mapCode(code(t, "y"), true); !ok || ctl != gesture.Key1 {
		t.Fatalf("y = %s %v", ctl, ok)
	}
	if ctl, _, _ := c.mapCode(code(t, "y"), false); ctl != gesture.Key1 {
		t.Errorf("y release = %s", ctl)
	}
	if _, ok, _ := c.mapCode(code(t, "f5"), true); ok {
		t.Error("f5 explained")
	}
	c.ResetLayer()
	if li := infos[len(infos)-1]; li.Layer != 1 || li.Short != "" {
		t.Errorf("after reset: %+v", li)
	}
	if ctl, _, _ := c.mapCode(code(t, "1"), true); ctl != gesture.Key1 {
		t.Errorf("1 = %s", ctl)
	}
}

func TestLayerShownInUI(t *testing.T) {
	u := newTestUI(t)
	u.showDevice(pad.Device{VendorID: 0x1189, ProductID: 0x8890}, nil)
	u.showLayer(layerInfo{Layer: 3, Text: "Pad on layer 3 · opencode", Short: "L3"})
	if got := u.pillPad.text.Text; got != "Pad 8890 · L3" {
		t.Errorf("pill = %q", got)
	}
	if !u.profiles.layerLb.Visible() || !strings.Contains(u.profiles.layerLb.Text, "layer 3 · opencode") {
		t.Errorf("label = %q", u.profiles.layerLb.Text)
	}
	u.profiles.setMode(ModeGesture)
	if u.profiles.layerLb.Visible() {
		t.Error("layer shown in gesture mode")
	}
	u.showLayer(layerInfo{Layer: 1, Text: "Pad on layer 1 · claude-code"})
	if got := u.pillPad.text.Text; got != "Pad 8890" {
		t.Errorf("pill = %q", got)
	}
}

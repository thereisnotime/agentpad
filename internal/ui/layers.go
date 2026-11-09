package ui

// Pad layer guessing for plain-mode live input, without Fyne. Apply writes
// the active profile as layer 1 and the next two as layers 2 and 3; the
// pad's side button switches layers without telling anyone, so agentpad
// infers the layer from the keys it sees.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// layerMap is what one pad layer sends in plain mode.
type layerMap struct {
	Name     string
	Map      gesture.KeyMap  // main keycode -> control
	Expected map[uint16]bool // every code the layer can send (modifiers too)
	// Scripts are the controls whose action is run:<name> -> script; the
	// pad sends them as F19..F24 (profile.PlainLayers).
	Scripts map[gesture.Control]string
}

// buildLayerMaps computes the plain key maps of layers (layer 1 first),
// with run:<name> controls on the spare keys Apply programs for them.
// Layers whose map can't be built are kept with empty maps.
func buildLayerMaps(layers []profile.Profile, o profile.Orientation) []layerMap {
	out := make([]layerMap, len(layers))
	programmed := profile.PlainLayers(layers)
	scripts := profile.ScriptControls(layers)
	for i, p := range programmed {
		km, rep, err := gesture.PlainKeyMap(p, o)
		if err != nil {
			km, rep.Expected = gesture.KeyMap{}, map[uint16]bool{}
		}
		sc := make(map[gesture.Control]string, len(scripts[i]))
		for c, name := range scripts[i] {
			sc[gesture.Control(c)] = name
		}
		out[i] = layerMap{Name: p.Name, Map: km, Expected: rep.Expected, Scripts: sc}
	}
	return out
}

// layerGuess is the current guess: Cur is the 0-based layer used to draw
// presses, Cands the layers still possible (Cur among them).
type layerGuess struct {
	Cur   int
	Cands []int
}

// newLayerGuess starts on layer 1 (what Apply leaves the pad on).
func newLayerGuess() layerGuess { return layerGuess{Cands: []int{0}} }

// explaining lists the layers that can produce code: those mapping it to a
// control, else those that can send it at all (a modifier, a sequence key).
func explaining(maps []layerMap, code uint16) []int {
	var s []int
	for i, m := range maps {
		if _, ok := m.Map[code]; ok {
			s = append(s, i)
		}
	}
	if len(s) > 0 {
		return s
	}
	for i, m := range maps {
		if m.Expected[code] {
			s = append(s, i)
		}
	}
	return s
}

// press updates the guess for a pressed key. explained is false when no
// layer can produce code (the pad holds some other mapping); the guess is
// then unchanged. A key the current candidates explain narrows them; one
// they don't moves the guess to the layers that do.
func (g layerGuess) press(maps []layerMap, code uint16) (next layerGuess, explained bool) {
	s := explaining(maps, code)
	if len(s) == 0 {
		return g, false
	}
	var keep []int
	for _, c := range g.Cands {
		if slices.Contains(s, c) {
			keep = append(keep, c)
		}
	}
	if len(keep) == 0 {
		keep = s
	}
	next = layerGuess{Cur: g.Cur, Cands: keep}
	if !slices.Contains(keep, g.Cur) {
		next.Cur = keep[0]
	}
	return next, true
}

// script is the script control ctl runs on the guessed layer ("" for none).
func (g layerGuess) script(maps []layerMap, ctl gesture.Control) string {
	if g.Cur >= len(maps) || ctl == "" {
		return ""
	}
	return maps[g.Cur].Scripts[ctl]
}

// control is the control code is drawn on in the guessed layer.
func (g layerGuess) control(maps []layerMap, code uint16) (gesture.Control, bool) {
	if g.Cur >= len(maps) {
		return "", false
	}
	c, ok := maps[g.Cur].Map[code]
	return c, ok
}

// layerInfo describes the guess for the UI.
type layerInfo struct {
	Layer int    // 1-based guessed layer, 0 = unknown
	Text  string // "Pad on layer 2 · codex", "Pad on layer 2 or 3 · codex or opencode"
	Short string // header pill suffix: "" on layer 1, else "L2" / "L2/3"
}

func (g layerGuess) info(maps []layerMap) layerInfo {
	if len(maps) == 0 || g.Cur >= len(maps) {
		return layerInfo{}
	}
	nums, names := make([]string, 0, len(g.Cands)), make([]string, 0, len(g.Cands))
	for _, c := range g.Cands {
		if c < len(maps) {
			nums = append(nums, fmt.Sprint(c+1))
			names = append(names, maps[c].Name)
		}
	}
	li := layerInfo{Layer: g.Cur + 1,
		Text: "Pad on layer " + strings.Join(nums, " or ") + " · " + strings.Join(names, " or ")}
	if len(g.Cands) > 1 || g.Cur != 0 {
		li.Short = "L" + strings.Join(nums, "/")
	}
	return li
}

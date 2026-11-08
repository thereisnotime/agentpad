package profile

import (
	"fmt"
	"strings"
	"sync"
)

// RunPrefix starts a script action: "run:<name>" runs the user script
// <name> from settings.yaml. In gesture mode agentpad runs it instead of
// typing; in plain mode the control is programmed to one of ScriptKeys and
// agentpad runs the script when it sees that key.
const RunPrefix = "run:"

// ScriptKeys are the spare keys plain mode programs for run:<name>
// controls, in assignment order. ch57x-keyboard-tool v1.7.0 accepts them;
// nothing binds F19..F24 by default.
var ScriptKeys = []string{"f19", "f20", "f21", "f22", "f23", "f24"}

// RunAction returns the action that runs script name.
func RunAction(name string) string { return RunPrefix + name }

// ScriptName returns the script of a run:<name> action; ok is false for
// any other action.
func ScriptName(action string) (name string, ok bool) {
	return strings.CutPrefix(action, RunPrefix)
}

var (
	resolverMu sync.RWMutex
	resolver   func(string) bool
)

// SetScriptResolver sets how Validate checks run:<name> actions: f reports
// whether a script exists. nil (the default) accepts any well-formed name,
// so tests and import plans work without settings. The UI sets it from
// settings.yaml.
func SetScriptResolver(f func(name string) bool) {
	resolverMu.Lock()
	resolver = f
	resolverMu.Unlock()
}

func currentResolver() func(string) bool {
	resolverMu.RLock()
	defer resolverMu.RUnlock()
	return resolver
}

// validateRun checks a run:<name> action.
func validateRun(a string, resolve func(string) bool) error {
	name, _ := ScriptName(a)
	if !nameRe.MatchString(name) {
		return fmt.Errorf("script name %q: must be non-empty and contain only a-z, 0-9 and -", name)
	}
	if resolve != nil && !resolve(name) {
		return fmt.Errorf("unknown script %q (define it under scripts: in settings.yaml)", name)
	}
	return nil
}

// layerControls returns pointers to the six plain actions of p in
// key1, key2, key3, knob-ccw, knob-press, knob-cw order.
func layerControls(p *Profile) []*string {
	return []*string{&p.Keys[0], &p.Keys[1], &p.Keys[2], &p.Knob.CCW, &p.Knob.Press, &p.Knob.CW}
}

// PlainLayers returns the layers as plain mode programs them: every
// run:<name> action is replaced by a key from ScriptKeys. Keys are handed
// out first come, first served over the layers in order (layer 1 first)
// and, within a layer, in key1, key2, key3, knob-ccw, knob-press, knob-cw
// order, one key per control. While keys are left, every script control on
// the pad gets its own key, so a key also tells which layer is active.
// After the sixth, assignment starts over at f19 for the next layer; the
// guessed layer then tells them apart. A single profile can never need
// more than six (it has six controls). The input is not modified.
func PlainLayers(layers []Profile) []Profile {
	out := make([]Profile, len(layers))
	next := 0
	for i, p := range layers {
		n := 0
		for _, a := range layerControls(&p) {
			if _, ok := ScriptName(*a); ok {
				n++
			}
		}
		if next+n > len(ScriptKeys) {
			next = 0 // this layer's controls must not share keys
		}
		for _, a := range layerControls(&p) {
			if _, ok := ScriptName(*a); ok {
				*a = ScriptKeys[next]
				next++
			}
		}
		out[i] = p
	}
	return out
}

// ScriptControls returns, per layer, the script each control runs in plain
// mode: control name (CtrlKey1, ...) -> script. Layers without script
// controls get an empty map.
func ScriptControls(layers []Profile) []map[string]string {
	names := GestureControls()
	out := make([]map[string]string, len(layers))
	for i := range layers {
		p := layers[i]
		out[i] = map[string]string{}
		for j, a := range layerControls(&p) {
			if name, ok := ScriptName(*a); ok {
				out[i][names[j]] = name
			}
		}
	}
	return out
}

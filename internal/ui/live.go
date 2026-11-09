package ui

// Live pad input and gesture mode, without Fyne: the liveController owns the
// plain-mode Observer, the gesture Engine and the virtual keyboard, and
// switches between the two modes. Its methods block (they open devices and
// wait for pad uploads), so the UI calls them from a background goroutine.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/thereisnotime/agentpad/internal/evdev"
	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/vkbd"
)

// LiveDevices are the OS pieces live input and gesture mode use. Zero
// fields mean the real evdev / uinput / ydotool implementations; tests pass
// fakes so nothing touches a real pad.
type LiveDevices struct {
	// Observe opens the pad without grabbing it (plain-mode live view).
	Observe gesture.Opener
	// Grab opens and grabs the pad (gesture mode).
	Grab gesture.Opener
	// NewEmitter opens the virtual keyboard used in gesture mode.
	NewEmitter func() (vkbd.Emitter, error)
	// Disabled turns plain-mode live input off (no observer is started).
	Disabled bool
}

func (d LiveDevices) withDefaults() LiveDevices {
	if d.Observe == nil {
		d.Observe = gesture.EvdevObserveSource()
	}
	if d.Grab == nil {
		d.Grab = gesture.EvdevSource()
	}
	if d.NewEmitter == nil {
		d.NewEmitter = vkbd.New
	}
	return d
}

// liveStatus is what the UI shows about live input / gesture mode.
type liveStatus struct {
	Mode    Mode
	Reading bool   // the pad's input devices are open
	Backend string // gesture mode: "uinput" or "ydotool"
	Err     error  // why input is not available (permission, not found, disconnected)
}

// liveCallbacks are called from the controller's reader goroutines; they
// must not block and must not call back into the controller.
type liveCallbacks struct {
	Input   func(PadInputEvent) // raw press/release, both modes
	Gesture func(gesture.Event) // recognised gesture, gesture mode
	Status  func(liveStatus)    // status changed
	// Agentpad handles reserved actions ("agentpad:jump") instead of typing them.
	Agentpad func(action string)
	// Script runs a user script: a run:<name> gesture action (gesture
	// mode) or a press of a run:<name> control on the guessed layer
	// (plain mode).
	Script func(name string, t scriptTrigger)
	// Unexpected reports a pressed key no pad layer can produce: the pad
	// holds a different mapping (plain mode only).
	Unexpected func(code uint16)
	// Layer reports a new guess of the pad's layer (plain mode only).
	Layer func(layerInfo)
}

// scriptTrigger is the pad input that asked for a script.
type scriptTrigger struct {
	Control string // key1, knob-cw, key1+key3, ...
	Gesture string // gesture kind; "" in plain mode
	Layer   int    // 1-based pad layer in plain mode, 0 in gesture mode
}

type liveController struct {
	// Pad layers and the guess of the one in use (plain mode).
	layerMu    sync.Mutex
	layerProfs []profile.Profile // as applied: active profile first
	layers     []layerMap
	guess      layerGuess

	dev    LiveDevices
	upload func(doc []byte) error // synchronous pad upload (through the device worker)
	cb     liveCallbacks

	mu     sync.Mutex // serialises mode changes
	mode   Mode
	prof   profile.Profile
	orient profile.Orientation
	obs    *gesture.Observer
	eng    *gesture.Engine
	em     vkbd.Emitter
	closed bool
	pumps  sync.WaitGroup

	stMu   sync.Mutex
	status liveStatus
}

func newLiveController(dev LiveDevices, upload func([]byte) error, cb liveCallbacks) *liveController {
	return &liveController{
		dev: dev.withDefaults(), upload: upload, cb: cb,
		mode: ModePlain, orient: profile.OrientNormal,
		status: liveStatus{Mode: ModePlain},
	}
}

// Status returns the current status.
func (c *liveController) Status() liveStatus {
	c.stMu.Lock()
	defer c.stMu.Unlock()
	return c.status
}

func (c *liveController) setStatus(f func(*liveStatus)) {
	c.stMu.Lock()
	f(&c.status)
	st := c.status
	c.stMu.Unlock()
	if c.cb.Status != nil {
		c.cb.Status(st)
	}
}

// Mode returns the active mode.
func (c *liveController) Mode() Mode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

// SetProfile sets the active profile and orientation. In plain mode it
// rebuilds the layer key maps live input is drawn with; in gesture mode it hands the profile to
// the engine and re-programs the gesture mapping when the orientation
// changed. It returns what the plain key map cannot tell apart.
func (c *liveController) SetProfile(p profile.Profile, o profile.Orientation) (gesture.KeyMapReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rotated := o != c.orient
	c.prof, c.orient = p, o
	_, rep, err := gesture.PlainKeyMap(p, o)
	if err != nil {
		return rep, err
	}
	c.rebuildLayers(p, o)
	if c.mode == ModeGesture {
		c.eng.SetProfile(p)
		if rotated {
			return rep, c.uploadGestureMapping()
		}
		return rep, nil
	}
	return rep, nil // the observer reports raw codes; the pump maps them per layer
}

// SetLayers sets the profiles on the pad's layers (layer 1 first), as the
// last Apply wrote them. The guess goes back to layer 1 when they change.
func (c *liveController) SetLayers(layers []profile.Profile) {
	c.layerMu.Lock()
	c.layerProfs = slices.Clone(layers)
	c.layerMu.Unlock()
	c.mu.Lock()
	p, o := c.prof, c.orient
	c.mu.Unlock()
	c.rebuildLayers(p, o)
}

// rebuildLayers recomputes the layer key maps for active profile p (which
// replaces layer 1 when it has the same name, e.g. after a save).
func (c *liveController) rebuildLayers(p profile.Profile, o profile.Orientation) {
	c.layerMu.Lock()
	ls := slices.Clone(c.layerProfs)
	switch {
	case p.Name == "": // no active profile yet: the layers as given
	case len(ls) == 0 || ls[0].Name != p.Name:
		ls = []profile.Profile{p} // active without an Apply: other layers unknown
	default:
		ls[0] = p // e.g. saved edits of the active profile
	}
	before := make([]string, len(c.layers))
	for i, m := range c.layers {
		before[i] = m.Name
	}
	c.layers = buildLayerMaps(ls, o)
	after := make([]string, len(c.layers))
	for i, m := range c.layers {
		after[i] = m.Name
	}
	changed := !slices.Equal(before, after)
	if changed {
		c.guess = newLayerGuess()
	}
	info := c.guess.info(c.layers)
	c.layerMu.Unlock()
	if changed && c.cb.Layer != nil {
		c.cb.Layer(info)
	}
}

// ResetLayer sets the guess back to layer 1 (after Apply).
func (c *liveController) ResetLayer() {
	c.layerMu.Lock()
	c.guess = newLayerGuess()
	info := c.guess.info(c.layers)
	c.layerMu.Unlock()
	if c.cb.Layer != nil {
		c.cb.Layer(info)
	}
}

// mapCode maps a raw keycode to a control on the guessed layer. On a press
// it updates the guess; explained is false when no layer sends code.
func (c *liveController) mapCode(code uint16, down bool) (ctl gesture.Control, explained bool, info *layerInfo) {
	ctl, explained, info, _, _ = c.mapCodeScript(code, down)
	return ctl, explained, info
}

// mapCodeScript is mapCode plus the script the control runs on the guessed
// layer (and that layer, 1-based).
func (c *liveController) mapCodeScript(code uint16, down bool) (ctl gesture.Control, explained bool, info *layerInfo, script string, layer int) {
	c.layerMu.Lock()
	defer c.layerMu.Unlock()
	if len(c.layers) == 0 {
		return "", true, nil, "", 0 // nothing known yet
	}
	explained = true
	if down {
		next, ok := c.guess.press(c.layers, code)
		explained = ok
		if ok && (next.Cur != c.guess.Cur || !slices.Equal(next.Cands, c.guess.Cands)) {
			li := next.info(c.layers)
			info = &li
		}
		c.guess = next
	}
	ctl, _ = c.guess.control(c.layers, code)
	return ctl, explained, info, c.guess.script(c.layers, ctl), c.guess.Cur + 1
}

// EnsureObserver starts plain-mode live input if it is not running. Called
// at start-up and again on every poll, so a pad plugged in later (or a udev
// rule installed later) is picked up.
func (c *liveController) EnsureObserver() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.mode != ModePlain || c.obs != nil || c.dev.Disabled {
		return nil
	}
	return c.startObserver()
}

// startObserver needs c.mu.
func (c *liveController) startObserver() error {
	// raw codes only: the pump maps them through the guessed layer
	obs := gesture.NewObserver(c.dev.Observe, gesture.KeyMap{})
	if err := obs.Start(context.Background()); err != nil {
		c.setStatus(func(s *liveStatus) { s.Reading, s.Err = false, err })
		return err
	}
	c.obs = obs
	c.setStatus(func(s *liveStatus) { s.Reading, s.Err = true, nil })
	c.pumps.Add(1)
	go func() {
		defer c.pumps.Done()
		for ev := range obs.Events() {
			switch ev.Status {
			case "":
				ctl, explained, info, script, layer := c.mapCodeScript(ev.Code, ev.Down)
				if info != nil && c.cb.Layer != nil {
					c.cb.Layer(*info)
				}
				if script != "" && ev.Down && c.cb.Script != nil {
					c.cb.Script(script, scriptTrigger{Control: string(ctl), Layer: layer})
				}
				if ctl == "" {
					if ev.Down && !explained && c.cb.Unexpected != nil {
						c.cb.Unexpected(ev.Code)
					}
					continue
				}
				if c.cb.Input != nil {
					c.cb.Input(PadInputEvent{Control: string(ctl), Down: ev.Down, Time: ev.Time})
				}
			case gesture.StatusRunning:
				c.setStatus(func(s *liveStatus) { s.Reading, s.Err = true, nil })
			case gesture.StatusDisconnected:
				c.setStatus(func(s *liveStatus) { s.Reading, s.Err = false, ev.Err })
			}
		}
	}()
	return nil
}

// stopObserver needs c.mu.
func (c *liveController) stopObserver() {
	if c.obs != nil {
		c.obs.Stop()
		c.obs = nil
	}
}

// EnterGesture switches to gesture mode: open the virtual keyboard, grab
// the pad and start the engine, then program the gesture mapping. On any
// error everything is undone and the controller stays in plain mode. It
// returns the virtual keyboard backend ("uinput" or "ydotool").
func (c *liveController) EnterGesture(cfg gesture.Config) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", errors.New("agentpad is shutting down")
	}
	if c.mode == ModeGesture {
		return emitterBackend(c.em), nil
	}
	em, err := c.dev.NewEmitter()
	if err != nil {
		return "", err
	}
	c.stopObserver()
	eng := gesture.NewEngine(c.dev.Grab, agentpadEmitter{inner: em, handle: c.cb.Agentpad, script: c.cb.Script}, cfg)
	eng.SetProfile(c.prof)
	if err := eng.Start(context.Background()); err != nil {
		_ = em.Close()
		c.restartObserver()
		return "", err
	}
	c.eng, c.em = eng, em
	if err := c.uploadGestureMapping(); err != nil {
		eng.Stop()
		_ = em.Close()
		c.eng, c.em = nil, nil
		c.restartObserver()
		return "", err
	}
	c.mode = ModeGesture
	backend := emitterBackend(em)
	c.setStatus(func(s *liveStatus) { *s = liveStatus{Mode: ModeGesture, Reading: true, Backend: backend} })
	c.pumps.Add(1)
	go func() {
		defer c.pumps.Done()
		for ev := range eng.Events() {
			switch {
			case ev.Input != nil:
				if c.cb.Input != nil {
					c.cb.Input(PadInputEvent{Control: string(ev.Input.Control), Down: ev.Input.Down, Time: ev.Input.Time})
				}
			case ev.IsGesture():
				if c.cb.Gesture != nil {
					c.cb.Gesture(ev)
				}
			case ev.Status == gesture.StatusRunning:
				c.setStatus(func(s *liveStatus) { s.Reading, s.Err = true, nil })
			case ev.Status == gesture.StatusDisconnected:
				c.setStatus(func(s *liveStatus) { s.Reading, s.Err = false, ev.Err })
			}
		}
	}()
	return backend, nil
}

// uploadGestureMapping needs c.mu.
func (c *liveController) uploadGestureMapping() error {
	doc, err := profile.GestureModeMapping(c.orient)
	if err != nil {
		return err
	}
	return c.upload(doc)
}

// restartObserver needs c.mu; errors only show up in the status.
func (c *liveController) restartObserver() {
	if !c.closed && !c.dev.Disabled {
		_ = c.startObserver()
	}
}

// leaveGesture stops the engine and the virtual keyboard and uploads the
// plain mapping doc. Needs c.mu.
func (c *liveController) leaveGesture(doc []byte) error {
	if c.mode != ModeGesture {
		return nil
	}
	c.eng.Stop()
	_ = c.em.Close()
	c.eng, c.em = nil, nil
	c.mode = ModePlain
	c.setStatus(func(s *liveStatus) { *s = liveStatus{Mode: ModePlain} })
	return c.upload(doc)
}

// EnterPlain leaves gesture mode: stop the engine, close the virtual
// keyboard, upload the plain mapping doc and restart live input.
func (c *liveController) EnterPlain(doc []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode != ModeGesture {
		return nil
	}
	err := c.leaveGesture(doc)
	c.restartObserver()
	return err
}

// Close stops everything. If gesture mode was on, the plain mapping doc is
// uploaded so the pad never keeps sending F13..F18 without agentpad.
func (c *liveController) Close(doc []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	err := c.leaveGesture(doc)
	c.stopObserver()
	c.pumps.Wait()
	return err
}

// emitterBackend names the virtual keyboard backend.
func emitterBackend(em vkbd.Emitter) string {
	switch em.(type) {
	case *vkbd.Uinput:
		return "uinput"
	case *vkbd.Ydotool:
		return "ydotool"
	case nil:
		return ""
	}
	return "virtual keyboard"
}

// agentpadEmitter types actions through inner, except the reserved
// "agentpad:*" actions, which go to handle, and run:<name>, which goes to
// script.
type agentpadEmitter struct {
	inner  gesture.Emitter
	handle func(action string)
	script func(name string, t scriptTrigger)
}

func (e agentpadEmitter) Emit(action string) error {
	return e.EmitGesture(gesture.Gesture{}, action)
}

// EmitGesture implements gesture.GestureEmitter so scripts learn the gesture.
func (e agentpadEmitter) EmitGesture(g gesture.Gesture, action string) error {
	if name, ok := profile.ScriptName(action); ok {
		if e.script == nil {
			return fmt.Errorf("%s is not available here", action)
		}
		e.script(name, scriptTrigger{Control: string(g.Control), Gesture: string(g.Kind)})
		return nil
	}
	if !strings.HasPrefix(action, profile.AgentpadPrefix) {
		return e.inner.Emit(action)
	}
	if !profile.IsAgentpadAction(action) {
		return fmt.Errorf("unknown agentpad action %q", action)
	}
	if e.handle == nil {
		return fmt.Errorf("%s is not available here", action)
	}
	e.handle(action)
	return nil
}

// liveErrMessage explains a live input / gesture mode error with the fix.
func liveErrMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, evdev.ErrPermission):
		return "No permission to read the pad's input devices. Run `just udev-install`, then replug the pad."
	case errors.Is(err, vkbd.ErrNoBackend):
		return "No virtual keyboard: /dev/uinput is not writable and ydotool is not running. " +
			"Run `just udev-install` and replug the pad (log out and back in if /dev/uinput stays root-only), or start ydotoold."
	case errors.Is(err, evdev.ErrNotFound):
		return "No pad input device found. Plug the pad in and try again."
	case errors.Is(err, evdev.ErrDisconnected):
		return "The pad was unplugged. agentpad picks it up again when it comes back."
	}
	if m := writeErrMessage(err); m != err.Error() {
		return m
	}
	return err.Error()
}

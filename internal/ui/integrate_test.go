package ui

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/evdev"
	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/jump"
	"github.com/thereisnotime/agentpad/internal/notify"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/vkbd"
)

// ---- fakes: no real pad, uinput, D-Bus or windows ----

type rawKey struct {
	code uint16
	down bool
}

type fakeSource struct{ ch chan rawKey }

func (s *fakeSource) Read(ctx context.Context, fn func(uint16, bool, time.Time)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case k := <-s.ch:
			fn(k.code, k.down, time.Now())
		}
	}
}
func (s *fakeSource) Close() error { return nil }

type fakePad struct {
	mu       sync.Mutex
	src      *fakeSource
	opens    int
	grabErr  error
	emitErr  error
	emitters []*fakeEmitter
}

func newFakePad() *fakePad { return &fakePad{src: &fakeSource{ch: make(chan rawKey, 16)}} }

func (f *fakePad) open(grab bool) gesture.Opener {
	return func() (gesture.Source, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.opens++
		if grab && f.grabErr != nil {
			return nil, f.grabErr
		}
		return f.src, nil
	}
}

func (f *fakePad) devices() LiveDevices {
	return LiveDevices{
		Observe: f.open(false),
		Grab:    f.open(true),
		NewEmitter: func() (vkbd.Emitter, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.emitErr != nil {
				return nil, f.emitErr
			}
			e := &fakeEmitter{}
			f.emitters = append(f.emitters, e)
			return e, nil
		},
	}
}

type fakeEmitter struct {
	mu     sync.Mutex
	typed  []string
	closed bool
}

func (e *fakeEmitter) Emit(a string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.typed = append(e.typed, a)
	return nil
}
func (e *fakeEmitter) Close() error { e.mu.Lock(); e.closed = true; e.mu.Unlock(); return nil }

type uploads struct {
	mu   sync.Mutex
	docs [][]byte
	err  error
}

func (u *uploads) upload(doc []byte) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.docs = append(u.docs, doc)
	return u.err
}

func (u *uploads) last() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.docs) == 0 {
		return ""
	}
	return string(u.docs[len(u.docs)-1])
}

func (u *uploads) count() int { u.mu.Lock(); defer u.mu.Unlock(); return len(u.docs) }

func gestureDoc(t *testing.T, o profile.Orientation) string {
	t.Helper()
	d, err := profile.GestureModeMapping(o)
	if err != nil {
		t.Fatal(err)
	}
	return string(d)
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// noLive never opens a pad, a virtual keyboard or anything else.
func noLive() LiveDevices {
	fail := func() (gesture.Source, error) { return nil, evdev.ErrNotFound }
	return LiveDevices{Disabled: true, Observe: fail, Grab: fail,
		NewEmitter: func() (vkbd.Emitter, error) { return nil, vkbd.ErrNoBackend }}
}

type fakeJumper struct {
	mu    sync.Mutex
	jumps []string
}

func (f *fakeJumper) Jump(_ context.Context, s events.Session) jump.Report {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jumps = append(f.jumps, s.Key())
	return jump.Report{Session: s.Key(), Steps: []jump.Step{{Name: jump.StepWindow, Status: jump.StatusDone}}}
}
func (f *fakeJumper) Close() error { return nil }

// ---- mode switching ----

func TestLiveModeSwitching(t *testing.T) {
	pad := newFakePad()
	up := &uploads{}
	var mu sync.Mutex
	var inputs []string
	var gestures []string
	c := newLiveController(pad.devices(), up.upload, liveCallbacks{
		Input: func(ev PadInputEvent) {
			mu.Lock()
			inputs = append(inputs, ev.Control)
			mu.Unlock()
		},
		Gesture: func(ev gesture.Event) {
			mu.Lock()
			gestures = append(gestures, gestureLine(ev).Text)
			mu.Unlock()
		},
	})
	p := profile.Builtins()[0] // claude-code: 1, 2, escape
	if _, err := c.SetProfile(p, profile.OrientNormal); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureObserver(); err != nil {
		t.Fatal(err)
	}
	if !c.Status().Reading || c.Mode() != ModePlain {
		t.Fatalf("observer status %+v", c.Status())
	}
	pad.src.ch <- rawKey{code: 2, down: true} // KEY_1 -> key1
	waitFor(t, "plain input", func() bool { mu.Lock(); defer mu.Unlock(); return len(inputs) == 1 })

	backend, err := c.EnterGesture(gesture.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if backend != "virtual keyboard" || c.Mode() != ModeGesture || c.Status().Backend != backend {
		t.Errorf("backend %q mode %s status %+v", backend, c.Mode(), c.Status())
	}
	if up.last() != gestureDoc(t, profile.OrientNormal) {
		t.Errorf("gesture mapping not uploaded: %q", up.last())
	}
	// F13 tap -> key1 tap -> "1" typed through the emitter
	pad.src.ch <- rawKey{code: gesture.KeyF13, down: true}
	pad.src.ch <- rawKey{code: gesture.KeyF13, down: false}
	em := pad.emitters[0]
	waitFor(t, "gesture emitted", func() bool { em.mu.Lock(); defer em.mu.Unlock(); return len(em.typed) == 1 })
	if em.typed[0] != "1" {
		t.Errorf("typed %q", em.typed)
	}
	waitFor(t, "monitor", func() bool { mu.Lock(); defer mu.Unlock(); return len(gestures) == 1 })
	if gestures[0] != "key 1 · tap → 1" {
		t.Errorf("monitor line %q", gestures[0])
	}

	// rotating in gesture mode re-programs the gesture mapping
	if _, err := c.SetProfile(p, profile.OrientClockwise); err != nil {
		t.Fatal(err)
	}
	if up.last() != gestureDoc(t, profile.OrientClockwise) {
		t.Error("orientation change should re-upload the gesture mapping")
	}

	plain := []byte("plain mapping")
	if err := c.EnterPlain(plain); err != nil {
		t.Fatal(err)
	}
	if up.last() != "plain mapping" || !em.closed || c.Mode() != ModePlain || !c.Status().Reading {
		t.Errorf("back to plain: last=%q closed=%v mode=%s status=%+v", up.last(), em.closed, c.Mode(), c.Status())
	}
	n := up.count()
	if err := c.Close([]byte("other")); err != nil {
		t.Fatal(err)
	}
	if up.count() != n {
		t.Error("closing in plain mode must not touch the pad")
	}
}

func TestLiveCloseRestoresPlain(t *testing.T) {
	pad := newFakePad()
	up := &uploads{}
	c := newLiveController(pad.devices(), up.upload, liveCallbacks{})
	if _, err := c.EnterGesture(gesture.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	if err := c.Close([]byte("plain")); err != nil {
		t.Fatal(err)
	}
	if up.last() != "plain" || !pad.emitters[0].closed {
		t.Errorf("quit must restore the plain mapping: %q", up.last())
	}
	if _, err := c.EnterGesture(gesture.DefaultConfig()); err == nil {
		t.Error("no gesture mode after close")
	}
	if c.Close(nil) != nil || up.count() != 2 {
		t.Error("second close is a no-op")
	}
}

func TestLiveEnterGestureFailures(t *testing.T) {
	t.Run("no virtual keyboard", func(t *testing.T) {
		pad := newFakePad()
		pad.emitErr = errors.Join(vkbd.ErrNoBackend, errors.New("x"))
		up := &uploads{}
		c := newLiveController(pad.devices(), up.upload, liveCallbacks{})
		_, err := c.EnterGesture(gesture.Config{})
		if !errors.Is(err, vkbd.ErrNoBackend) || c.Mode() != ModePlain || up.count() != 0 {
			t.Errorf("err=%v mode=%s uploads=%d", err, c.Mode(), up.count())
		}
		if !strings.Contains(liveErrMessage(err), "just udev-install") {
			t.Errorf("fix missing: %s", liveErrMessage(err))
		}
	})
	t.Run("no permission", func(t *testing.T) {
		pad := newFakePad()
		pad.grabErr = evdev.ErrPermission
		up := &uploads{}
		c := newLiveController(pad.devices(), up.upload, liveCallbacks{})
		_, err := c.EnterGesture(gesture.Config{})
		if !errors.Is(err, evdev.ErrPermission) || c.Mode() != ModePlain || up.count() != 0 || !pad.emitters[0].closed {
			t.Errorf("err=%v mode=%s uploads=%d", err, c.Mode(), up.count())
		}
		if !c.Status().Reading {
			t.Error("plain live input should be back")
		}
		if m := liveErrMessage(err); !strings.Contains(m, "just udev-install") || !strings.Contains(m, "replug") {
			t.Errorf("fix missing: %s", m)
		}
		_ = c.Close(nil)
	})
	t.Run("upload fails", func(t *testing.T) {
		pad := newFakePad()
		up := &uploads{err: errors.New("usb gone")}
		c := newLiveController(pad.devices(), up.upload, liveCallbacks{})
		_, err := c.EnterGesture(gesture.Config{})
		if err == nil || c.Mode() != ModePlain || !pad.emitters[0].closed {
			t.Errorf("err=%v mode=%s", err, c.Mode())
		}
		_ = c.Close(nil)
	})
}

func TestLiveObserverPermission(t *testing.T) {
	c := newLiveController(LiveDevices{
		Observe:    func() (gesture.Source, error) { return nil, evdev.ErrPermission },
		Grab:       func() (gesture.Source, error) { return nil, evdev.ErrPermission },
		NewEmitter: func() (vkbd.Emitter, error) { return &fakeEmitter{}, nil },
	}, (&uploads{}).upload, liveCallbacks{})
	if err := c.EnsureObserver(); !errors.Is(err, evdev.ErrPermission) {
		t.Fatalf("err = %v", err)
	}
	if st := c.Status(); st.Reading || !errors.Is(st.Err, evdev.ErrPermission) {
		t.Errorf("status %+v", st)
	}
	if got := modeStatusText(ModePlain, c.Status()); !strings.Contains(got, "just udev-install") {
		t.Errorf("status text %q", got)
	}
	disabled := newLiveController(LiveDevices{Disabled: true, Observe: func() (gesture.Source, error) {
		t.Fatal("disabled live input must not open the pad")
		return nil, nil
	}}, nil, liveCallbacks{})
	if disabled.EnsureObserver() != nil {
		t.Error("disabled")
	}
}

// ---- agentpad: actions ----

func TestAgentpadEmitter(t *testing.T) {
	inner := &fakeEmitter{}
	var got []string
	e := agentpadEmitter{inner: inner, handle: func(a string) { got = append(got, a) }}
	for _, a := range []string{"ctrl-c", "agentpad:jump", "agentpad:next-profile", "1"} {
		if err := e.Emit(a); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
	if !slices.Equal(inner.typed, []string{"ctrl-c", "1"}) || !slices.Equal(got, []string{"agentpad:jump", "agentpad:next-profile"}) {
		t.Errorf("typed %q handled %q", inner.typed, got)
	}
	if e.Emit("agentpad:bogus") == nil || len(inner.typed) != 2 {
		t.Error("unknown agentpad actions are errors and never typed")
	}
	if (agentpadEmitter{inner: inner}).Emit("agentpad:jump") == nil {
		t.Error("no handler is an error")
	}
}

func TestEngineInterceptsJump(t *testing.T) {
	pad := newFakePad()
	jumped := make(chan string, 1)
	c := newLiveController(pad.devices(), (&uploads{}).upload, liveCallbacks{Agentpad: func(a string) { jumped <- a }})
	_, _ = c.SetProfile(profile.Builtins()[0], profile.OrientNormal) // knob-press longhold = agentpad:jump
	cfg := gesture.DefaultConfig()
	cfg.LongHold = 30 * time.Millisecond
	cfg.Hold = 10 * time.Millisecond
	if _, err := c.EnterGesture(cfg); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(nil) }()
	pad.src.ch <- rawKey{code: gesture.KeyF17, down: true}
	time.Sleep(60 * time.Millisecond)
	pad.src.ch <- rawKey{code: gesture.KeyF17, down: false}
	select {
	case a := <-jumped:
		if a != profile.ActionJump {
			t.Errorf("action %q", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long hold on the knob did not jump")
	}
	if len(pad.emitters[0].typed) != 0 {
		t.Errorf("agentpad:jump must not be typed: %q", pad.emitters[0].typed)
	}
}

// ---- jump toast ----

func TestJumpToast(t *testing.T) {
	s := events.Session{Agent: "claude", SessionID: "s1", Origin: events.Origin{
		KonsoleService: ":1.5", KonsoleWindow: "/Windows/1", KonsoleSession: "/Sessions/3", ZellijSession: "z", ZellijPane: "2",
	}}
	step := func(n jump.StepName, be string, st jump.Status, why string) jump.Step {
		return jump.Step{Name: n, Backend: be, Status: st, Reason: why}
	}
	tests := []struct {
		name  string
		steps []jump.Step
		kind  toastKind
		want  string
	}{
		{"all done", []jump.Step{
			step(jump.StepMultiplexer, "zellij", jump.StatusDone, "focused"),
			step(jump.StepTerminalTab, "konsole", jump.StatusDone, "selected"),
			step(jump.StepWindow, "kwin", jump.StatusDone, "activated"),
		}, toastSuccess, "Jumped to Claude · konsole tab 3 · zellij pane 2"},
		{"partial", []jump.Step{
			step(jump.StepMultiplexer, "", jump.StatusSkipped, "no zellij or tmux info in origin"),
			step(jump.StepTerminalTab, "konsole", jump.StatusDone, "selected"),
			step(jump.StepWindow, "", jump.StatusSkipped, "no window backend"),
		}, toastInfo, "Jumped to Claude · konsole tab 3 (skipped: multiplexer, window)"},
		{"window only", []jump.Step{
			step(jump.StepMultiplexer, "", jump.StatusSkipped, "none"),
			step(jump.StepTerminalTab, "", jump.StatusSkipped, "none"),
			step(jump.StepWindow, "x11", jump.StatusDone, "sent"),
		}, toastInfo, "Jumped to Claude · window raised (skipped: multiplexer, terminal tab)"},
		{"done and failed", []jump.Step{
			step(jump.StepTerminalTab, "konsole", jump.StatusDone, "selected"),
			step(jump.StepWindow, "kwin", jump.StatusFailed, "no window found for pids [12]"),
		}, toastError, "Jumped to Claude · konsole tab 3. Failed: window: no window found for pids [12]"},
		{"failed", []jump.Step{
			step(jump.StepMultiplexer, "", jump.StatusSkipped, "none"),
			step(jump.StepTerminalTab, "konsole", jump.StatusFailed, "session bus: no bus"),
		}, toastError, "Couldn't jump to Claude: terminal tab: session bus: no bus"},
	}
	for _, tt := range tests {
		kind, got := jumpToast(s, jump.Report{Session: s.Key(), Steps: tt.steps})
		if kind != tt.kind || got != tt.want {
			t.Errorf("%s: got (%d) %q, want (%d) %q", tt.name, kind, got, tt.kind, tt.want)
		}
	}
	bare := events.Session{Agent: "codex", SessionID: "x"}
	if _, got := jumpToast(bare, jump.Report{Steps: []jump.Step{step(jump.StepWindow, "", jump.StatusSkipped, "x")}}); !strings.Contains(got, "Couldn't jump to Codex") || !strings.Contains(got, "Setup") {
		t.Errorf("no origin: %q", got)
	}
	tmux := events.Session{Agent: "opencode", Origin: events.Origin{TmuxPane: "%3"}}
	if _, got := jumpToast(tmux, jump.Report{Steps: []jump.Step{step(jump.StepMultiplexer, "tmux", jump.StatusDone, "ok")}}); got != "Jumped to OpenCode · tmux pane %3" {
		t.Errorf("tmux: %q", got)
	}
}

// ---- notifications ----

type fakeNotifier struct {
	mu        sync.Mutex
	shown     []string
	dismissed []string
	closed    bool
}

func (f *fakeNotifier) Notify(n notify.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown = append(f.shown, n.Key)
	return nil
}
func (f *fakeNotifier) Dismiss(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dismissed = append(f.dismissed, key)
}
func (f *fakeNotifier) Close() error { f.closed = true; return nil }

func TestAttentionNotifier(t *testing.T) {
	sink := &fakeNotifier{}
	on := true
	connects := 0
	n := newAttentionNotifier(func() (NotifyBackend, error) { connects++; return sink, nil }, func() bool { return on })
	a := events.Session{Agent: "claude", SessionID: "a", State: events.StateAttention}
	b := events.Session{Agent: "codex", SessionID: "b", State: events.StateAttention}
	n.apply(nil)
	if connects != 0 {
		t.Error("connect lazily, only when something waits")
	}
	n.apply([]events.Session{a})
	n.apply([]events.Session{a, b})
	n.apply([]events.Session{b})
	if !slices.Equal(sink.shown, []string{"claude/a", "codex/b"}) || !slices.Equal(sink.dismissed, []string{"claude/a"}) {
		t.Errorf("shown %q dismissed %q", sink.shown, sink.dismissed)
	}
	on = false
	n.apply([]events.Session{b})
	if !slices.Equal(sink.dismissed, []string{"claude/a", "codex/b"}) || len(sink.shown) != 2 {
		t.Errorf("toggle off dismisses: %q", sink.dismissed)
	}

	// goroutine path: update never blocks; stop dismisses and closes
	on = true
	stop := make(chan struct{})
	go n.run(stop)
	for range 100 {
		n.update([]events.Session{a})
	}
	waitFor(t, "notify", func() bool { sink.mu.Lock(); defer sink.mu.Unlock(); return len(sink.shown) == 3 })
	close(stop)
	<-n.done
	if !sink.closed || sink.dismissed[len(sink.dismissed)-1] != "claude/a" {
		t.Errorf("stop: closed=%v dismissed=%q", sink.closed, sink.dismissed)
	}
}

func TestAttentionNotifierConnectRetry(t *testing.T) {
	tries := 0
	n := newAttentionNotifier(func() (NotifyBackend, error) { tries++; return nil, errors.New("no bus") }, nil)
	now := time.Unix(1000, 0)
	n.now = func() time.Time { return now }
	w := []events.Session{{Agent: "claude", SessionID: "a"}}
	n.apply(w)
	n.apply(w)
	now = now.Add(time.Minute)
	n.apply(w)
	if tries != 2 {
		t.Errorf("connect tries = %d, want 2 (rate limited)", tries)
	}
}

// ---- tray ----

func TestTrayModeAndJump(t *testing.T) {
	m := trayModel(trayState{Orient: profile.OrientNormal, Mode: ModeGesture, Waiting: 0})
	if m[1].Label != "Jump to waiting agent" || !m[1].Disabled || m[1].Action != trayJump {
		t.Errorf("jump item with nothing waiting: %+v", m[1])
	}
	modes := m[3]
	if modes.Label != "Mode" || len(modes.Children) != 2 || modes.Children[0].Checked || !modes.Children[1].Checked ||
		modes.Children[1].Arg != string(ModeGesture) || modes.Children[0].Action != trayMode {
		t.Errorf("mode submenu: %+v", modes)
	}
	m = trayModel(trayState{Orient: profile.OrientNormal, Waiting: 1})
	if m[1].Disabled || m[1].Label != "Jump to waiting agent" || !m[3].Children[0].Checked {
		t.Errorf("one waiting: %+v", m[1])
	}
	m = trayModel(trayState{Orient: profile.OrientNormal, Waiting: 2})
	if m[1].Label != "Jump to waiting agent (2)" || m[1].Disabled {
		t.Errorf("two waiting: %+v", m[1])
	}
}

// ---- gesture slots ----

func TestGestureSlots(t *testing.T) {
	p := profile.Builtins()[0]
	if s := slotsFor(ctlKey1, p, false); len(s) != 1 || s[0].ID != slotAction {
		t.Fatalf("plain slots %+v", s)
	}
	key := slotsFor(ctlKey3, p, true)
	var ids, labels []string
	for _, s := range key {
		ids = append(ids, s.ID)
		labels = append(labels, s.Label)
	}
	wantIDs := []string{slotAction, "g:key3:tap", "g:key3:hold", "g:key3:longhold", "g:key3:doubletap",
		"g:key1+key2:chord", "g:key1+key3:chord", "g:key2+key3:chord"}
	if !slices.Equal(ids, wantIDs) {
		t.Fatalf("ids %q", ids)
	}
	if !slices.Equal(labels[1:5], []string{"Tap", "Hold", "Long hold", "Double-tap"}) || labels[5] != "Keys 1 + 2" {
		t.Errorf("labels %q", labels)
	}
	if key[1].Placeholder != "escape (plain)" || key[2].Placeholder != notUsed || key[1].Section != sectionGestures || key[5].Section != sectionChords {
		t.Errorf("placeholders/sections %+v", key)
	}
	if getAction(p, ctlKey3, "g:key3:hold") != "ctrl-c" || getAction(p, ctlKey3, "g:key3:tap") != "" {
		t.Error("gesture slots show explicit actions only")
	}
	knob := slotsFor(ctlKnobCW, p, true)
	if knob[1].ID != "g:knob-cw:rotate" || knob[1].Label != "Turn" || knob[1].Placeholder != "down (plain)" ||
		knob[2].ID != "g:knob-cw:pressrotate" || knob[2].Label != "Press + turn" {
		t.Errorf("knob slots %+v", knob[:3])
	}
	if s := slotsFor(ctlKnobPress, p, true); len(s) != 8 || s[3].ID != "g:knob-press:longhold" {
		t.Errorf("knob press slots %+v", s)
	}

	// editing copies the map and empty removes
	orig := profile.Builtins()[0]
	q := setAction(orig, ctlKey1, "g:key1:doubletap", " escape ")
	if q.Gestures["key1"]["doubletap"] != "escape" || orig.Gestures["key1"] != nil {
		t.Error("setAction must not modify the original gestures map")
	}
	q = setAction(q, ctlKey1, "g:key1+key3:chord", "ctrl-c")
	q = setAction(q, ctlKey1, "g:key1:doubletap", "")
	q = setAction(q, ctlKey1, "g:key1+key3:chord", "")
	if _, ok := q.Gestures["key1"]; ok || len(q.Gestures) != len(orig.Gestures) {
		t.Errorf("empty slots remove entries: %v", q.Gestures)
	}
	bare := profile.Profile{Name: "x"}
	if setAction(setAction(bare, ctlKey1, "g:key1:hold", "a"), ctlKey1, "g:key1:hold", "").Gestures != nil {
		t.Error("all-empty gestures go back to nil")
	}
	// validation: mouse is rejected, agentpad actions are fine
	if err := profile.Validate(setAction(orig, ctlKey1, "g:key1:hold", "click")); !errors.Is(err, profile.ErrMouseInGesture) {
		t.Errorf("mouse in gesture: %v", err)
	}
	if err := profile.Validate(setAction(orig, ctlKey1, "g:key1:hold", "agentpad:next-profile")); err != nil {
		t.Errorf("agentpad action: %v", err)
	}
	opts := gestureOptions()
	if slices.Contains(opts, "click") || slices.Contains(opts, "wheel(-100)") || !slices.Contains(opts, "agentpad:jump") || !slices.Contains(opts, "enter") {
		t.Error("gesture options: no mouse, with agentpad actions")
	}
	if c, k, ok := parseGestureSlot("g:key1+key2:chord"); !ok || c != "key1+key2" || k != "chord" {
		t.Error("parseGestureSlot")
	}
	if _, _, ok := parseGestureSlot(slotAction); ok {
		t.Error("plain slot is not a gesture slot")
	}
}

// ---- monitor and warnings ----

func TestGestureMonitor(t *testing.T) {
	now := time.Now()
	ev := gesture.Event{Time: now, Gesture: gesture.Gesture{Control: gesture.Key1, Kind: gesture.Hold}, Action: "ctrl-c"}
	if got := gestureLine(ev).Text; got != "key 1 · hold → ctrl-c" {
		t.Errorf("line %q", got)
	}
	ev = gesture.Event{Gesture: gesture.Gesture{Control: "key1+key3", Kind: gesture.Chord}}
	if got := gestureLine(ev).Text; got != "keys 1+3 · chord (not used)" {
		t.Errorf("chord %q", got)
	}
	ev = gesture.Event{Gesture: gesture.Gesture{Control: gesture.KnobCW, Kind: gesture.PressRotate}, Action: "shift-tab", Err: errors.New("boom")}
	if it := gestureLine(ev); it.Text != "knob right · press + turn → shift-tab failed: boom" || !it.Err {
		t.Errorf("error %+v", it)
	}
	var l gestureLog
	for i := range 12 {
		l.add(monitorItem{Text: string(rune('a' + i))})
	}
	if len(l.items) != monitorSize || l.items[0].Text != "l" {
		t.Errorf("log keeps the newest %d: %+v", monitorSize, l.items)
	}
	if monitorAlpha(0, 0) != 255 || monitorAlpha(3, 0) >= monitorAlpha(1, 0) || monitorAlpha(0, monitorFade+time.Hour) != 0 ||
		monitorAlpha(0, monitorFade+time.Second) == 255 {
		t.Error("monitorAlpha fades by position and age")
	}
}

func TestCollisionWarning(t *testing.T) {
	p := profile.Profile{Name: "x", Keys: [3]string{"enter", "a", "b"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"}}
	want := "Key 1 and knob press both send Enter, so live input can't tell them apart"
	if got := collisionWarning(p, profile.OrientNormal); got != want {
		t.Errorf("got %q", got)
	}
	p.Keys[1] = "ctrl-enter"
	if got := collisionWarning(p, profile.OrientNormal); got != "Key 1, key 2 and knob press send the same key, so live input can't tell them apart" {
		t.Errorf("got %q", got)
	}
	if collisionWarning(profile.Builtins()[0], profile.OrientNormal) != "" {
		t.Error("claude-code has no collisions")
	}
}

// ---- app level, still with fakes ----

// uiQueue replaces fyne.Do so background work is run on the test goroutine.
type uiQueue struct {
	mu sync.Mutex
	fs []func()
}

func queueUI(u *appUI) *uiQueue {
	q := &uiQueue{}
	u.post = func(f func()) { q.mu.Lock(); q.fs = append(q.fs, f); q.mu.Unlock() }
	return q
}

// drain runs queued UI work until cond holds.
func (q *uiQueue) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	waitFor(t, what, func() bool {
		q.mu.Lock()
		fs := q.fs
		q.fs = nil
		q.mu.Unlock()
		for _, f := range fs {
			f()
		}
		return cond()
	})
}

func TestAppGestureModeAndQuitRestores(t *testing.T) {
	u := newTestUI(t)
	pad := newFakePad()
	up := &uploads{}
	q := queueUI(u)
	u.live = newLiveController(pad.devices(), up.upload, liveCallbacks{})
	u.setMode(ModeGesture)
	q.until(t, "gesture mode", func() bool { return u.mode == ModeGesture })
	if u.prefs.Mode() != ModeGesture || u.pad.modeRadio.Selected != modeLabelGesture || !u.profiles.gestureMode {
		t.Error("mode shown everywhere")
	}
	if up.last() != gestureDoc(t, profile.OrientNormal) {
		t.Fatalf("gesture mapping not written")
	}
	// applying a profile in gesture mode does not touch the pad
	n := up.count()
	var applied bool
	u.applyProfile(u.profiles.profiles[1], u.profiles.profiles, func(names []string, err error) {
		applied = err == nil && names == nil
	})
	if !applied || u.prefs.ActiveProfile() != u.profiles.profiles[1].Name {
		t.Error("apply in gesture mode switches the engine profile")
	}
	want := string(u.plainDoc())
	u.shutdown()
	if up.count() != n+1 || up.last() != want {
		t.Errorf("quit must restore the plain mapping of the active profile; uploads %d->%d", n, up.count())
	}
	if !pad.emitters[0].closed {
		t.Error("emitter closed on quit")
	}
}

func TestAppJumpFromCardAndTray(t *testing.T) {
	u := newTestUI(t)
	q := queueUI(u)
	j := u.deps.Jumper.(*fakeJumper)
	for i, id := range []string{"a", "b"} {
		u.onEvent(events.Event{Time: time.Now().Add(time.Duration(i) * time.Second), Agent: "claude", SessionID: id, Kind: events.KindToolPending})
	}
	if !u.jumpBtn.Visible() || !u.sessions.cards[0].jumpBtn.Visible() {
		t.Error("jump buttons show in attention")
	}
	if m := trayModel(u.tray.state()); u.tray.desk != nil && m[1].Label != "Jump to waiting agent (2)" {
		t.Errorf("tray label %q", m[1].Label)
	}
	var order []string
	for range 3 {
		u.jumpNext()
		order = append(order, u.lastJump)
	}
	if !slices.Equal(order, []string{"claude/a", "claude/b", "claude/a"}) {
		t.Errorf("jump cycles oldest first: %q", order)
	}
	u.sessions.cards[1].jumpBtn.OnTapped()
	q.until(t, "jumps", func() bool { j.mu.Lock(); defer j.mu.Unlock(); return len(j.jumps) == 4 })
	u.onAgentpadAction(profile.ActionJump)
	q.until(t, "pad jump", func() bool { j.mu.Lock(); defer j.mu.Unlock(); return len(j.jumps) == 5 })
	q.until(t, "toast", func() bool {
		return u.toasts.box.Visible() && strings.HasPrefix(u.toasts.label.Text, "Jumped to Claude")
	})
}

// ---- setup: edit file ----

func TestPlanEdit(t *testing.T) {
	have := map[string]bool{"/home/me/.codex/config.toml": true, "/etc/udev/rules.d/50-agentpad.rules": false}
	exists := func(p string) bool { return have[p] }
	tests := []struct {
		t    editTarget
		path string
		step editStep
	}{
		{editTarget{Path: "~/.codex/config.toml"}, "/home/me/.codex/config.toml", editOpen},
		{editTarget{Path: "~/.claude/settings.json"}, "/home/me/.claude/settings.json", editCreate},
		{editTarget{Path: udevRulePath, ReadOnly: true}, udevRulePath, editUdevHint},
		{editTarget{Path: "relative.txt"}, "relative.txt", editImpossible},
		{editTarget{Path: ""}, "", editImpossible},
	}
	for _, tt := range tests {
		p, step := planEdit(tt.t, "/home/me", exists)
		if p != tt.path || step != tt.step {
			t.Errorf("%+v: got %q %d, want %q %d", tt.t, p, step, tt.path, tt.step)
		}
	}
	have[udevRulePath] = true
	if _, step := planEdit(editTarget{Path: udevRulePath, ReadOnly: true}, "/home/me", exists); step != editOpen {
		t.Error("an existing udev rule just opens")
	}
	if _, step := planEdit(editTarget{Path: "~/x"}, "", exists); step != editImpossible {
		t.Error("no home, no path")
	}
}

func TestCreateEmptyNeverOverwrites(t *testing.T) {
	p := t.TempDir() + "/a/b/settings.json"
	if fileExists(p) {
		t.Fatal("exists")
	}
	if err := createEmpty(p); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(p); err != nil || st.Size() != 0 || st.Mode().Perm() != 0o644 {
		t.Fatalf("created %v %v", st, err)
	}
	if err := os.WriteFile(p, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if createEmpty(p) == nil {
		t.Error("must not overwrite")
	}
	if b, _ := os.ReadFile(p); string(b) != "keep" {
		t.Error("content changed")
	}
}

func TestEditFileOpensExisting(t *testing.T) {
	u := newTestUI(t)
	var opened []string
	u.openFile = func(p string) error { opened = append(opened, p); return nil }
	p := t.TempDir() + "/rule"
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	u.editFile(editTarget{Path: p, ReadOnly: true})
	if !slices.Equal(opened, []string{p}) {
		t.Errorf("opened %q", opened)
	}
}

func TestEditedTag(t *testing.T) {
	if !editedBuiltin("codex", true) || editedBuiltin("codex", false) || editedBuiltin("my-own", true) {
		t.Error("only overridden built-ins are tagged")
	}
	u := newTestUI(t)
	p := u.profiles
	p.entry().SetText("f13")
	p.save()
	ch := p.chips.Objects[p.selected].(*chip)
	if !ch.tag.Visible() || ch.tag.Text != "edited" || !strings.Contains(p.inspSub.Text, editedNote) {
		t.Errorf("tag %q visible=%v sub=%q", ch.tag.Text, ch.tag.Visible(), p.inspSub.Text)
	}
	if other := p.chips.Objects[(p.selected+1)%len(p.chips.Objects)].(*chip); other.tag.Visible() {
		t.Error("untouched profiles have no tag")
	}
}

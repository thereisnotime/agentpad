package gesture

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
)

type rawKey struct {
	code uint16
	down bool
	t    time.Time
}

type fakeSource struct {
	keys   chan rawKey
	closed chan struct{}
	once   sync.Once
}

func newFakeSource() *fakeSource {
	return &fakeSource{keys: make(chan rawKey, 16), closed: make(chan struct{})}
}

func (s *fakeSource) Read(ctx context.Context, fn func(uint16, bool, time.Time)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case k, ok := <-s.keys:
			if !ok {
				return errors.New("unplugged")
			}
			fn(k.code, k.down, k.t)
		}
	}
}

func (s *fakeSource) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

type fakeEmitter struct {
	mu      sync.Mutex
	actions []string
	err     error
}

func (f *fakeEmitter) Emit(a string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, a)
	return f.err
}

func (f *fakeEmitter) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.actions)
}

func claude() profile.Profile { return profile.Builtins()[0] }

// next waits for the next event matching keep.
func next(t *testing.T, e *Engine, keep func(Event) bool) Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-e.Events():
			if !ok {
				t.Fatal("events closed")
			}
			if keep(ev) {
				return ev
			}
		case <-timeout:
			t.Fatal("timed out waiting for event")
		}
	}
}

func isGesture(ev Event) bool { return ev.IsGesture() }

func startEngine(t *testing.T, cfg Config, srcs ...*fakeSource) (*Engine, *fakeEmitter) {
	t.Helper()
	var mu sync.Mutex
	i := 0
	open := func() (Source, error) {
		mu.Lock()
		defer mu.Unlock()
		if i >= len(srcs) {
			return nil, errors.New("no pad")
		}
		i++
		return srcs[i-1], nil
	}
	em := &fakeEmitter{}
	e := NewEngine(open, em, cfg)
	e.retry = 5 * time.Millisecond
	e.SetProfile(claude())
	if err := e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Stop)
	next(t, e, func(ev Event) bool { return ev.Status == StatusRunning })
	return e, em
}

func TestEngineTapAndPressRotate(t *testing.T) {
	src := newFakeSource()
	e, em := startEngine(t, DefaultConfig(), src)
	now := time.Now()
	src.keys <- rawKey{KeyF13, true, now}
	if ev := next(t, e, func(Event) bool { return true }); ev.Input == nil || ev.Input.Control != Key1 || !ev.Input.Down {
		t.Fatalf("raw input event %+v", ev)
	}
	src.keys <- rawKey{KeyF13, false, now.Add(60 * time.Millisecond)}
	ev := next(t, e, isGesture)
	if ev.Gesture.Control != Key1 || ev.Gesture.Kind != Tap || ev.Action != "1" || ev.Err != nil {
		t.Fatalf("tap event %+v", ev)
	}
	// Knob held, then rotated: the forced release coincides with the detent.
	k := now.Add(time.Second)
	src.keys <- rawKey{KeyF17, true, k}
	src.keys <- rawKey{KeyF17, false, k.Add(300 * time.Millisecond)}
	src.keys <- rawKey{KeyF18, true, k.Add(300 * time.Millisecond)}
	src.keys <- rawKey{KeyF18, false, k.Add(308 * time.Millisecond)}
	src.keys <- rawKey{999, true, k} // unknown key: ignored
	ev = next(t, e, isGesture)
	if ev.Gesture.Control != KnobCW || ev.Gesture.Kind != PressRotate || ev.Action != "shift-tab" {
		t.Fatalf("press-rotate event %+v", ev)
	}
	if got := em.got(); !slices.Equal(got, []string{"1", "shift-tab"}) {
		t.Fatalf("emitted %v", got)
	}
}

func TestEngineHoldTimer(t *testing.T) {
	src := newFakeSource()
	e, em := startEngine(t, Config{Hold: 20 * time.Millisecond, LongHold: time.Hour}, src)
	src.keys <- rawKey{KeyF15, true, time.Now()} // key 3 held: claude-code hold = ctrl-c
	ev := next(t, e, isGesture)
	if ev.Gesture.Kind != Hold || ev.Action != "ctrl-c" {
		t.Fatalf("hold event %+v", ev)
	}
	src.keys <- rawKey{KeyF15, false, time.Now()}
	// Switching profiles changes the lookup (and what is worth waiting for).
	e.SetProfile(profile.Builtins()[3]) // media: no gestures
	now := time.Now()
	src.keys <- rawKey{KeyF15, true, now}
	src.keys <- rawKey{KeyF15, false, now.Add(time.Second)} // long press, no hold action: tap
	ev = next(t, e, isGesture)
	if ev.Gesture.Kind != Tap || ev.Action != "next" {
		t.Fatalf("media tap %+v", ev)
	}
	if got := em.got(); !slices.Equal(got, []string{"ctrl-c", "next"}) {
		t.Fatalf("emitted %v", got)
	}
	if e.Profile().Name != "media" || e.Config().Hold != 20*time.Millisecond {
		t.Fatal("accessors")
	}
}

func TestEngineEmitErrorAndUnmapped(t *testing.T) {
	src := newFakeSource()
	e, em := startEngine(t, DefaultConfig(), src)
	em.err = errors.New("no uinput")
	now := time.Now()
	src.keys <- rawKey{KeyF14, true, now}
	src.keys <- rawKey{KeyF14, false, now.Add(10 * time.Millisecond)}
	if ev := next(t, e, isGesture); ev.Action != "2" || ev.Err == nil {
		t.Fatalf("emit error not reported: %+v", ev)
	}
	e.SetProfile(profile.Profile{Name: "empty"})
	src.keys <- rawKey{KeyF16, true, now.Add(time.Second)}
	if ev := next(t, e, isGesture); ev.Gesture.Kind != Rotate || ev.Action != "" || ev.Err != nil {
		t.Fatalf("unmapped: %+v", ev)
	}
}

func TestEngineReconnect(t *testing.T) {
	a, b := newFakeSource(), newFakeSource()
	e, em := startEngine(t, DefaultConfig(), a, b)
	close(a.keys) // unplug
	ev := next(t, e, func(ev Event) bool { return ev.Status != "" })
	if ev.Status != StatusDisconnected || ev.Err == nil {
		t.Fatalf("disconnect event %+v", ev)
	}
	<-a.closed
	next(t, e, func(ev Event) bool { return ev.Status == StatusRunning })
	b.keys <- rawKey{KeyF16, true, time.Now()}
	if ev := next(t, e, isGesture); ev.Action != "up" {
		t.Fatalf("after reconnect %+v", ev)
	}
	e.Stop()
	<-b.closed
	var last Event
	for ev := range e.Events() {
		last = ev
	}
	if last.Status != StatusStopped {
		t.Fatalf("last event %+v", last)
	}
	e.Stop() // idempotent
	if err := e.Start(context.Background()); !errors.Is(err, ErrStarted) {
		t.Fatalf("restart: %v", err)
	}
	_ = em
}

func TestEngineStartError(t *testing.T) {
	boom := errors.New("permission denied")
	e := NewEngine(func() (Source, error) { return nil, boom }, &fakeEmitter{}, Config{})
	if err := e.Start(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("start: %v", err)
	}
	e.Stop()
	if _, ok := <-e.Events(); !ok {
		t.Fatal("expected stopped event before close")
	}
	// Stop before Start closes Events too.
	e2 := NewEngine(func() (Source, error) { return newFakeSource(), nil }, &fakeEmitter{}, Config{})
	e2.Stop()
	for range e2.Events() {
	}
	if err := e2.Start(context.Background()); !errors.Is(err, ErrStarted) {
		t.Fatalf("start after stop: %v", err)
	}
}

func TestEngineContextCancel(t *testing.T) {
	src := newFakeSource()
	e := NewEngine(func() (Source, error) { return src, nil }, &fakeEmitter{}, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	if err := e.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-src.closed
	for range e.Events() {
	}
	e.Stop()
}

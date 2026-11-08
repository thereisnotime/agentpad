package gesture

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestPlainKeyMap(t *testing.T) {
	m, rep, err := PlainKeyMap(profile.Builtins()[0], profile.OrientClockwise) // 1, 2, escape; up/enter/down
	if err != nil {
		t.Fatal(err)
	}
	want := KeyMap{2: Key1, 3: Key2, 1: Key3, 103: KnobCCW, 28: KnobPress, 108: KnobCW}
	if !reflect.DeepEqual(m, want) || rep.Ambiguous() {
		t.Fatalf("map %v report %+v", m, rep)
	}

	// The user's test mapping: key 1 and the knob press both send Enter.
	p := profile.Profile{Name: "t", Keys: [3]string{"enter", "ctrl-c", "a,b"},
		Knob: profile.Knob{CCW: "click", Press: "enter", CW: "volumeup"}}
	m, rep, err = PlainKeyMap(p, profile.OrientNormal)
	if err != nil {
		t.Fatal(err)
	}
	want = KeyMap{28: Key1, 46: Key2, 30: Key3, 115: KnobCW}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("map %v", m)
	}
	if !rep.Ambiguous() || !reflect.DeepEqual(rep.Collisions, []Collision{{Code: 28, Winner: Key1, Others: []Control{KnobPress}}}) ||
		!reflect.DeepEqual(rep.Unmapped, []Control{KnobCCW}) {
		t.Fatalf("report %+v", rep)
	}
	if rep.Collisions[0].String() == "" {
		t.Fatal("collision string")
	}

	// Three-way collision is one entry; a bare modifier maps to its first modifier.
	p = profile.Profile{Name: "t", Keys: [3]string{"x", "x", "ctrl-shift"}, Knob: profile.Knob{CCW: "x", Press: "<4>", CW: "a"}}
	_, rep, _ = PlainKeyMap(p, profile.OrientNormal)
	wantC := []Collision{{Code: 45, Winner: Key1, Others: []Control{Key2, KnobCCW}}, {Code: 30, Winner: KnobPress, Others: []Control{KnobCW}}}
	if !reflect.DeepEqual(rep.Collisions, wantC) {
		t.Fatalf("collisions %+v", rep.Collisions)
	}
	if m, _, _ := PlainKeyMap(p, profile.OrientNormal); m[29] != Key3 {
		t.Fatalf("bare modifier: %v", m)
	}
	if _, _, err := PlainKeyMap(p, "sideways"); err == nil {
		t.Fatal("bad orientation accepted")
	}

	g := GestureKeyMap()
	if len(g) != 6 || g[KeyF13] != Key1 || g[KeyF18] != KnobCW {
		t.Fatalf("gesture map %v", g)
	}
}

func TestObserver(t *testing.T) {
	a, b := newFakeSource(), newFakeSource()
	var mu sync.Mutex
	srcs := []*fakeSource{a, b}
	open := func() (Source, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(srcs) == 0 {
			return nil, errors.New("gone")
		}
		s := srcs[0]
		srcs = srcs[1:]
		return s, nil
	}
	km, _, _ := PlainKeyMap(profile.Builtins()[0], profile.OrientNormal)
	o := NewObserver(open, km)
	o.retry = 5 * time.Millisecond
	if err := o.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer o.Stop()
	wait := func(keep func(ControlEvent) bool) ControlEvent {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case ev, ok := <-o.Events():
				if !ok {
					t.Fatal("closed")
				}
				if keep(ev) {
					return ev
				}
			case <-timeout:
				t.Fatal("timeout")
			}
		}
	}
	isInput := func(ev ControlEvent) bool { return ev.Status == "" }
	wait(func(ev ControlEvent) bool { return ev.Status == StatusRunning })
	now := time.Now()
	a.keys <- rawKey{29, true, now} // ctrl: not mapped, dropped
	a.keys <- rawKey{28, true, now} // enter = knob press
	if ev := wait(isInput); ev.Control != KnobPress || !ev.Down || ev.Code != 28 || !ev.Time.Equal(now) {
		t.Fatalf("event %+v", ev)
	}
	o.SetKeyMap(GestureKeyMap())
	a.keys <- rawKey{28, false, now} // no longer mapped
	a.keys <- rawKey{KeyF14, true, now}
	if ev := wait(isInput); ev.Control != Key2 {
		t.Fatalf("after SetKeyMap %+v", ev)
	}
	close(a.keys)
	if ev := wait(func(ev ControlEvent) bool { return ev.Status != "" }); ev.Status != StatusDisconnected || ev.Err == nil {
		t.Fatalf("disconnect %+v", ev)
	}
	wait(func(ev ControlEvent) bool { return ev.Status == StatusRunning })
	b.keys <- rawKey{KeyF18, false, now}
	if ev := wait(isInput); ev.Control != KnobCW || ev.Down {
		t.Fatalf("after reconnect %+v", ev)
	}
	o.Stop()
	<-b.closed
	for range o.Events() {
	}
	if err := o.Start(context.Background()); !errors.Is(err, ErrStarted) {
		t.Fatalf("restart %v", err)
	}
}

func TestObserverStartError(t *testing.T) {
	o := NewObserver(func() (Source, error) { return nil, errors.New("denied") }, nil)
	if err := o.Start(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	o.Stop()
	for range o.Events() {
	}
	o2 := NewObserver(nil, nil)
	o2.Stop()
	for range o2.Events() {
	}
}

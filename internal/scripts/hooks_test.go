package scripts

import (
	"slices"
	"sync"
	"testing"
	"time"
)

type fired struct {
	mu   sync.Mutex
	runs []string
	ps   []Payload
}

func (f *fired) run(s string, p Payload) {
	f.mu.Lock()
	f.runs = append(f.runs, s)
	f.ps = append(f.ps, p)
	f.mu.Unlock()
}

func TestHooksFilterAndDebounce(t *testing.T) {
	f := &fired{}
	h := NewHooks(f.run)
	now := time.Unix(1000, 0)
	h.now = func() time.Time { return now }
	h.SetHooks([]Hook{
		{On: EventAttention, Agent: "claude", Script: "ping"},
		{On: EventAttention, Script: "beep"},
		{On: EventAttention, Script: "beep"}, // duplicate: runs once
		{On: EventGesture, Script: "g"},
	})
	if got := h.Fire(Payload{Event: EventAttention, Agent: "codex", SessionID: "s1"}); !slices.Equal(got, []string{"beep"}) {
		t.Errorf("codex: %v", got)
	}
	if got := h.Fire(Payload{Event: EventAttention, Agent: "claude", SessionID: "s1"}); !slices.Equal(got, []string{"ping", "beep"}) {
		t.Errorf("claude: %v", got)
	}
	now = now.Add(200 * time.Millisecond)
	if got := h.Fire(Payload{Event: EventAttention, Agent: "claude", SessionID: "s1"}); got != nil {
		t.Errorf("not debounced: %v", got)
	}
	if got := h.Fire(Payload{Event: EventAttention, Agent: "claude", SessionID: "s2"}); len(got) != 2 {
		t.Errorf("other session debounced: %v", got)
	}
	now = now.Add(400 * time.Millisecond)
	if got := h.Fire(Payload{Event: EventAttention, Agent: "claude", SessionID: "s1"}); len(got) != 2 {
		t.Errorf("after 600ms: %v", got)
	}
	// gestures on different controls are different events
	h.Fire(Payload{Event: EventGesture, Control: "key1", Gesture: "tap"})
	if got := h.Fire(Payload{Event: EventGesture, Control: "key2", Gesture: "tap"}); len(got) != 1 {
		t.Errorf("key2 tap: %v", got)
	}
	if got := h.Fire(Payload{Event: EventBusy}); got != nil {
		t.Errorf("unhooked event ran %v", got)
	}
	if f.ps[0].Trigger != "hook attention" {
		t.Errorf("trigger %q", f.ps[0].Trigger)
	}
}

func TestHooksSessionTransitions(t *testing.T) {
	f := &fired{}
	h := NewHooks(f.run)
	now := time.Unix(1000, 0)
	h.now = func() time.Time { now = now.Add(time.Second); return now }
	h.SetHooks([]Hook{{On: EventAttention, Script: "a"}, {On: EventBusy, Script: "b"}, {On: EventIdle, Script: "i"}})
	p := Payload{Agent: "claude", SessionID: "s1"}
	h.SessionState(p, EventIdle)      // first sight, idle: nothing
	h.SessionState(p, EventBusy)      // b
	h.SessionState(p, EventBusy)      // same state: nothing
	h.SessionState(p, EventAttention) // a
	h.SessionState(p, EventAttention) // nothing
	h.SessionState(p, EventIdle)      // i
	h.ForgetSession("claude", "s1")
	h.SessionState(p, EventAttention) // new session already waiting: a
	if want := []string{"b", "a", "i", "a"}; !slices.Equal(f.runs, want) {
		t.Fatalf("runs = %v, want %v", f.runs, want)
	}
	if f.ps[1].Event != EventAttention || f.ps[1].Agent != "claude" {
		t.Errorf("payload %+v", f.ps[1])
	}
}

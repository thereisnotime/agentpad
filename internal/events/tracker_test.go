package events

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func ev(agent, sid string, kind Kind, at time.Time) Event {
	return Event{Agent: agent, SessionID: sid, Kind: kind, Time: at}
}

func TestTrackerTransitions(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		kinds []Kind
		want  State
		gone  bool
	}{
		{"start", []Kind{KindSessionStart}, StateIdle, false},
		{"prompt", []Kind{KindSessionStart, KindPrompt}, StateBusy, false},
		{"pending", []Kind{KindPrompt, KindToolPending}, StateAttention, false},
		{"approved", []Kind{KindPrompt, KindToolPending, KindToolDone}, StateBusy, false},
		{"waiting input", []Kind{KindPrompt, KindWaitingInput}, StateIdle, false},
		{"stop", []Kind{KindPrompt, KindIdle}, StateIdle, false},
		{"other keeps busy", []Kind{KindPrompt, KindOther}, StateBusy, false},
		{"other keeps attention", []Kind{KindToolPending, KindOther}, StateAttention, false},
		{"other creates idle", []Kind{KindOther}, StateIdle, false},
		{"end", []Kind{KindPrompt, KindSessionEnd}, "", true},
		{"end unknown", []Kind{KindSessionEnd}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker()
			for i, k := range tc.kinds {
				tr.Apply(ev(AgentClaude, "s1", k, t0.Add(time.Duration(i)*time.Second)))
			}
			ss := tr.Sessions()
			if tc.gone {
				if len(ss) != 0 {
					t.Fatalf("expected no sessions, got %+v", ss)
				}
				return
			}
			if len(ss) != 1 || ss[0].State != tc.want {
				t.Fatalf("got %+v, want state %s", ss, tc.want)
			}
		})
	}
}

func TestTrackerAggregateAndChange(t *testing.T) {
	t0 := time.Now()
	tr := NewTracker()
	if tr.Aggregate() != StateIdle {
		t.Fatal("empty tracker should be idle")
	}
	steps := []struct {
		e       Event
		agg     State
		changed bool
	}{
		{ev(AgentClaude, "a", KindSessionStart, t0), StateIdle, false},
		{ev(AgentClaude, "a", KindPrompt, t0), StateBusy, true},
		{ev(AgentCodex, "b", KindToolPending, t0), StateAttention, true},
		{ev(AgentClaude, "a", KindToolDone, t0), StateAttention, false},
		{ev(AgentCodex, "b", KindSessionEnd, t0), StateBusy, true},
		{ev(AgentClaude, "a", KindIdle, t0), StateIdle, true},
		{ev(AgentClaude, "a", KindOther, t0), StateIdle, false},
	}
	for i, s := range steps {
		if got := tr.Apply(s.e); got != s.changed {
			t.Errorf("step %d: changed=%v want %v", i, got, s.changed)
		}
		if got := tr.Aggregate(); got != s.agg {
			t.Errorf("step %d: aggregate=%s want %s", i, got, s.agg)
		}
	}
}

func TestTrackerSameSessionIDDifferentAgents(t *testing.T) {
	tr := NewTracker()
	now := time.Now()
	tr.Apply(ev(AgentClaude, "x", KindPrompt, now))
	tr.Apply(ev(AgentCodex, "x", KindToolPending, now))
	if n := len(tr.Sessions()); n != 2 {
		t.Fatalf("sessions = %d", n)
	}
}

func TestTrackerSessionsOrderAndFields(t *testing.T) {
	t0 := time.Now()
	tr := NewTracker()
	tr.Apply(Event{Agent: AgentClaude, SessionID: "old", Kind: KindPrompt, Time: t0, Cwd: "/a", Tool: "Bash"})
	tr.Apply(Event{Agent: AgentClaude, SessionID: "new", Kind: KindPrompt, Time: t0.Add(time.Minute)})
	tr.Apply(Event{Agent: AgentClaude, SessionID: "old", Kind: KindOther, Time: t0.Add(-time.Hour)}) // stale, no cwd
	ss := tr.Sessions()
	if len(ss) != 2 || ss[0].SessionID != "new" || ss[1].SessionID != "old" {
		t.Fatalf("order wrong: %+v", ss)
	}
	if ss[1].Cwd != "/a" || ss[1].LastTool != "Bash" || !ss[1].LastEvent.Equal(t0) {
		t.Fatalf("fields wrong: %+v", ss[1])
	}
}

func TestTrackerPrune(t *testing.T) {
	t0 := time.Now()
	tr := NewTracker()
	tr.Apply(ev(AgentClaude, "stale", KindToolPending, t0.Add(-2*time.Hour)))
	tr.Apply(ev(AgentClaude, "fresh", KindIdle, t0))
	if !tr.Prune(time.Hour, t0) {
		t.Fatal("prune should change aggregate")
	}
	ss := tr.Sessions()
	if len(ss) != 1 || ss[0].SessionID != "fresh" || tr.Aggregate() != StateIdle {
		t.Fatalf("got %+v", ss)
	}
	if tr.Prune(time.Hour, t0) {
		t.Fatal("second prune should be a no-op")
	}
}

func TestTrackerConcurrent(t *testing.T) {
	tr := NewTracker()
	kinds := []Kind{KindSessionStart, KindPrompt, KindToolPending, KindToolDone, KindIdle, KindOther}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				tr.Apply(ev(AgentClaude, fmt.Sprintf("s%d", g), kinds[i%len(kinds)], time.Now()))
				_ = tr.Aggregate()
				_ = tr.Sessions()
				if i%50 == 0 {
					tr.Prune(time.Hour, time.Now())
				}
			}
		}()
	}
	wg.Wait()
	if n := len(tr.Sessions()); n != 8 {
		t.Fatalf("sessions = %d", n)
	}
}

package events

import (
	"testing"
	"time"
)

func keys(ss []Session) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Key()
	}
	return out
}

func TestTrackerOriginLatestNonEmptyWins(t *testing.T) {
	t0 := time.Now()
	tr := NewTracker()
	a := &Origin{KonsoleSession: "/Sessions/1"}
	b := &Origin{KonsoleSession: "/Sessions/2", ZellijPane: "4"}
	tr.Apply(Event{Agent: AgentClaude, SessionID: "s", Kind: KindPrompt, Time: t0, Origin: a})
	tr.Apply(Event{Agent: AgentClaude, SessionID: "s", Kind: KindToolDone, Time: t0, Origin: b})
	tr.Apply(Event{Agent: AgentClaude, SessionID: "s", Kind: KindOther, Time: t0})
	tr.Apply(Event{Agent: AgentClaude, SessionID: "s", Kind: KindOther, Time: t0, Origin: &Origin{}})
	if got := tr.Sessions()[0].Origin; got != *b {
		t.Fatalf("origin = %+v", got)
	}
}

func TestTrackerWaitingSince(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := NewTracker()
	tr.Apply(ev(AgentClaude, "s", KindPrompt, t0))
	if !tr.Sessions()[0].WaitingSince.IsZero() {
		t.Fatal("busy session has WaitingSince")
	}
	tr.Apply(ev(AgentClaude, "s", KindToolPending, t0.Add(time.Second)))
	tr.Apply(ev(AgentClaude, "s", KindToolPending, t0.Add(5*time.Second))) // still waiting: keep first time
	tr.Apply(ev(AgentClaude, "s", KindOther, t0.Add(6*time.Second)))
	if got := tr.Sessions()[0].WaitingSince; !got.Equal(t0.Add(time.Second)) {
		t.Fatalf("WaitingSince = %v", got)
	}
	tr.Apply(ev(AgentClaude, "s", KindToolDone, t0.Add(7*time.Second)))
	if !tr.Sessions()[0].WaitingSince.IsZero() {
		t.Fatal("WaitingSince not cleared")
	}
	tr.Apply(ev(AgentClaude, "s", KindToolPending, time.Time{}))
	if tr.Sessions()[0].WaitingSince.IsZero() {
		t.Fatal("zero event time should fall back to now")
	}
}

func TestTrackerWaitingOrderAndCycle(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := NewTracker()
	if _, ok := tr.NextWaiting(""); ok {
		t.Fatal("nothing waiting")
	}
	tr.Apply(ev(AgentClaude, "b", KindToolPending, t0.Add(2*time.Second)))
	tr.Apply(ev(AgentCodex, "a", KindToolPending, t0.Add(1*time.Second)))
	tr.Apply(ev(AgentClaude, "busy", KindPrompt, t0))
	tr.Apply(ev(AgentOpenCode, "c", KindToolPending, t0.Add(3*time.Second)))
	tr.Apply(ev(AgentClaude, "tie", KindToolPending, t0.Add(3*time.Second)))
	// A later event on a waiting session doesn't move it in the queue.
	tr.Apply(ev(AgentCodex, "a", KindOther, t0.Add(time.Hour)))

	want := []string{"codex/a", "claude/b", "claude/tie", "opencode/c"}
	if got := keys(tr.Waiting()); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("Waiting = %v want %v", got, want)
	}

	cur := ""
	var seen []string
	for range 5 {
		s, ok := tr.NextWaiting(cur)
		if !ok {
			t.Fatal("expected a waiting session")
		}
		seen = append(seen, s.Key())
		cur = s.Key()
	}
	if seen[0] != "codex/a" || seen[3] != "opencode/c" || seen[4] != "codex/a" {
		t.Fatalf("cycle = %v", seen)
	}
	if s, _ := tr.NextWaiting("claude/busy"); s.Key() != "codex/a" {
		t.Fatalf("unknown cursor -> %s", s.Key())
	}

	// Answering b drops it from the queue; cycling from b restarts at the oldest.
	tr.Apply(ev(AgentClaude, "b", KindToolDone, t0.Add(time.Minute)))
	if got := keys(tr.Waiting()); len(got) != 3 || got[1] != "claude/tie" {
		t.Fatalf("after answer = %v", got)
	}
	if s, _ := tr.NextWaiting("claude/b"); s.Key() != "codex/a" {
		t.Fatalf("stale cursor -> %s", s.Key())
	}
	// Re-entering attention queues it at the back.
	tr.Apply(ev(AgentClaude, "b", KindToolPending, t0.Add(2*time.Minute)))
	if got := keys(tr.Waiting()); got[len(got)-1] != "claude/b" {
		t.Fatalf("re-entered = %v", got)
	}
}

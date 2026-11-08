package events

import (
	"sort"
	"sync"
	"time"
)

// Tracker keeps per-session state. Safe for concurrent use.
type Tracker struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{sessions: map[string]*Session{}}
}

// Apply updates session state; returns true if the aggregate state changed.
func (t *Tracker) Apply(e Event) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	before := t.aggregate()

	key := e.Agent + "/" + e.SessionID
	if e.Kind == KindSessionEnd {
		delete(t.sessions, key)
		return t.aggregate() != before
	}

	s, ok := t.sessions[key]
	if !ok {
		if e.SessionID == "" {
			return false
		}
		s = &Session{Agent: e.Agent, SessionID: e.SessionID, State: StateIdle}
		t.sessions[key] = s
	}
	if e.Cwd != "" {
		s.Cwd = e.Cwd
	}
	if e.Tool != "" {
		s.LastTool = e.Tool
	}
	if e.Time.After(s.LastEvent) {
		s.LastEvent = e.Time
	}
	if e.Origin != nil && !e.Origin.IsZero() {
		s.Origin = *e.Origin
	}

	prev := s.State
	switch e.Kind {
	case KindSessionStart, KindWaitingInput, KindIdle:
		s.State = StateIdle
	case KindPrompt, KindToolDone:
		s.State = StateBusy
	case KindToolPending:
		s.State = StateAttention
	}
	switch {
	case s.State != StateAttention:
		s.WaitingSince = time.Time{}
	case prev != StateAttention || s.WaitingSince.IsZero():
		s.WaitingSince = e.Time
		if s.WaitingSince.IsZero() {
			s.WaitingSince = time.Now()
		}
	}
	return t.aggregate() != before
}

// Sessions returns active sessions, most recent first.
func (t *Tracker) Sessions() []Session {
	t.mu.Lock()
	out := make([]Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		out = append(out, *s)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastEvent.Equal(out[j].LastEvent) {
			return out[i].LastEvent.After(out[j].LastEvent)
		}
		return out[i].Agent+"/"+out[i].SessionID < out[j].Agent+"/"+out[j].SessionID
	})
	return out
}

// Aggregate returns attention > busy > idle across sessions.
func (t *Tracker) Aggregate() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.aggregate()
}

func (t *Tracker) aggregate() State {
	agg := StateIdle
	for _, s := range t.sessions {
		switch s.State {
		case StateAttention:
			return StateAttention
		case StateBusy:
			agg = StateBusy
		}
	}
	return agg
}

// Prune drops sessions with no events for longer than d; returns true if aggregate changed.
func (t *Tracker) Prune(d time.Duration, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	before := t.aggregate()
	cutoff := now.Add(-d)
	for k, s := range t.sessions {
		if s.LastEvent.Before(cutoff) {
			delete(t.sessions, k)
		}
	}
	return t.aggregate() != before
}

// Waiting returns the sessions in StateAttention, oldest-waiting first (ties
// broken by key so the order is stable).
func (t *Tracker) Waiting() []Session {
	t.mu.Lock()
	out := make([]Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		if s.State == StateAttention {
			out = append(out, *s)
		}
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].WaitingSince.Equal(out[j].WaitingSince) {
			return out[i].WaitingSince.Before(out[j].WaitingSince)
		}
		return out[i].Key() < out[j].Key()
	})
	return out
}

// NextWaiting returns the waiting session that follows the one with key
// after ("agent/session_id", see Session.Key) in Waiting order, wrapping
// around. An empty or unknown key (e.g. the session stopped waiting) yields
// the oldest waiting session. ok is false when nothing is waiting.
func (t *Tracker) NextWaiting(after string) (Session, bool) {
	w := t.Waiting()
	if len(w) == 0 {
		return Session{}, false
	}
	for i, s := range w {
		if s.Key() == after {
			return w[(i+1)%len(w)], true
		}
	}
	return w[0], true
}

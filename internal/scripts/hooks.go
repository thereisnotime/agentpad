package scripts

import (
	"slices"
	"sync"
	"time"
)

// Debounce is how long an identical event (same event, agent, session,
// control and gesture) is ignored after it fired.
const Debounce = 500 * time.Millisecond

// Hooks matches agentpad events against the configured hooks and starts
// the hooked scripts through run. It also turns per-session states into
// attention / busy / idle transitions. Safe for concurrent use; it never
// blocks on a script.
type Hooks struct {
	run func(script string, p Payload)
	now func() time.Time

	mu     sync.Mutex
	hooks  []Hook
	last   map[string]time.Time // debounce key -> last fire
	states map[string]string    // agent/session -> last state
}

// NewHooks returns a dispatcher calling run for every matching hook.
func NewHooks(run func(script string, p Payload)) *Hooks {
	return &Hooks{run: run, now: time.Now, last: map[string]time.Time{}, states: map[string]string{}}
}

// SetHooks replaces the hook list.
func (h *Hooks) SetHooks(hooks []Hook) {
	h.mu.Lock()
	h.hooks = slices.Clone(hooks)
	h.mu.Unlock()
}

// Matching returns the scripts hooked to p (event and agent filter), in
// hook order, each once.
func (h *Hooks) Matching(p Payload) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.matchingLocked(p)
}

func (h *Hooks) matchingLocked(p Payload) []string {
	var out []string
	for _, hk := range h.hooks {
		if hk.On != p.Event || (hk.Agent != "" && hk.Agent != p.Agent) {
			continue
		}
		if !slices.Contains(out, hk.Script) {
			out = append(out, hk.Script)
		}
	}
	return out
}

// Fire runs the scripts hooked to p.Event, unless the same event fired for
// the same session within Debounce. It returns the scripts started.
func (h *Hooks) Fire(p Payload) []string {
	now := h.now()
	if p.Time.IsZero() {
		p.Time = now
	}
	key := p.Event + "\x00" + p.Agent + "\x00" + p.SessionID + "\x00" + p.Control + "\x00" + p.Gesture
	h.mu.Lock()
	if t, ok := h.last[key]; ok && now.Sub(t) < Debounce {
		h.mu.Unlock()
		return nil
	}
	h.last[key] = now
	if len(h.last) > 256 { // forget old keys
		for k, t := range h.last {
			if now.Sub(t) >= Debounce {
				delete(h.last, k)
			}
		}
	}
	scripts := h.matchingLocked(p)
	h.mu.Unlock()
	if p.Trigger == "" {
		p.Trigger = "hook " + p.Event
	}
	for _, s := range scripts {
		h.run(s, p)
	}
	return scripts
}

// SessionState records the state ("attention", "busy" or "idle") of the
// session in p and fires that event when it changed. A session seen for the
// first time fires only when it is not idle (session_start covers that).
func (h *Hooks) SessionState(p Payload, state string) []string {
	key := p.Agent + "/" + p.SessionID
	h.mu.Lock()
	prev, seen := h.states[key]
	h.states[key] = state
	h.mu.Unlock()
	if prev == state || (!seen && state == EventIdle) {
		return nil
	}
	p.Event = state
	return h.Fire(p)
}

// ForgetSession drops a session's state (it ended or expired).
func (h *Hooks) ForgetSession(agent, session string) {
	h.mu.Lock()
	delete(h.states, agent+"/"+session)
	h.mu.Unlock()
}

// Sessions returns the session keys ("agent/id") with a recorded state.
func (h *Hooks) Sessions() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.states))
	for k := range h.states {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

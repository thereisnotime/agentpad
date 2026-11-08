package ui

// Attention jump and notifications, without Fyne.

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/jump"
	"github.com/thereisnotime/agentpad/internal/notify"
)

// JumpBackend focuses a session's terminal (jump.Jumper fits).
type JumpBackend interface {
	Jump(ctx context.Context, s events.Session) jump.Report
	Close() error
}

// NotifyBackend shows and dismisses attention notifications
// (notify.Notifier fits).
type NotifyBackend interface {
	Notify(n notify.Notification) error
	Dismiss(key string)
	Close() error
}

// agentShort is the short agent name used in toasts.
func agentShort(agent string) string {
	switch agent {
	case events.AgentClaude:
		return "Claude"
	case events.AgentCodex:
		return "Codex"
	case events.AgentOpenCode:
		return "OpenCode"
	case "":
		return "the agent"
	}
	return agent
}

// trailingNumber returns the digits at the end of s ("/Sessions/3" -> "3").
func trailingNumber(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[i:]
}

// jumpPart describes a done step for the toast, e.g. "konsole tab 3".
func jumpPart(st jump.Step, o events.Origin) string {
	switch st.Name {
	case jump.StepTerminalTab:
		if n := trailingNumber(o.KonsoleSession); st.Backend == "konsole" && n != "" {
			return "konsole tab " + n
		}
		return "terminal tab"
	case jump.StepMultiplexer:
		switch st.Backend {
		case "zellij":
			return "zellij pane " + o.ZellijPane
		case "tmux":
			return "tmux pane " + o.TmuxPane
		}
		return "multiplexer pane"
	case jump.StepWindow:
		return "window raised"
	}
	return string(st.Name)
}

// jumpToast turns a jump report into a toast, e.g.
// "Jumped to Claude · konsole tab 3 · zellij pane 2".
func jumpToast(s events.Session, rep jump.Report) (toastKind, string) {
	who := agentShort(s.Agent)
	var tab, mux, win *jump.Step
	var skipped, failed []jump.Step
	for i := range rep.Steps {
		st := &rep.Steps[i]
		switch st.Status {
		case jump.StatusDone:
			switch st.Name {
			case jump.StepTerminalTab:
				tab = st
			case jump.StepMultiplexer:
				mux = st
			default:
				win = st
			}
		case jump.StatusSkipped:
			skipped = append(skipped, *st)
		case jump.StatusFailed:
			failed = append(failed, *st)
		}
	}
	var parts []string
	for _, st := range []*jump.Step{tab, mux} {
		if st != nil {
			parts = append(parts, jumpPart(*st, s.Origin))
		}
	}
	if len(parts) == 0 && win != nil {
		parts = append(parts, jumpPart(*win, s.Origin))
	}
	if len(parts) == 0 {
		if len(failed) > 0 {
			return toastError, "Couldn't jump to " + who + ": " + stepProblem(failed[0])
		}
		if s.Origin.IsZero() {
			return toastError, "Couldn't jump to " + who + ": the hook sent no terminal info. Update the hook from Setup."
		}
		reasons := make([]string, 0, len(skipped))
		for _, st := range skipped {
			reasons = append(reasons, st.Reason)
		}
		return toastError, "Couldn't jump to " + who + ": " + strings.Join(reasons, "; ")
	}
	msg := "Jumped to " + who + " · " + strings.Join(parts, " · ")
	if len(failed) > 0 {
		probs := make([]string, 0, len(failed))
		for _, st := range failed {
			probs = append(probs, stepProblem(st))
		}
		return toastError, msg + ". Failed: " + strings.Join(probs, "; ")
	}
	if len(skipped) > 0 {
		names := make([]string, 0, len(skipped))
		for _, st := range skipped {
			names = append(names, string(st.Name))
		}
		return toastInfo, msg + " (skipped: " + strings.Join(names, ", ") + ")"
	}
	return toastSuccess, msg
}

func stepProblem(st jump.Step) string { return string(st.Name) + ": " + st.Reason }

// attentionNotifier shows a notification when a session starts waiting and
// dismisses it when the session stops waiting. It runs on its own goroutine;
// update never blocks (only the latest snapshot is kept).
type attentionNotifier struct {
	connect func() (NotifyBackend, error) // called lazily on the notifier goroutine
	enabled func() bool
	retry   time.Duration // between failed connects

	mu      sync.Mutex
	pending []events.Session
	has     bool
	wake    chan struct{}

	sink    NotifyBackend
	lastTry time.Time
	shown   map[string]bool
	now     func() time.Time
	done    chan struct{}
}

func newAttentionNotifier(connect func() (NotifyBackend, error), enabled func() bool) *attentionNotifier {
	return &attentionNotifier{
		connect: connect, enabled: enabled, retry: 30 * time.Second,
		wake: make(chan struct{}, 1), shown: map[string]bool{}, now: time.Now,
		done: make(chan struct{}),
	}
}

// update hands over the sessions currently waiting. Any goroutine, never blocks.
func (n *attentionNotifier) update(waiting []events.Session) {
	n.mu.Lock()
	n.pending, n.has = waiting, true
	n.mu.Unlock()
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

// run processes updates until stop is closed, then dismisses what it showed.
func (n *attentionNotifier) run(stop <-chan struct{}) {
	defer close(n.done)
	for {
		select {
		case <-stop:
			n.apply(nil)
			if n.sink != nil {
				_ = n.sink.Close()
			}
			return
		case <-n.wake:
			n.mu.Lock()
			w, ok := n.pending, n.has
			n.has = false
			n.mu.Unlock()
			if ok {
				n.apply(w)
			}
		}
	}
}

// apply diffs waiting against what is shown. Notifier goroutine (or tests).
func (n *attentionNotifier) apply(waiting []events.Session) {
	on := n.enabled == nil || n.enabled()
	want := map[string]events.Session{}
	if on {
		for _, s := range waiting {
			want[s.Key()] = s
		}
	}
	for key := range n.shown {
		if _, ok := want[key]; !ok {
			if n.sink != nil {
				n.sink.Dismiss(key)
			}
			delete(n.shown, key)
		}
	}
	for _, s := range waiting {
		if _, ok := want[s.Key()]; !ok || n.shown[s.Key()] {
			continue
		}
		if !n.ensureSink() {
			return
		}
		if n.sink.Notify(notify.ForSession(s)) == nil {
			n.shown[s.Key()] = true
		}
	}
}

func (n *attentionNotifier) ensureSink() bool {
	if n.sink != nil {
		return true
	}
	if !n.lastTry.IsZero() && n.now().Sub(n.lastTry) < n.retry {
		return false
	}
	n.lastTry = n.now()
	s, err := n.connect()
	if err != nil {
		return false
	}
	n.sink = s
	return true
}

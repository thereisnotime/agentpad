// Package jump brings a waiting agent's terminal back into focus.
//
// A jump runs up to three best-effort steps, in this order, using the
// events.Origin the hook captured for the session:
//
//  1. multiplexer: focus the agent's zellij pane (switching to its tab first)
//     and/or its tmux window + pane;
//  2. terminal tab: select the Konsole tab over D-Bus
//     (org.kde.konsole.Window.setCurrentSession);
//  3. window: raise and activate the terminal window, with a short-lived KWin
//     script on KDE Plasma Wayland or a _NET_ACTIVE_WINDOW client message on X11.
//
// Every step is reported as done, skipped (information missing, backend not
// applicable) or failed, with a human-readable reason. A failing step never
// stops the following ones.
//
// # API for the UI
//
//	j := jump.New()              // real exec/D-Bus/X11 backends, connects lazily
//	defer j.Close()
//	rep := j.Jump(ctx, session)  // session from events.Tracker.NextWaiting / Waiting
//	if !rep.OK() { log(rep.String()) }
//
// Cycling through several waiting sessions (oldest first) is the caller's job:
// keep the key of the last jumped session and pass it to
// events.Tracker.NextWaiting. Jump is safe for concurrent use; concurrent
// calls are serialised.
//
// All external effects go through the Jumper's fields (Run, Bus, X11, ...)
// so tests can replace them.
package jump

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/hook"
)

// StepName identifies a jump step.
type StepName string

const (
	StepMultiplexer StepName = "multiplexer"
	StepTerminalTab StepName = "terminal tab"
	StepWindow      StepName = "window"
)

// Status is the outcome of one step.
type Status string

const (
	StatusDone    Status = "done"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

// Step is one reported jump step.
type Step struct {
	Name    StepName
	Backend string // zellij, tmux, konsole, kwin, x11, or "" when skipped early
	Status  Status
	Reason  string
}

func (s Step) String() string {
	b := string(s.Name)
	if s.Backend != "" {
		b += " (" + s.Backend + ")"
	}
	return fmt.Sprintf("%s: %s: %s", b, s.Status, s.Reason)
}

// Report is the result of a jump.
type Report struct {
	Session  string // "agent/session_id"
	Steps    []Step
	Duration time.Duration
}

// OK reports whether at least one step was done and none failed.
func (r Report) OK() bool {
	done := false
	for _, s := range r.Steps {
		switch s.Status {
		case StatusFailed:
			return false
		case StatusDone:
			done = true
		}
	}
	return done
}

// Step returns the first step with the given name.
func (r Report) Step(n StepName) (Step, bool) {
	for _, s := range r.Steps {
		if s.Name == n {
			return s, true
		}
	}
	return Step{}, false
}

func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "jump %s (%s)", r.Session, r.Duration.Round(time.Millisecond))
	for _, s := range r.Steps {
		b.WriteString("\n  ")
		b.WriteString(s.String())
	}
	return b.String()
}

// WindowBackend selects how the terminal window is raised.
type WindowBackend string

const (
	WindowAuto WindowBackend = ""     // pick from the desktop session (see DetectWindowBackend)
	WindowKWin WindowBackend = "kwin" // KDE Plasma (Wayland or X11)
	WindowX11  WindowBackend = "x11"  // EWMH _NET_ACTIVE_WINDOW
	WindowNone WindowBackend = "none" // never raise
)

// Runner runs an external command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Bus is the subset of a D-Bus session connection the jumper uses.
type Bus interface {
	// Call invokes method ("interface.Member") on dest at path.
	Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error)
	// Callback registers a one-shot receiver for feedback from a KWin script.
	// The script calls callDBus(dest, path, CallbackInterface, "Result", token,
	// found, caption); the arguments arrive on ch. cancel releases the token.
	Callback(token string) (dest, path string, ch <-chan []any, cancel func(), err error)
	Close() error
}

// X11 activates windows through EWMH.
type X11 interface {
	// Activate sends _NET_ACTIVE_WINDOW for win. When win is 0 it first looks up
	// a client window whose _NET_WM_PID is in pids (nearest first). Returns the
	// window it activated.
	Activate(win uint32, pids []int) (uint32, error)
}

// Jumper performs jumps. The zero value is not usable; call New.
type Jumper struct {
	// Run executes zellij/tmux. Default: os/exec with ExecTimeout.
	Run Runner
	// Bus is the session bus. Nil means connect lazily with DialBus.
	Bus Bus
	// DialBus connects to the session bus when Bus is nil.
	DialBus func() (Bus, error)
	// X11 is the X11 backend. Nil means connect lazily with DialX11.
	X11 X11
	// DialX11 connects to $DISPLAY when X11 is nil.
	DialX11 func() (X11, error)
	// Getenv reads agentpad's own environment (session type detection).
	Getenv func(string) string
	// Ancestors returns pid followed by its ancestors, nearest first.
	Ancestors func(pid int) []int
	// Backend forces a window backend; WindowAuto detects it.
	Backend WindowBackend
	// ExecTimeout bounds each external command (default 2s).
	ExecTimeout time.Duration
	// ScriptWait is how long a KWin script may run before it is unloaded
	// (default 300ms).
	ScriptWait time.Duration
	// ScriptDir holds the temporary KWin scripts (default os.TempDir()).
	ScriptDir string

	mu      sync.Mutex
	ownBus  bool
	scripts int
}

// New returns a Jumper wired to the real system: os/exec, the D-Bus session
// bus and X11 (both connected on first use).
func New() *Jumper {
	return &Jumper{
		Run:     execRunner,
		DialBus: func() (Bus, error) { return ConnectSessionBus() },
		DialX11: func() (X11, error) { return DialX11() },
		Getenv:  os.Getenv,
		Ancestors: func(pid int) []int {
			out := []int{pid}
			for _, p := range hook.Ancestors("/proc", pid) {
				out = append(out, p.PID)
			}
			return out
		},
	}
}

// Close releases the D-Bus connection the jumper opened itself.
func (j *Jumper) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.ownBus && j.Bus != nil {
		err := j.Bus.Close()
		j.Bus, j.ownBus = nil, false
		return err
	}
	return nil
}

// Jump focuses s's multiplexer pane, terminal tab and window, in that order.
func (j *Jumper) Jump(ctx context.Context, s events.Session) Report {
	j.mu.Lock()
	defer j.mu.Unlock()
	start := time.Now()
	o := s.Origin
	rep := Report{Session: s.Key()}

	rep.Steps = append(rep.Steps, j.multiplexer(ctx, o)...)
	tab, hint, konsolePID := j.konsoleTab(ctx, o)
	rep.Steps = append(rep.Steps, tab)
	rep.Steps = append(rep.Steps, j.raise(ctx, o, hint, konsolePID))
	rep.Duration = time.Since(start)
	return rep
}

func (j *Jumper) execTimeout() time.Duration {
	if j.ExecTimeout > 0 {
		return j.ExecTimeout
	}
	return 2 * time.Second
}

func (j *Jumper) getenv(k string) string {
	if j.Getenv == nil {
		return ""
	}
	return j.Getenv(k)
}

// bus returns the session bus, connecting on first use. Caller holds j.mu.
func (j *Jumper) bus() (Bus, error) {
	if j.Bus != nil {
		return j.Bus, nil
	}
	if j.DialBus == nil {
		return nil, fmt.Errorf("no D-Bus connection")
	}
	b, err := j.DialBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	j.Bus, j.ownBus = b, true
	return b, nil
}

func done(n StepName, backend, reason string, a ...any) Step {
	return Step{Name: n, Backend: backend, Status: StatusDone, Reason: fmt.Sprintf(reason, a...)}
}

func skipped(n StepName, backend, reason string, a ...any) Step {
	return Step{Name: n, Backend: backend, Status: StatusSkipped, Reason: fmt.Sprintf(reason, a...)}
}

func failed(n StepName, backend, reason string, a ...any) Step {
	return Step{Name: n, Backend: backend, Status: StatusFailed, Reason: fmt.Sprintf(reason, a...)}
}

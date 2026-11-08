package scripts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Runner limits.
const (
	// MaxConcurrent is how many scripts run at once; more wait their turn.
	MaxConcurrent = 4
	// KeepRuns is how many runs Runs returns (and Load reads back).
	KeepRuns = 50
	// OutputCap caps captured stdout and stderr, each.
	OutputCap = 64 << 10
	// logMaxBytes is when the run log is trimmed to its last logKeepLines.
	logMaxBytes  = 1 << 20
	logKeepLines = 200
)

// Payload describes what triggered a run. It is written as JSON on the
// script's stdin and as AGENTPAD_* environment variables.
type Payload struct {
	Event     string    `json:"event"` // hook event, "pad" or "manual"
	Time      time.Time `json:"time"`
	Agent     string    `json:"agent,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Cwd       string    `json:"cwd,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	Message   string    `json:"message,omitempty"`
	Profile   string    `json:"profile,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Control   string    `json:"control,omitempty"` // key1, knob-cw, key1+key3, ...
	Gesture   string    `json:"gesture,omitempty"` // tap, hold, ...
	Action    string    `json:"action,omitempty"`
	Pad       string    `json:"pad,omitempty"` // e.g. "1189:8890"
	// Trigger is a short human description for the run list, e.g.
	// "hook attention", "key 2 hold", "Run now".
	Trigger string `json:"trigger,omitempty"`
}

// Env returns the AGENTPAD_* variables for p.
func (p Payload) Env() []string {
	return []string{
		"AGENTPAD_EVENT=" + p.Event,
		"AGENTPAD_AGENT=" + p.Agent,
		"AGENTPAD_SESSION=" + p.SessionID,
		"AGENTPAD_CWD=" + p.Cwd,
		"AGENTPAD_TOOL=" + p.Tool,
		"AGENTPAD_PROFILE=" + p.Profile,
		"AGENTPAD_CONTROL=" + p.Control,
		"AGENTPAD_GESTURE=" + p.Gesture,
	}
}

// RunID identifies one run.
type RunID int64

// Status of a run.
type Status string

// Run statuses.
const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusOK      Status = "ok"      // exit 0
	StatusFailed  Status = "failed"  // non-zero exit
	StatusTimeout Status = "timeout" // killed after the timeout
	StatusError   Status = "error"   // could not start (unknown script, bad cwd, ...)
	StatusSkipped Status = "skipped" // the script was still running
)

// Done reports whether s is final.
func (s Status) Done() bool { return s != StatusQueued && s != StatusRunning }

// Run is one script run as recorded in the log and shown in the UI.
type Run struct {
	ID       RunID         `json:"id"`
	Script   string        `json:"script"`
	Trigger  string        `json:"trigger,omitempty"`
	Event    string        `json:"event,omitempty"`
	Start    time.Time     `json:"start"`
	Duration time.Duration `json:"duration_ns"`
	Status   Status        `json:"status"`
	ExitCode int           `json:"exit_code"`
	Err      string        `json:"error,omitempty"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	// Truncated is set when stdout or stderr went over OutputCap.
	Truncated bool `json:"truncated,omitempty"`
}

// Options configure a Runner. Zero values get defaults.
type Options struct {
	// LogPath is the JSONL run log; empty disables it.
	LogPath string
	// Environ is the base environment; nil means os.Environ().
	Environ []string
	// MaxConcurrent defaults to MaxConcurrent.
	MaxConcurrent int
	// Shell runs `shell:` scripts; default /bin/sh.
	Shell string
	// KillGrace defaults to KillGrace.
	KillGrace time.Duration
}

// Runner runs scripts. It is safe for concurrent use and never blocks its
// callers on a script.
type Runner struct {
	opts Options
	sem  chan struct{}
	wg   sync.WaitGroup

	mu       sync.Mutex
	scripts  map[string]Script
	running  map[string]bool
	runs     []Run // oldest first, at most KeepRuns
	nextID   RunID
	onRun    []func(Run)
	cancels  map[RunID]context.CancelFunc
	closed   bool
	logMu    sync.Mutex
	now      func() time.Time
	homeDir  func() (string, error)
	afterRun func() // tests
}

// NewRunner returns a runner with no scripts.
func NewRunner(opts Options) *Runner {
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = MaxConcurrent
	}
	if opts.Shell == "" {
		opts.Shell = "/bin/sh"
	}
	if opts.KillGrace <= 0 {
		opts.KillGrace = KillGrace
	}
	return &Runner{
		opts: opts, sem: make(chan struct{}, opts.MaxConcurrent),
		scripts: map[string]Script{}, running: map[string]bool{},
		cancels: map[RunID]context.CancelFunc{},
		now:     time.Now, homeDir: os.UserHomeDir,
	}
}

// SetScripts replaces the script definitions (runs in progress continue).
func (r *Runner) SetScripts(m map[string]Script) {
	cp := make(map[string]Script, len(m))
	for k, v := range m {
		cp[k] = v.Clone()
	}
	r.mu.Lock()
	r.scripts = cp
	r.mu.Unlock()
}

// Has reports whether a script is defined.
func (r *Runner) Has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.scripts[name]
	return ok
}

// OnRun registers f; it is called (on a runner goroutine, must not block)
// when a run is queued, starts and finishes.
func (r *Runner) OnRun(f func(Run)) {
	r.mu.Lock()
	r.onRun = append(r.onRun, f)
	r.mu.Unlock()
}

// Runs returns the last KeepRuns runs, newest first.
func (r *Runner) Runs() []Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.runs)
	slices.Reverse(out)
	return out
}

// Running reports whether script name is running (or queued).
func (r *Runner) Running(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[name]
}

// Last returns the newest finished run of script name.
func (r *Runner) Last(name string) (Run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.runs) - 1; i >= 0; i-- {
		if r.runs[i].Script == name && r.runs[i].Status.Done() && r.runs[i].Status != StatusSkipped {
			return r.runs[i], true
		}
	}
	return Run{}, false
}

// Run starts script name in the background and returns its ID at once.
// A script that is still running is not started again: the trigger is
// recorded as skipped. ctx cancels the run like a timeout does.
func (r *Runner) Run(ctx context.Context, name string, p Payload) RunID {
	if p.Time.IsZero() {
		p.Time = r.now()
	}
	r.mu.Lock()
	r.nextID++
	run := Run{ID: r.nextID, Script: name, Trigger: p.Trigger, Event: p.Event, Start: r.now(), Status: StatusQueued}
	s, ok := r.scripts[name]
	switch {
	case r.closed:
		run.Status, run.Err = StatusError, "agentpad is shutting down"
	case !ok:
		run.Status, run.Err = StatusError, fmt.Sprintf("unknown script %q", name)
	case r.running[name]:
		run.Status, run.Err = StatusSkipped, "still running, trigger dropped"
	}
	if run.Status.Done() {
		r.recordLocked(run)
		cbs := slices.Clone(r.onRun)
		r.mu.Unlock()
		r.notify(cbs, run)
		r.appendLog(run)
		return run.ID
	}
	r.running[name] = true
	ctx, cancel := context.WithCancel(ctx)
	r.cancels[run.ID] = cancel
	r.recordLocked(run)
	cbs := slices.Clone(r.onRun)
	r.wg.Add(1)
	r.mu.Unlock()
	r.notify(cbs, run)
	id := run.ID

	go func() {
		defer r.wg.Done()
		defer cancel()
		select {
		case r.sem <- struct{}{}:
		case <-ctx.Done():
			run.Status, run.Err = StatusError, "cancelled before it started"
			r.finish(run)
			return
		}
		run.Status, run.Start = StatusRunning, r.now()
		r.update(run)
		run = r.exec(ctx, s, p, run)
		r.finish(run)
		<-r.sem
	}()
	return id
}

// recordLocked appends or replaces run in the history. Needs r.mu.
func (r *Runner) recordLocked(run Run) {
	for i := len(r.runs) - 1; i >= 0; i-- {
		if r.runs[i].ID == run.ID {
			r.runs[i] = run
			return
		}
	}
	r.runs = append(r.runs, run)
	if len(r.runs) > KeepRuns {
		r.runs = slices.Delete(r.runs, 0, len(r.runs)-KeepRuns)
	}
}

func (r *Runner) update(run Run) {
	r.mu.Lock()
	r.recordLocked(run)
	cbs := slices.Clone(r.onRun)
	r.mu.Unlock()
	r.notify(cbs, run)
}

func (r *Runner) finish(run Run) {
	r.mu.Lock()
	delete(r.running, run.Script)
	delete(r.cancels, run.ID)
	r.recordLocked(run)
	cbs := slices.Clone(r.onRun)
	after := r.afterRun
	r.mu.Unlock()
	r.appendLog(run)
	r.notify(cbs, run)
	if after != nil {
		after()
	}
}

func (r *Runner) notify(cbs []func(Run), run Run) {
	for _, f := range cbs {
		f(run)
	}
}

// Wait blocks until every started run has finished.
func (r *Runner) Wait() { r.wg.Wait() }

// Close stops accepting runs, kills the running ones (SIGTERM, then
// SIGKILL after the grace period) and waits for them.
func (r *Runner) Close() {
	r.mu.Lock()
	r.closed = true
	for _, c := range r.cancels {
		c()
	}
	r.mu.Unlock()
	r.wg.Wait()
}

// expandHome expands a leading "~" or "~/".
func (r *Runner) expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := r.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}

// command builds the exec.Cmd for s (without starting it).
func (r *Runner) command(s Script, p Payload) (*exec.Cmd, error) {
	var argv []string
	if s.Shell != "" {
		argv = []string{r.opts.Shell, "-c", s.Shell}
	} else {
		if len(s.Run) == 0 {
			return nil, errors.New("script has no command")
		}
		argv = slices.Clone(s.Run)
		first, err := r.expandHome(argv[0])
		if err != nil {
			return nil, err
		}
		argv[0] = first
	}
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // running the user's own scripts is the point
	if s.Cwd != "" {
		dir, err := r.expandHome(s.Cwd)
		if err != nil {
			return nil, err
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("cwd %s is not a directory", dir)
		}
		cmd.Dir = dir
	}
	env := r.opts.Environ
	if env == nil {
		env = os.Environ()
	}
	env = append(slices.Clone(env), p.Env()...)
	for _, k := range sortedKeys(s.Env) {
		env = append(env, k+"="+s.Env[k])
	}
	cmd.Env = env
	stdin, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(append(stdin, '\n'))
	setProcessGroup(cmd)
	// a grandchild keeping stdout open must not hang Wait
	cmd.WaitDelay = r.opts.KillGrace
	return cmd, nil
}

// exec runs s to completion and fills in the result fields of run.
func (r *Runner) exec(ctx context.Context, s Script, p Payload, run Run) Run {
	start := r.now()
	cmd, err := r.command(s, p)
	if err != nil {
		run.Status, run.Err, run.ExitCode = StatusError, err.Error(), -1
		return run
	}
	var stdout, stderr capBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		run.Status, run.Err, run.ExitCode = StatusError, err.Error(), -1
		run.Duration = r.now().Sub(start)
		return run
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(s.EffectiveTimeout())
	defer timer.Stop()
	timedOut := false
	select {
	case err = <-done:
	case <-timer.C:
		timedOut = true
	case <-ctx.Done():
		timedOut = true
	}
	if timedOut {
		killGroup(cmd, false)
		select {
		case err = <-done:
		case <-time.After(r.opts.KillGrace):
			killGroup(cmd, true)
			err = <-done
		}
		// the leader may be gone while others in its group live on
		killGroup(cmd, true)
	}
	run.Duration = r.now().Sub(start)
	run.Stdout, run.Stderr = stdout.String(), stderr.String()
	run.Truncated = stdout.truncated || stderr.truncated
	run.ExitCode = cmd.ProcessState.ExitCode()
	switch {
	case timedOut:
		run.Status = StatusTimeout
		run.Err = fmt.Sprintf("killed after %s", s.EffectiveTimeout())
		if ctx.Err() != nil {
			run.Err = "cancelled"
		}
	case err == nil:
		run.Status = StatusOK
	default:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			run.Status = StatusFailed
			if run.ExitCode < 0 {
				run.Err = ee.Error()
			}
		} else {
			run.Status, run.Err = StatusFailed, err.Error()
		}
	}
	return run
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// capBuffer keeps the first OutputCap bytes written to it.
type capBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	truncated bool
}

func (b *capBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := OutputCap - b.buf.Len(); room < len(p) {
		b.truncated = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *capBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// appendLog adds a finished run to the JSONL log, trimming it now and then.
func (r *Runner) appendLog(run Run) {
	if r.opts.LogPath == "" || !run.Status.Done() {
		return
	}
	line, err := json.Marshal(run)
	if err != nil {
		return
	}
	r.logMu.Lock()
	defer r.logMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.opts.LogPath), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(r.opts.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	fi, err := f.Stat()
	_ = f.Close()
	if err == nil && fi.Size() > logMaxBytes {
		trimLog(r.opts.LogPath, logKeepLines)
	}
}

// trimLog keeps the last n lines of path.
func trimLog(path string, n int) {
	lines, err := readLines(path)
	if err != nil || len(lines) <= n {
		return
	}
	data := []byte(strings.Join(lines[len(lines)-n:], "\n") + "\n")
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			out = append(out, sc.Text())
		}
	}
	return out, sc.Err()
}

// Load reads the last KeepRuns runs back from the log (at start-up), so
// "Last runs" survives a restart. A missing log is not an error.
func (r *Runner) Load() error {
	if r.opts.LogPath == "" {
		return nil
	}
	lines, err := readLines(r.opts.LogPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var runs []Run
	for _, l := range lines[max(0, len(lines)-KeepRuns):] {
		var run Run
		if json.Unmarshal([]byte(l), &run) == nil && run.Script != "" {
			runs = append(runs, run)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range runs {
		r.nextID++
		run.ID = r.nextID
		r.runs = append(r.runs, run)
	}
	if len(r.runs) > KeepRuns {
		r.runs = slices.Delete(r.runs, 0, len(r.runs)-KeepRuns)
	}
	return nil
}

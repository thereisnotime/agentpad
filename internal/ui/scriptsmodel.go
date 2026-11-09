package ui

// Scripts screen, inspector and hook wiring helpers, without Fyne.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/scripts"
)

// scriptForm is the script editor's fields as typed.
type scriptForm struct {
	Name    string
	Command string // argv as a command line (split, never run by a shell), or the shell line
	Shell   bool
	Cwd     string
	Env     string // KEY=value lines
	Timeout string // e.g. "30s", "" = default
}

// formFromScript fills the editor from s.
func formFromScript(name string, s scripts.Script) scriptForm {
	f := scriptForm{Name: name, Cwd: s.Cwd, Env: envLines(s.Env)}
	if s.Shell != "" {
		f.Shell, f.Command = true, s.Shell
	} else {
		f.Command = argvText(s.Run)
	}
	if s.Timeout > 0 {
		f.Timeout = s.Timeout.String()
	}
	return f
}

// script validates the form and returns the script.
func (f scriptForm) script() (string, scripts.Script, error) {
	name := strings.TrimSpace(f.Name)
	if !scripts.ValidName(name) {
		return "", scripts.Script{}, fmt.Errorf("name %q: use a-z, 0-9 and - only", name)
	}
	var s scripts.Script
	cmd := strings.TrimSpace(f.Command)
	if f.Shell {
		s.Shell = cmd
	} else {
		argv, err := splitArgs(cmd)
		if err != nil {
			return "", s, err
		}
		s.Run = argv
	}
	s.Cwd = strings.TrimSpace(f.Cwd)
	env, err := parseEnvLines(f.Env)
	if err != nil {
		return "", s, err
	}
	s.Env = env
	if t := strings.TrimSpace(f.Timeout); t != "" {
		d, err := time.ParseDuration(t)
		if err != nil {
			return "", s, fmt.Errorf("timeout %q: use e.g. 30s or 2m", t)
		}
		s.Timeout = d
	}
	if err := s.Validate(); err != nil {
		return "", s, err
	}
	return name, s, nil
}

// splitArgs splits a command line into argv like a shell would split words
// ('single', "double" quotes and backslash escapes), without expanding
// anything. The result is run directly, not through a shell.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord := false
	var quote rune
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case quote != 0:
			switch {
			case r == quote:
				quote = 0
			case r == '\\' && quote == '"':
				esc = true
			default:
				cur.WriteRune(r)
			}
		case r == '\\':
			esc, inWord = true, true
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || esc {
		return nil, errors.New("command: unterminated quote or trailing backslash")
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out, nil
}

// argvText renders argv so splitArgs reads it back.
func argvText(argv []string) string { return scripts.Script{Run: argv}.Preview() }

// parseEnvLines reads KEY=value lines (blank lines and # comments skipped).
func parseEnvLines(s string) (map[string]string, error) {
	var out map[string]string
	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("env line %d: want KEY=value", i+1)
		}
		if out == nil {
			out = map[string]string{}
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}

// envLines renders env as sorted KEY=value lines.
func envLines(env map[string]string) string {
	keys := slices.Sorted(maps.Keys(env))
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + env[k]
	}
	return strings.Join(lines, "\n")
}

// runResult is the status column of a run: "exit 0", "exit 3", "timeout"...
func runResult(r scripts.Run) string {
	switch r.Status {
	case scripts.StatusOK, scripts.StatusFailed:
		if r.ExitCode < 0 {
			return "killed"
		}
		return fmt.Sprintf("exit %d", r.ExitCode)
	case scripts.StatusTimeout:
		if r.Err == "cancelled" {
			return "cancelled"
		}
		return "timeout"
	case scripts.StatusError:
		return "error"
	case scripts.StatusSkipped:
		return "skipped"
	case scripts.StatusQueued:
		return "queued"
	}
	return "running"
}

// runColor is the status dot colour of a run (idle grey when never run).
func runColor(r scripts.Run, ok bool) colorKind {
	switch {
	case !ok:
		return dotNone
	case r.Status == scripts.StatusOK:
		return dotOK
	case r.Status == scripts.StatusRunning || r.Status == scripts.StatusQueued:
		return dotRunning
	case r.Status == scripts.StatusSkipped:
		return dotNone
	}
	return dotFailed
}

// colorKind names the status dot colours (mapped to the palette by the UI).
type colorKind int

const (
	dotNone colorKind = iota
	dotOK
	dotFailed
	dotRunning
)

// durationText is a short duration: "12 ms", "1.4 s", "2m05s".
func durationText(d time.Duration) string {
	switch {
	case d <= 0:
		return "—"
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// runToast is the Run now toast for a finished run.
func runToast(r scripts.Run) (toastKind, string) {
	switch r.Status {
	case scripts.StatusOK:
		msg := r.Script + " finished (exit 0, " + durationText(r.Duration) + ")."
		if out := firstLine(r.Stdout); out != "" {
			msg += " " + out
		}
		return toastSuccess, msg
	case scripts.StatusSkipped:
		return toastInfo, r.Script + " is still running."
	case scripts.StatusTimeout:
		return toastError, r.Script + " " + r.Err + "."
	case scripts.StatusError:
		return toastError, r.Script + " did not start: " + r.Err
	}
	msg := r.Script + " failed (" + runResult(r) + ")."
	if out := firstLine(r.Stderr); out != "" {
		msg += " " + out
	}
	return toastError, msg
}

// firstLine is the first non-empty line of s, cut to 120 runes.
func firstLine(s string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return capLabel(l, 120)
		}
	}
	return ""
}

// padTrigger describes a pad trigger for the run list: "key 2 · hold",
// "knob right · layer 2".
func padTrigger(t scriptTrigger) string {
	s := controlLabel(t.Control)
	if s == "" {
		s = "pad"
	}
	switch {
	case t.Gesture != "":
		s += " · " + strings.ToLower(kindLabel(t.Gesture))
	case t.Layer > 1:
		s += fmt.Sprintf(" · layer %d", t.Layer)
	}
	return s
}

// scriptActions are the run:<name> actions for names.
func scriptActions(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = profile.RunAction(n)
	}
	return out
}

// eventLabel is the hook event as shown in the UI.
func eventLabel(ev string) string {
	switch ev {
	case scripts.EventAttention:
		return "Agent needs you"
	case scripts.EventBusy:
		return "Agent busy"
	case scripts.EventIdle:
		return "Agent idle"
	case scripts.EventSessionStart:
		return "Session starts"
	case scripts.EventSessionEnd:
		return "Session ends"
	case scripts.EventGesture:
		return "Gesture"
	case scripts.EventProfileApplied:
		return "Profile applied"
	case scripts.EventModeChanged:
		return "Mode changed"
	case scripts.EventPadConnected:
		return "Pad connected"
	case scripts.EventPadDisconnected:
		return "Pad disconnected"
	case scripts.EventJump:
		return "Jump"
	}
	return ev
}

// eventFromLabel is the inverse of eventLabel ("" when unknown).
func eventFromLabel(label string) string {
	for _, ev := range scripts.Events() {
		if eventLabel(ev) == label {
			return ev
		}
	}
	return ""
}

// anyAgent is the hook agent filter option for "every agent".
const anyAgent = "Any agent"

// sessionPayload is the hook payload for a session event.
func sessionPayload(s events.Session, e events.Event) scripts.Payload {
	p := scripts.Payload{Time: e.Time, Agent: s.Agent, SessionID: s.SessionID, Cwd: s.Cwd, Tool: s.LastTool, Message: e.Message}
	if p.Agent == "" {
		p.Agent, p.SessionID, p.Cwd, p.Tool = e.Agent, e.SessionID, e.Cwd, e.Tool
	}
	return p
}

// sessionHooks fires the hooks for one received event: session_start /
// session_end, and attention / busy / idle when the session's state changed.
// after is the session after the tracker applied e (found false when it
// ended or was never tracked).
func sessionHooks(h *scripts.Hooks, e events.Event, after events.Session, found bool) {
	p := sessionPayload(after, e)
	switch e.Kind {
	case events.KindSessionEnd:
		p.Event = scripts.EventSessionEnd
		h.Fire(p)
		h.ForgetSession(e.Agent, e.SessionID)
		return
	case events.KindSessionStart:
		p.Event = scripts.EventSessionStart
		h.Fire(p)
	}
	if found {
		h.SessionState(p, string(after.State))
	}
}

// findSession returns the tracked session with key.
func findSession(list []events.Session, agent, id string) (events.Session, bool) {
	for _, s := range list {
		if s.Agent == agent && s.SessionID == id {
			return s, true
		}
	}
	return events.Session{}, false
}

// inspectorPicks are the quick pick chips: the common keys, then one
// run:<name> chip per script.
func inspectorPicks(scriptNames []string) []string {
	return append(slices.Clone(quickPicks), scriptActions(scriptNames)...)
}

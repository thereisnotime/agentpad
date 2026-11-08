// Package scripts runs the user's own commands: named scripts from
// settings.yaml, started from pad controls (`run:<name>`), from event hooks
// or with "Run now" in the UI.
//
// Nothing here imports Fyne. A Runner never blocks its caller: every run
// happens on its own goroutine, bounded by a timeout that kills the whole
// process group.
package scripts

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Script is one named script. Exactly one of Run and Shell is set.
type Script struct {
	// Run is the argument vector; no shell is involved. "~" is expanded in
	// Run[0].
	Run []string `yaml:"run,omitempty,flow"`
	// Shell is a command line run with /bin/sh -c (explicit opt-in).
	Shell string `yaml:"shell,omitempty"`
	// Cwd is the working directory ("~" expanded); empty means agentpad's.
	Cwd string `yaml:"cwd,omitempty"`
	// Env is added to agentpad's environment (after the AGENTPAD_* values).
	Env map[string]string `yaml:"env,omitempty"`
	// Timeout bounds a run; 0 means DefaultTimeout.
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// Hook runs Script when event On happens (for Agent, if set).
type Hook struct {
	On     string `yaml:"on"`
	Agent  string `yaml:"agent,omitempty"` // claude | codex | opencode; empty = any
	Script string `yaml:"script"`
}

// Timeouts.
const (
	DefaultTimeout = 30 * time.Second
	MaxTimeout     = 10 * time.Minute
	// KillGrace is how long a timed-out process group gets between SIGTERM
	// and SIGKILL.
	KillGrace = 2 * time.Second
)

// Hook events.
const (
	EventAttention       = "attention"
	EventBusy            = "busy"
	EventIdle            = "idle"
	EventSessionStart    = "session_start"
	EventSessionEnd      = "session_end"
	EventGesture         = "gesture"
	EventProfileApplied  = "profile_applied"
	EventModeChanged     = "mode_changed"
	EventPadConnected    = "pad_connected"
	EventPadDisconnected = "pad_disconnected"
	EventJump            = "jump"
)

// Events returns every hook event name in display order.
func Events() []string {
	return []string{EventAttention, EventBusy, EventIdle, EventSessionStart, EventSessionEnd,
		EventGesture, EventProfileApplied, EventModeChanged, EventPadConnected, EventPadDisconnected, EventJump}
}

// Agents are the values a hook's agent filter accepts.
func Agents() []string { return []string{"claude", "codex", "opencode"} }

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9-]+$`)
	envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidName reports whether name is a valid script name (a-z, 0-9 and -).
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Validate checks one script definition.
func (s Script) Validate() error {
	switch {
	case len(s.Run) > 0 && s.Shell != "":
		return errors.New("set either run or shell, not both")
	case len(s.Run) == 0 && strings.TrimSpace(s.Shell) == "":
		return errors.New("needs run (a command and its arguments) or shell")
	case len(s.Run) > 0 && strings.TrimSpace(s.Run[0]) == "":
		return errors.New("run: the command (first item) is empty")
	case s.Timeout < 0:
		return fmt.Errorf("timeout %s is negative", s.Timeout)
	case s.Timeout > MaxTimeout:
		return fmt.Errorf("timeout %s is longer than %s", s.Timeout, MaxTimeout)
	}
	for _, k := range slices.Sorted(maps.Keys(s.Env)) {
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("env: %q is not a valid variable name", k)
		}
	}
	return nil
}

// EffectiveTimeout is Timeout, or DefaultTimeout when unset.
func (s Script) EffectiveTimeout() time.Duration {
	if s.Timeout <= 0 {
		return DefaultTimeout
	}
	return s.Timeout
}

// Preview is a one-line rendering of the command for the UI.
func (s Script) Preview() string {
	if s.Shell != "" {
		return "sh -c " + quoteArg(s.Shell)
	}
	parts := make([]string, len(s.Run))
	for i, a := range s.Run {
		parts[i] = quoteArg(a)
	}
	return strings.Join(parts, " ")
}

func quoteArg(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`;&|<>(){}*?!#") {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

// Clone returns a deep copy of s.
func (s Script) Clone() Script {
	s.Run = slices.Clone(s.Run)
	s.Env = maps.Clone(s.Env)
	return s
}

// Validate checks one hook against the known scripts (nil scripts: any name).
func (h Hook) Validate(scripts map[string]Script) error {
	if !slices.Contains(Events(), h.On) {
		return fmt.Errorf("on: %q is not an event (want %s)", h.On, strings.Join(Events(), " | "))
	}
	if h.Agent != "" && !slices.Contains(Agents(), h.Agent) {
		return fmt.Errorf("agent: %q is not an agent (want %s)", h.Agent, strings.Join(Agents(), " | "))
	}
	if h.Script == "" {
		return errors.New("script is missing")
	}
	if scripts != nil {
		if _, ok := scripts[h.Script]; !ok {
			return fmt.Errorf("script %q is not defined under scripts", h.Script)
		}
	}
	return nil
}

// String is a short description, e.g. "attention (claude) → ping-phone".
func (h Hook) String() string {
	on := h.On
	if h.Agent != "" {
		on += " (" + h.Agent + ")"
	}
	return on + " → " + h.Script
}

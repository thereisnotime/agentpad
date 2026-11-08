// Package events normalises coding-agent hook payloads and tracks sessions.
package events

import (
	"encoding/json"
	"time"
)

// Kind is a normalised lifecycle event kind.
type Kind string

const (
	KindSessionStart Kind = "session_start"
	KindPrompt       Kind = "prompt"        // user submitted a prompt, agent working
	KindToolPending  Kind = "tool_pending"  // waiting for user approval
	KindToolDone     Kind = "tool_done"     // tool ran (approved)
	KindWaitingInput Kind = "waiting_input" // agent idle, waiting for the user
	KindIdle         Kind = "idle"          // turn finished
	KindSessionEnd   Kind = "session_end"
	KindOther        Kind = "other"
)

// Agent identifiers accepted by the receiver and hook command.
const (
	AgentClaude   = "claude"
	AgentCodex    = "codex"
	AgentOpenCode = "opencode"
)

// Event is a normalised agent event.
type Event struct {
	Time      time.Time       `json:"time"`
	Agent     string          `json:"agent"`
	SessionID string          `json:"session_id"`
	Kind      Kind            `json:"kind"`
	Name      string          `json:"name"` // original event name, e.g. "PreToolUse"
	Tool      string          `json:"tool,omitempty"`
	Cwd       string          `json:"cwd,omitempty"`
	Message   string          `json:"message,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

// State is a session or aggregate state.
type State string

const (
	StateIdle      State = "idle"
	StateBusy      State = "busy"
	StateAttention State = "attention"
)

// Session is the tracked state of one agent session.
type Session struct {
	Agent     string
	SessionID string
	Cwd       string
	State     State
	LastTool  string
	LastEvent time.Time
}

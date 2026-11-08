package events

import (
	"bytes"
	"encoding/json"
)

// EventsV2Path is the v2 route prefix: POST /v2/events/<agent> with an Envelope body.
const EventsV2Path = "/v2/events/"

// Origin records where an agent runs so a jump can bring it back into focus.
// Every field is optional; the hook fills what its environment and /proc tell it.
type Origin struct {
	AgentPID    int    `json:"agent_pid,omitempty"`
	TTY         string `json:"tty,omitempty"` // e.g. /dev/pts/7
	TermProgram string `json:"term_program,omitempty"`
	// WindowID is the terminal's X11 window id as exported in $WINDOWID (decimal).
	// On Wayland Konsole sets it to a meaningless value; only trust it on X11.
	WindowID string `json:"window_id,omitempty"`

	KonsoleService string `json:"konsole_service,omitempty"` // $KONSOLE_DBUS_SERVICE, e.g. :1.123
	KonsoleWindow  string `json:"konsole_window,omitempty"`  // $KONSOLE_DBUS_WINDOW, e.g. /Windows/1
	KonsoleSession string `json:"konsole_session,omitempty"` // $KONSOLE_DBUS_SESSION, e.g. /Sessions/26

	ZellijSession string `json:"zellij_session,omitempty"` // $ZELLIJ_SESSION_NAME
	ZellijPane    string `json:"zellij_pane,omitempty"`    // $ZELLIJ_PANE_ID, numeric terminal pane id

	Tmux     string `json:"tmux,omitempty"`      // $TMUX: socket,pid,session
	TmuxPane string `json:"tmux_pane,omitempty"` // $TMUX_PANE, e.g. %3
}

// IsZero reports whether no field is set.
func (o Origin) IsZero() bool { return o == Origin{} }

// Envelope is the v2 request body: the agent's raw payload plus the origin.
type Envelope struct {
	Payload json.RawMessage `json:"payload"`
	Origin  *Origin         `json:"origin,omitempty"`
}

// NewEnvelope wraps payload. A payload that is not valid JSON (or empty) is
// carried as a JSON string so the envelope itself always stays valid.
func NewEnvelope(payload []byte, o Origin) Envelope {
	env := Envelope{}
	if len(bytes.TrimSpace(payload)) > 0 && json.Valid(payload) {
		env.Payload = append(json.RawMessage(nil), payload...)
	} else {
		b, _ := json.Marshal(string(payload))
		env.Payload = b
	}
	if !o.IsZero() {
		env.Origin = &o
	}
	return env
}

// payloadBytes returns the original payload bytes: raw JSON as-is, a JSON
// string unwrapped (the non-JSON fallback of NewEnvelope).
func (e Envelope) payloadBytes() []byte {
	p := bytes.TrimSpace(e.Payload)
	if len(p) > 0 && p[0] == '"' {
		var s string
		if json.Unmarshal(p, &s) == nil {
			return []byte(s)
		}
	}
	return p
}

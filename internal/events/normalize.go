package events

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const maxMessageRunes = 200

// Normalize converts a raw agent payload into an Event. Never fails; unknown
// payloads become KindOther with Raw kept.
func Normalize(agent string, payload []byte, now time.Time) Event {
	e := Event{Time: now, Agent: agent, Kind: KindOther}
	if json.Valid(payload) {
		e.Raw = append(json.RawMessage(nil), payload...)
	} else {
		b, _ := json.Marshal(strings.ToValidUTF8(string(payload), "�"))
		e.Raw = b
	}

	var m map[string]any
	_ = json.Unmarshal(payload, &m) // lenient: m stays nil on bad input

	switch agent {
	case AgentClaude:
		normClaude(&e, m)
	case AgentCodex:
		normCodex(&e, m)
	case AgentOpenCode:
		normOpenCode(&e, m)
	}
	if e.SessionID == "" {
		e.SessionID = agent + ":default"
	}
	return e
}

func normClaude(e *Event, m map[string]any) {
	e.Name = str(m, "hook_event_name")
	e.SessionID = str(m, "session_id")
	e.Cwd = str(m, "cwd")
	e.Tool = str(m, "tool_name")
	e.Message = truncate(str(m, "message"))

	switch e.Name {
	case "SessionStart":
		e.Kind = KindSessionStart
	case "UserPromptSubmit":
		e.Kind = KindPrompt
	case "PermissionRequest":
		e.Kind = KindToolPending
	case "PostToolUse":
		e.Kind = KindToolDone
	case "Stop":
		e.Kind = KindIdle
	case "SessionEnd":
		e.Kind = KindSessionEnd
	case "Notification":
		switch str(m, "notification_type") {
		case "permission_prompt":
			e.Kind = KindToolPending
		case "idle_prompt", "agent_needs_input":
			e.Kind = KindWaitingInput
		case "":
			msg := strings.ToLower(e.Message)
			switch {
			case strings.Contains(msg, "permission"):
				e.Kind = KindToolPending
			case strings.Contains(msg, "waiting for your input"):
				e.Kind = KindWaitingInput
			}
		}
	}
	// PreToolUse, SubagentStop and everything else stay KindOther.
}

func normCodex(e *Event, m map[string]any) {
	typ := str(m, "type")
	e.Name = typ
	e.SessionID = first(str(m, "thread-id"), str(m, "session-id"), str(m, "conversation-id"))
	e.Cwd = str(m, "cwd")
	e.Message = truncate(str(m, "last-assistant-message"))
	switch {
	case typ == "agent-turn-complete":
		e.Kind = KindIdle
	case strings.Contains(typ, "approval"):
		e.Kind = KindToolPending
	}
}

func normOpenCode(e *Event, m map[string]any) {
	typ := str(m, "type")
	e.Name = typ
	props := obj(m, "properties")
	if props == nil {
		props = obj(m, "data") // v2 event envelope
	}
	info := obj(props, "info")
	e.SessionID = first(str(props, "sessionID"), str(info, "id"))
	e.Cwd = str(info, "directory")

	switch typ {
	case "session.created":
		e.Kind = KindSessionStart
	case "session.status":
		status := str(props, "status") // tolerate a plain string
		if s := obj(props, "status"); s != nil {
			status = str(s, "type")
		}
		switch status {
		case "busy", "retry":
			e.Kind = KindPrompt
		case "idle":
			e.Kind = KindIdle
		}
	case "session.idle":
		e.Kind = KindIdle
	case "permission.updated", "permission.asked":
		e.Kind = KindToolPending
		e.Tool = first(str(props, "permission"), str(props, "type"))
		e.Message = truncate(str(props, "title"))
	case "permission.replied":
		e.Kind = KindToolDone
	case "session.deleted":
		e.Kind = KindSessionEnd
	}
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func obj(m map[string]any, k string) map[string]any {
	o, _ := m[k].(map[string]any)
	return o
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string) string {
	if utf8.RuneCountInString(s) <= maxMessageRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxMessageRunes])
}

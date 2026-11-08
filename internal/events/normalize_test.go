package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTestdata(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		file, agent string
		kind        Kind
		name, sid   string
		tool, cwd   string
	}{
		{"claude_session_start.json", AgentClaude, KindSessionStart, "SessionStart", "abc123", "", "/home/u/proj"},
		{"claude_prompt.json", AgentClaude, KindPrompt, "UserPromptSubmit", "abc123", "", "/home/u/proj"},
		{"claude_permission_request.json", AgentClaude, KindToolPending, "PermissionRequest", "abc123", "Bash", "/home/u/proj"},
		{"claude_notification_permission.json", AgentClaude, KindToolPending, "Notification", "abc123", "", "/home/u/proj"},
		{"claude_notification_idle.json", AgentClaude, KindWaitingInput, "Notification", "abc123", "", "/home/u/proj"},
		{"claude_notification_legacy.json", AgentClaude, KindToolPending, "Notification", "abc123", "", "/home/u/proj"},
		{"claude_pre_tool_use.json", AgentClaude, KindOther, "PreToolUse", "abc123", "Bash", "/home/u/proj"},
		{"claude_post_tool_use.json", AgentClaude, KindToolDone, "PostToolUse", "abc123", "Write", "/home/u/proj"},
		{"claude_stop.json", AgentClaude, KindIdle, "Stop", "abc123", "", "/home/u/proj"},
		{"claude_session_end.json", AgentClaude, KindSessionEnd, "SessionEnd", "abc123", "", "/home/u/proj"},
		{"codex_turn_complete.json", AgentCodex, KindIdle, "agent-turn-complete", "b5f6c1c2-1111-4222-8333-944455556666", "", "/home/u/proj"},
		{"codex_approval.json", AgentCodex, KindToolPending, "approval-requested", "conv-9", "", "/home/u/proj"},
		{"opencode_session_created.json", AgentOpenCode, KindSessionStart, "session.created", "ses_01", "", "/home/u/proj"},
		{"opencode_status_busy.json", AgentOpenCode, KindPrompt, "session.status", "ses_01", "", ""},
		{"opencode_status_idle.json", AgentOpenCode, KindIdle, "session.status", "ses_01", "", ""},
		{"opencode_session_idle.json", AgentOpenCode, KindIdle, "session.idle", "ses_01", "", ""},
		{"opencode_permission_asked.json", AgentOpenCode, KindToolPending, "permission.asked", "ses_01", "bash", ""},
		{"opencode_permission_updated.json", AgentOpenCode, KindToolPending, "permission.updated", "ses_01", "edit", ""},
		{"opencode_permission_replied.json", AgentOpenCode, KindToolDone, "permission.replied", "ses_01", "", ""},
		{"opencode_session_deleted.json", AgentOpenCode, KindSessionEnd, "session.deleted", "ses_01", "", "/home/u/proj"},
		{"opencode_file_edited.json", AgentOpenCode, KindOther, "file.edited", "opencode:default", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			e := Normalize(tc.agent, b, now)
			if e.Kind != tc.kind || e.Name != tc.name || e.SessionID != tc.sid || e.Tool != tc.tool || e.Cwd != tc.cwd {
				t.Errorf("got kind=%s name=%q sid=%q tool=%q cwd=%q", e.Kind, e.Name, e.SessionID, e.Tool, e.Cwd)
			}
			if !e.Time.Equal(now) || e.Agent != tc.agent {
				t.Errorf("time/agent not set: %v %q", e.Time, e.Agent)
			}
			if string(e.Raw) != string(b) {
				t.Errorf("raw not kept")
			}
		})
	}
}

func TestNormalizeEdgeCases(t *testing.T) {
	now := time.Now()
	long := strings.Repeat("é", 300)
	tests := []struct {
		name    string
		agent   string
		payload string
		kind    Kind
		sid     string
		msgLen  int
	}{
		{"garbage", AgentClaude, "not json \xff", KindOther, "claude:default", 0},
		{"empty", AgentClaude, "", KindOther, "claude:default", 0},
		{"unknown agent", "aider", `{"hook_event_name":"Stop","session_id":"x"}`, KindOther, "aider:default", 0},
		{"unknown claude event", AgentClaude, `{"hook_event_name":"FutureThing","session_id":"s"}`, KindOther, "s", 0},
		{"subagent stop", AgentClaude, `{"hook_event_name":"SubagentStop","session_id":"s"}`, KindOther, "s", 0},
		{"notification other type", AgentClaude, `{"hook_event_name":"Notification","notification_type":"auth_success","message":"permission granted"}`, KindOther, "claude:default", 18},
		{"notification no type waiting", AgentClaude, `{"hook_event_name":"Notification","message":"Claude is waiting for your input"}`, KindWaitingInput, "claude:default", 32},
		{"notification no type other", AgentClaude, `{"hook_event_name":"Notification","message":"hello"}`, KindOther, "claude:default", 5},
		{"wrong field types", AgentClaude, `{"hook_event_name":5,"session_id":["x"]}`, KindOther, "claude:default", 0},
		{"codex session-id fallback", AgentCodex, `{"type":"x","session-id":"s1"}`, KindOther, "s1", 0},
		{"codex truncate", AgentCodex, `{"type":"agent-turn-complete","last-assistant-message":"` + long + `"}`, KindIdle, "codex:default", 200},
		{"codex array payload", AgentCodex, `[1,2]`, KindOther, "codex:default", 0},
		{"opencode string status", AgentOpenCode, `{"type":"session.status","properties":{"sessionID":"s","status":"busy"}}`, KindPrompt, "s", 0},
		{"opencode v2 data envelope", AgentOpenCode, `{"type":"session.idle","data":{"sessionID":"s2"}}`, KindIdle, "s2", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Normalize(tc.agent, []byte(tc.payload), now)
			if e.Kind != tc.kind || e.SessionID != tc.sid {
				t.Errorf("got kind=%s sid=%q", e.Kind, e.SessionID)
			}
			if n := len([]rune(e.Message)); n != tc.msgLen {
				t.Errorf("message runes = %d, want %d", n, tc.msgLen)
			}
			if !json.Valid(e.Raw) {
				t.Errorf("raw is not valid JSON: %q", e.Raw)
			}
			if _, err := json.Marshal(e); err != nil {
				t.Errorf("event does not marshal: %v", err)
			}
		})
	}
}

func TestNormalizeGarbageRawIsString(t *testing.T) {
	e := Normalize(AgentClaude, []byte("hello"), time.Now())
	var s string
	if err := json.Unmarshal(e.Raw, &s); err != nil || s != "hello" {
		t.Fatalf("raw = %s, err %v", e.Raw, err)
	}
}

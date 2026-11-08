package events

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	o := Origin{AgentPID: 42, TTY: "/dev/pts/7", KonsoleService: ":1.80", KonsoleWindow: "/Windows/1",
		KonsoleSession: "/Sessions/26", ZellijSession: "main", ZellijPane: "3", Tmux: "/tmp/tmux-1000/default,1,0", TmuxPane: "%2"}
	tests := []struct {
		name    string
		payload string
		origin  Origin
	}{
		{"json", `{"hook_event_name":"Stop","session_id":"s"}`, o},
		{"json no origin", `{"a":1}`, Origin{}},
		{"not json", `hello "world"`, o},
		{"empty", ``, o},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(NewEnvelope([]byte(tc.payload), tc.origin))
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(b) {
				t.Fatalf("invalid json %s", b)
			}
			var got Envelope
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if string(got.payloadBytes()) != tc.payload {
				t.Fatalf("payload %q want %q", got.payloadBytes(), tc.payload)
			}
			if tc.origin.IsZero() != (got.Origin == nil) {
				t.Fatalf("origin presence wrong: %s", b)
			}
			if got.Origin != nil && *got.Origin != tc.origin {
				t.Fatalf("origin %+v", *got.Origin)
			}
		})
	}
	if b, _ := json.Marshal(o); !strings.Contains(string(b), `"zellij_pane":"3"`) || !strings.Contains(string(b), `"agent_pid":42`) {
		t.Fatalf("json names changed: %s", b)
	}
}

func TestServerV2(t *testing.T) {
	_, base, got := startTestServer(t)
	c := &http.Client{Timeout: 2 * time.Second}
	origin := Origin{AgentPID: 7, KonsoleSession: "/Sessions/3", ZellijPane: "2"}
	env, _ := json.Marshal(NewEnvelope([]byte(`{"hook_event_name":"Notification","notification_type":"permission_prompt","session_id":"s1","cwd":"/w"}`), origin))
	noOrigin, _ := json.Marshal(NewEnvelope([]byte(`{"type":"agent-turn-complete","thread-id":"t1"}`), Origin{}))
	tests := []struct {
		path, body string
		status     int
	}{
		{"/v2/events/claude", string(env), 204},
		{"/v2/events/codex", string(noOrigin), 204},
		{"/v2/events/opencode", `{"type":"session.idle","properties":{"sessionID":"o1"}}`, 204}, // bare payload falls back
		{"/v1/events/claude", `{"hook_event_name":"Stop","session_id":"s2"}`, 204},
		{"/v2/events/aider", string(env), 404},
		{"/v2/events/claude", strings.Repeat("x", maxBodyV2+10), 413},
	}
	for _, tc := range tests {
		resp, err := c.Post(base+tc.path, "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("%s: status %d want %d", tc.path, resp.StatusCode, tc.status)
		}
	}
	ev := got()
	if len(ev) != 4 {
		t.Fatalf("got %d events: %+v", len(ev), ev)
	}
	if ev[0].Kind != KindToolPending || ev[0].SessionID != "s1" || ev[0].Cwd != "/w" || ev[0].Origin == nil || *ev[0].Origin != origin {
		t.Fatalf("v2 claude = %+v", ev[0])
	}
	if ev[1].Kind != KindIdle || ev[1].SessionID != "t1" || ev[1].Origin != nil {
		t.Fatalf("v2 codex = %+v", ev[1])
	}
	if ev[2].SessionID != "o1" || ev[2].Origin != nil {
		t.Fatalf("v2 bare = %+v", ev[2])
	}
	if ev[3].SessionID != "s2" || ev[3].Origin != nil {
		t.Fatalf("v1 = %+v", ev[3])
	}
}

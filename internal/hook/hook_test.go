package hook

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
)

type recv struct {
	mu    sync.Mutex
	paths []string
	body  []string
}

func newRecv(t *testing.T) (*recv, string) {
	r := &recv{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.paths = append(r.paths, req.Method+" "+req.URL.Path)
		r.body = append(r.body, string(b))
		r.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return r, strings.TrimPrefix(srv.URL, "http://")
}

func TestRunForwards(t *testing.T) {
	claude := `{"hook_event_name":"Stop","session_id":"s"}`
	codex := `{"type":"agent-turn-complete","thread-id":"t"}`
	opencode := `{"type":"session.idle","properties":{"sessionID":"o"}}`
	tests := []struct {
		name, agent string
		args        []string
		stdin       string
		wantPath    string
		wantBody    string
	}{
		{"claude stdin", "claude", nil, claude, "POST /v2/events/claude", claude},
		{"opencode stdin", "opencode", nil, opencode, "POST /v2/events/opencode", opencode},
		{"codex argv", "codex", []string{"--flag", codex}, "ignored", "POST /v2/events/codex", codex},
		{"codex stdin fallback", "codex", nil, codex, "POST /v2/events/codex", codex},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, addr := newRecv(t)
			if code := Run(tc.agent, tc.args, strings.NewReader(tc.stdin), addr); code != 0 {
				t.Fatalf("exit %d", code)
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.paths) != 1 || r.paths[0] != tc.wantPath {
				t.Fatalf("got %v %v", r.paths, r.body)
			}
			var env struct {
				Payload json.RawMessage `json:"payload"`
				Origin  *events.Origin  `json:"origin"`
			}
			if err := json.Unmarshal([]byte(r.body[0]), &env); err != nil || string(env.Payload) != tc.wantBody {
				t.Fatalf("body %s (%v)", r.body[0], err)
			}
			if env.Origin == nil || env.Origin.AgentPID == 0 {
				t.Fatalf("no origin in %s", r.body[0])
			}
		})
	}
}

func TestRunReceiverDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	start := time.Now()
	if code := Run("claude", nil, strings.NewReader(`{}`), addr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestRunSlowReceiver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(800 * time.Millisecond)
	}))
	defer srv.Close()
	start := time.Now()
	if code := Run("claude", nil, strings.NewReader(`{}`), strings.TrimPrefix(srv.URL, "http://")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestRunStdinNeverCloses(t *testing.T) {
	r, addr := newRecv(t)
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	start := time.Now()
	if code := Run("claude", nil, pr, addr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.paths) != 1 {
		t.Fatalf("expected an (empty) post, got %v", r.paths)
	}
}

func TestRunUnknownAgent(t *testing.T) {
	var buf bytes.Buffer
	old := stderr
	stderr = &buf
	defer func() { stderr = old }()
	if code := Run("aider", nil, strings.NewReader(""), "127.0.0.1:1"); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "usage") {
		t.Fatalf("stderr = %q", buf.String())
	}
}

package events

import (
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func startTestServer(t *testing.T) (*Server, string, func() []Event) {
	t.Helper()
	var mu sync.Mutex
	var got []Event
	s := NewServer("127.0.0.1:0", func(e Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, "http://" + s.boundAddr(), func() []Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]Event(nil), got...)
	}
}

func TestServerRoutes(t *testing.T) {
	_, base, events := startTestServer(t)
	c := &http.Client{Timeout: 2 * time.Second}

	resp, err := c.Get(base + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "ok" {
		t.Fatalf("health = %d %q", resp.StatusCode, body)
	}

	tests := []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/v1/events/claude", `{"hook_event_name":"Notification","notification_type":"permission_prompt","session_id":"s1"}`, 204},
		{"POST", "/v1/events/codex", `{"type":"agent-turn-complete","thread-id":"t1"}`, 204},
		{"POST", "/v1/events/opencode", `{"type":"session.idle","properties":{"sessionID":"o1"}}`, 204},
		{"POST", "/v1/events/aider", `{}`, 404},
		{"GET", "/v1/events/claude", "", 405},
		{"POST", "/v1/events/claude", strings.Repeat("x", 1<<20+10), 413},
	}
	for _, tc := range tests {
		req, _ := http.NewRequest(tc.method, base+tc.path, strings.NewReader(tc.body))
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("%s %s: status %d want %d", tc.method, tc.path, resp.StatusCode, tc.status)
		}
	}

	got := events()
	if len(got) != 3 {
		t.Fatalf("got %d events", len(got))
	}
	if got[0].Kind != KindToolPending || got[0].SessionID != "s1" || got[1].Kind != KindIdle || got[2].SessionID != "o1" {
		t.Fatalf("events = %+v", got)
	}
}

func TestServerRejectsNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", ":0", "192.168.1.10:47321", "example.com:80", "nonsense"} {
		if err := NewServer(addr, nil).Start(); err == nil {
			t.Errorf("%q: expected error", addr)
		}
	}
	for _, addr := range []string{"localhost:0", "[::1]:0"} {
		s := NewServer(addr, nil)
		err := s.Start()
		if err != nil && !strings.Contains(err.Error(), "listen") {
			t.Errorf("%q: unexpected error %v", addr, err)
		}
		_ = s.Close()
	}
}

func TestServerPortBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if err := NewServer(ln.Addr().String(), nil).Start(); err == nil {
		t.Fatal("expected busy port error")
	}
}

func TestServerCloseIdempotent(t *testing.T) {
	s, base, _ := startTestServer(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Timeout: 500 * time.Millisecond}
	if _, err := c.Get(base + "/v1/health"); err == nil {
		t.Fatal("server still answering after close")
	}
}

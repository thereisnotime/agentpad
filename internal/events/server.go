package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/config"
)

const (
	maxBody = 1 << 20
	// maxBodyV2 leaves room for the envelope and for an escaped non-JSON payload.
	maxBodyV2 = 2*maxBody + 64<<10
)

// Server receives events on POST /v1/events/{agent} (raw agent payload) and
// POST /v2/events/{agent} (an Envelope with payload + origin), loopback only,
// and answers GET /v1/health with 200 "ok".
type Server struct {
	addr    string
	onEvent func(Event)

	mu    sync.Mutex
	srv   *http.Server
	bound net.Addr
}

// NewServer creates a server; onEvent is called for every normalised event.
func NewServer(addr string, onEvent func(Event)) *Server {
	if addr == "" {
		addr = config.ListenAddr
	}
	if onEvent == nil {
		onEvent = func(Event) {}
	}
	return &Server{addr: addr, onEvent: onEvent}
}

// Start listens and serves in the background; returns an error if the port is busy
// or addr is not a loopback address.
func (s *Server) Start() error {
	host, _, err := net.SplitHostPort(s.addr)
	if err != nil {
		return fmt.Errorf("events: bad listen address %q: %w", s.addr, err)
	}
	if !isLoopback(host) {
		return fmt.Errorf("events: refusing non-loopback listen address %q", s.addr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		return errors.New("events: server already started")
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("events: listen %s: %w", s.addr, err)
	}
	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	s.srv, s.bound = srv, ln.Addr()
	go func() { _ = srv.Serve(ln) }()
	return nil
}

// Close gracefully shuts the server down (1s timeout).
func (s *Server) Close() error {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return srv.Close()
	}
	return nil
}

// boundAddr returns the actual listen address (useful with port 0).
func (s *Server) boundAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bound == nil {
		return ""
	}
	return s.bound.String()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+config.EventsPath+"{agent}", s.handleEvent)
	mux.HandleFunc("POST "+EventsV2Path+"{agent}", s.handleEventV2)
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	})
	return mux
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	agent, body, ok := readEvent(w, r, maxBody)
	if !ok {
		return
	}
	s.onEvent(Normalize(agent, body, time.Now()))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEventV2(w http.ResponseWriter, r *http.Request) {
	agent, body, ok := readEvent(w, r, maxBodyV2)
	if !ok {
		return
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil || env.Payload == nil {
		// Not an envelope: treat the whole body as the payload, like v1.
		s.onEvent(Normalize(agent, body, time.Now()))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	e := Normalize(agent, env.payloadBytes(), time.Now())
	if env.Origin != nil && !env.Origin.IsZero() {
		o := *env.Origin
		e.Origin = &o
	}
	s.onEvent(e)
	w.WriteHeader(http.StatusNoContent)
}

func readEvent(w http.ResponseWriter, r *http.Request, limit int64) (string, []byte, bool) {
	agent := r.PathValue("agent")
	switch agent {
	case AgentClaude, AgentCodex, AgentOpenCode:
	default:
		http.NotFound(w, r)
		return "", nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		http.Error(w, "payload too large or unreadable", http.StatusRequestEntityTooLarge)
		return "", nil, false
	}
	return agent, body, true
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

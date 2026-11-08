package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/config"
)

const maxBody = 1 << 20

// Server receives events on POST /v1/events/{agent} (loopback only) and
// answers GET /v1/health with 200 "ok".
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
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	})
	return mux
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	agent := r.PathValue("agent")
	switch agent {
	case AgentClaude, AgentCodex, AgentOpenCode:
	default:
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "payload too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	s.onEvent(Normalize(agent, body, time.Now()))
	w.WriteHeader(http.StatusNoContent)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

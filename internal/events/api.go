package events

import "time"

// Stubs: implemented by the events work (replace bodies, keep signatures).

// Normalize converts a raw agent payload into an Event. Never fails; unknown
// payloads become KindOther with Raw kept.
func Normalize(agent string, payload []byte, now time.Time) Event { panic("TODO") }

// Tracker keeps per-session state. Safe for concurrent use.
type Tracker struct{}

func NewTracker() *Tracker { panic("TODO") }

// Apply updates session state; returns true if the aggregate state changed.
func (t *Tracker) Apply(e Event) bool { panic("TODO") }

// Sessions returns active sessions, most recent first.
func (t *Tracker) Sessions() []Session { panic("TODO") }

// Aggregate returns attention > busy > idle across sessions.
func (t *Tracker) Aggregate() State { panic("TODO") }

// Prune drops sessions with no events for longer than d; returns true if aggregate changed.
func (t *Tracker) Prune(d time.Duration, now time.Time) bool { panic("TODO") }

// Log is an append-only JSON Lines event log. Safe for concurrent use.
type Log struct{}

// OpenLog opens (creating dirs as needed) the log at path.
func OpenLog(path string) (*Log, error) { panic("TODO") }
func (l *Log) Append(e Event) error     { panic("TODO") }

// Tail returns up to n most recent events, oldest first.
func (l *Log) Tail(n int) ([]Event, error) { panic("TODO") }
func (l *Log) Close() error                { panic("TODO") }

// Server receives events on POST /v1/events/{agent} (loopback only) and
// answers GET /v1/health with 200 "ok".
type Server struct{}

// NewServer creates a server; onEvent is called for every normalised event.
func NewServer(addr string, onEvent func(Event)) *Server { panic("TODO") }

// Start listens and serves in the background; returns an error if the port is busy
// or addr is not a loopback address.
func (s *Server) Start() error { panic("TODO") }
func (s *Server) Close() error { panic("TODO") }

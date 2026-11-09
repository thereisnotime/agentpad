package gesture

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/evdev"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// Source delivers raw key events from the pad (Linux keycodes).
type Source interface {
	// Read calls fn for every key press/release until ctx is done or the
	// source fails (e.g. the pad was unplugged).
	Read(ctx context.Context, fn func(code uint16, down bool, t time.Time)) error
	// Close releases the source (ungrabs the pad).
	Close() error
}

// Opener opens a Source; called on Start and again after a disconnect.
type Opener func() (Source, error)

// Emitter sends an action to the focused application (vkbd.Emitter fits).
type Emitter interface {
	Emit(action string) error
}

// GestureEmitter is an Emitter that also wants to know which gesture an
// action belongs to (e.g. to pass it to a script). The engine calls
// EmitGesture instead of Emit when the emitter implements it.
type GestureEmitter interface {
	Emitter
	EmitGesture(g Gesture, action string) error
}

// EvdevSource opens and grabs every input node of the pad.
func EvdevSource() Opener {
	return func() (Source, error) {
		p, err := evdev.OpenPad()
		if err != nil {
			return nil, err
		}
		return evdevSource{p}, nil
	}
}

type evdevSource struct{ p *evdev.Pad }

func (s evdevSource) Read(ctx context.Context, fn func(uint16, bool, time.Time)) error {
	return s.p.Read(ctx, func(ev evdev.KeyEvent) { fn(ev.Code, ev.Down, ev.Time) })
}

func (s evdevSource) Close() error { return s.p.Close() }

// Status is the engine's connection state.
type Status string

const (
	StatusRunning      Status = "running"      // pad grabbed, gestures recognised
	StatusDisconnected Status = "disconnected" // pad gone; retrying every RetryInterval
	StatusStopped      Status = "stopped"      // engine finished
)

// Event is what the engine reports for a live monitor. Exactly one of:
//   - a raw control press/release (Input non-nil), for animating the pad;
//   - a recognised gesture (Input nil, Status empty), with the Action emitted;
//   - a status change (Status set, Err the cause of a disconnect).
type Event struct {
	Time    time.Time
	Input   *Input
	Gesture Gesture
	Action  string // action emitted for Gesture, "" if nothing is mapped
	Err     error  // emit error, or why the pad disconnected
	Status  Status
}

// IsGesture reports whether ev is a recognised gesture.
func (ev Event) IsGesture() bool { return ev.Input == nil && ev.Status == "" }

// RetryInterval is how often a disconnected pad is re-opened.
const RetryInterval = time.Second

// eventBuffer is the size of the Events channel; extra events are dropped.
const eventBuffer = 64

// ErrStarted is returned by Start on an engine that already ran.
var ErrStarted = errors.New("gesture engine already started")

// Engine runs gesture mode: Source -> Recognizer -> profile -> Emitter.
// An Engine runs once; create a new one to restart gesture mode.
type Engine struct {
	open Opener
	em   Emitter
	cfg  Config

	retry time.Duration
	now   func() time.Time

	mu      sync.Mutex
	prof    profile.Profile
	started bool

	events    chan Event
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// NewEngine returns an engine; cfg.Has is ignored (it is derived from the
// profile set with SetProfile).
func NewEngine(open Opener, em Emitter, cfg Config) *Engine {
	return &Engine{
		open: open, em: em, cfg: cfg,
		retry:  RetryInterval,
		now:    time.Now,
		events: make(chan Event, eventBuffer),
		done:   make(chan struct{}),
	}
}

// SetProfile sets the profile used to look up actions. Safe at any time.
func (e *Engine) SetProfile(p profile.Profile) {
	e.mu.Lock()
	e.prof = p
	e.mu.Unlock()
}

// Profile returns the current profile.
func (e *Engine) Profile() profile.Profile {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prof
}

// Events returns the monitor channel. It is closed when the engine stops.
func (e *Engine) Events() <-chan Event { return e.events }

// Config returns the effective thresholds.
func (e *Engine) Config() Config { return e.cfg.withDefaults() }

func (e *Engine) action(c Control, k Kind) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prof.GestureAction(string(c), string(k))
}

func (e *Engine) has(c Control, k Kind) bool {
	_, ok := e.action(c, k)
	return ok
}

// Start opens the source (returning its error, e.g. evdev.ErrPermission or
// evdev.ErrNotFound) and runs the engine until ctx is done or Stop is called.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return ErrStarted
	}
	e.started = true
	e.mu.Unlock()

	src, err := e.open()
	if err != nil {
		e.finish()
		close(e.done)
		return err
	}
	ctx, e.cancel = context.WithCancel(ctx)
	go func() {
		defer close(e.done)
		defer e.finish()
		e.run(ctx, src)
	}()
	return nil
}

// Stop ends gesture mode, releases the pad and closes Events. Idempotent;
// the Emitter is left open for the caller to close.
func (e *Engine) Stop() {
	e.mu.Lock()
	started := e.started
	e.started = true // a stopped engine cannot be started
	e.mu.Unlock()
	if !started {
		e.finish()
		return
	}
	if e.cancel != nil {
		e.cancel()
	}
	<-e.done
}

func (e *Engine) finish() {
	e.closeOnce.Do(func() {
		e.send(Event{Time: e.now(), Status: StatusStopped})
		close(e.events)
	})
}

func (e *Engine) send(ev Event) {
	select {
	case e.events <- ev:
	default: // nobody is watching closely enough; never block the engine
	}
}

func (e *Engine) run(ctx context.Context, src Source) {
	for {
		err := e.session(ctx, src)
		_ = src.Close()
		if ctx.Err() != nil {
			return
		}
		e.send(Event{Time: e.now(), Status: StatusDisconnected, Err: err})
		src = reopen(ctx, e.open, e.retry)
		if src == nil {
			return
		}
	}
}

// session reads one source until it fails or ctx is done.
func (e *Engine) session(ctx context.Context, src Source) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	inputs := make(chan Input, 64)
	readErr := make(chan error, 1)
	go func() {
		readErr <- src.Read(ctx, func(code uint16, down bool, t time.Time) {
			c, ok := ControlForKey(code)
			if !ok {
				return
			}
			select {
			case inputs <- Input{Control: c, Down: down, Time: t}:
			case <-ctx.Done():
			}
		})
	}()

	cfg := e.cfg
	cfg.Has = e.has
	rec := NewRecognizer(cfg)
	e.send(Event{Time: e.now(), Status: StatusRunning})

	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		var timerC <-chan time.Time
		var due time.Time
		if d, ok := rec.Next(); ok {
			due = d
			timer.Reset(max(d.Sub(e.now()), 0))
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			e.dispatch(rec.Tick(e.now()))
			if err == nil {
				err = evdev.ErrDisconnected
			}
			return err
		case in := <-inputs:
			raw := in
			e.send(Event{Time: in.Time, Input: &raw})
			e.dispatch(rec.Feed(in))
		case <-timerC:
			now := e.now()
			if now.Before(due) {
				now = due
			}
			e.dispatch(rec.Tick(now))
		}
		if timerC != nil && !timer.Stop() {
			select { // drain a fired-but-unread timer before Reset
			case <-timer.C:
			default:
			}
		}
	}
}

func (e *Engine) dispatch(gs []Gesture) {
	for _, g := range gs {
		ev := Event{Time: g.Time, Gesture: g}
		if a, ok := e.action(g.Control, g.Kind); ok {
			ev.Action = a
			if ge, ok := e.em.(GestureEmitter); ok {
				ev.Err = ge.EmitGesture(g, a)
			} else {
				ev.Err = e.em.Emit(a)
			}
		}
		e.send(ev)
	}
}

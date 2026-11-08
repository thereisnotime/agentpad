package gesture

import (
	"context"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/evdev"
)

// EvdevObserveSource opens the pad's input nodes without grabbing them, so
// keys keep reaching the focused application (plain mode).
func EvdevObserveSource() Opener {
	return func() (Source, error) {
		p, err := evdev.ObservePad()
		if err != nil {
			return nil, err
		}
		return evdevSource{p}, nil
	}
}

// ControlEvent is a raw control press/release (no gesture recognition), or
// a status change when Status is set.
type ControlEvent struct {
	Control Control
	Down    bool
	Time    time.Time
	Code    uint16 // Linux keycode it came from
	Status  Status
	Err     error
}

// Observer streams raw control events from the pad, e.g. to animate the pad
// drawing. With EvdevObserveSource it works in plain mode without taking
// keys away from other apps; codes are mapped with the KeyMap (PlainKeyMap
// for the active profile, GestureKeyMap in gesture mode). Unmapped codes
// (modifiers of a combo, keys of a sequence) are dropped. While a gesture
// Engine holds the grab, an observer sees nothing; use Engine.Events (it
// carries raw inputs too) instead. An Observer runs once.
type Observer struct {
	open  Opener
	retry time.Duration
	now   func() time.Time

	mu      sync.Mutex
	km      KeyMap
	started bool

	events    chan ControlEvent
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// NewObserver returns an observer using km (copied).
func NewObserver(open Opener, km KeyMap) *Observer {
	o := &Observer{open: open, retry: RetryInterval, now: time.Now,
		events: make(chan ControlEvent, eventBuffer), done: make(chan struct{})}
	o.SetKeyMap(km)
	return o
}

// SetKeyMap replaces the keycode map (e.g. after the profile changed).
func (o *Observer) SetKeyMap(km KeyMap) {
	cp := make(KeyMap, len(km))
	for k, v := range km {
		cp[k] = v
	}
	o.mu.Lock()
	o.km = cp
	o.mu.Unlock()
}

func (o *Observer) lookup(code uint16) (Control, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	c, ok := o.km[code]
	return c, ok
}

// Events returns the stream; it is closed when the observer stops.
func (o *Observer) Events() <-chan ControlEvent { return o.events }

// Start opens the source (returning its error) and streams until ctx is
// done or Stop is called. A disconnected pad is re-opened automatically.
func (o *Observer) Start(ctx context.Context) error {
	o.mu.Lock()
	if o.started {
		o.mu.Unlock()
		return ErrStarted
	}
	o.started = true
	o.mu.Unlock()
	src, err := o.open()
	if err != nil {
		o.finish()
		close(o.done)
		return err
	}
	ctx, o.cancel = context.WithCancel(ctx)
	go func() {
		defer close(o.done)
		defer o.finish()
		o.run(ctx, src)
	}()
	return nil
}

// Stop ends the stream and closes the source. Idempotent.
func (o *Observer) Stop() {
	o.mu.Lock()
	started := o.started
	o.started = true
	o.mu.Unlock()
	if !started {
		o.finish()
		return
	}
	if o.cancel != nil {
		o.cancel()
	}
	<-o.done
}

func (o *Observer) send(ev ControlEvent) {
	select {
	case o.events <- ev:
	default:
	}
}

func (o *Observer) finish() {
	o.closeOnce.Do(func() {
		o.send(ControlEvent{Time: o.now(), Status: StatusStopped})
		close(o.events)
	})
}

func (o *Observer) run(ctx context.Context, src Source) {
	for {
		o.send(ControlEvent{Time: o.now(), Status: StatusRunning})
		err := src.Read(ctx, func(code uint16, down bool, t time.Time) {
			if c, ok := o.lookup(code); ok {
				o.send(ControlEvent{Control: c, Down: down, Time: t, Code: code})
			}
		})
		_ = src.Close()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = evdev.ErrDisconnected
		}
		o.send(ControlEvent{Time: o.now(), Status: StatusDisconnected, Err: err})
		if src = reopen(ctx, o.open, o.retry); src == nil {
			return
		}
	}
}

// reopen retries open every interval until it succeeds or ctx is done.
func reopen(ctx context.Context, open Opener, every time.Duration) Source {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if src, err := open(); err == nil {
				return src
			}
		}
	}
}

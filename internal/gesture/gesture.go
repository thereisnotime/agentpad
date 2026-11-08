// Package gesture turns raw pad key events into gestures (tap, hold, long
// hold, double-tap, chord, knob rotate and press+rotate) and runs gesture
// mode: read the pad, recognise, look up the profile, emit the action.
//
// # API for the UI
//
// Gesture mode in short: program the pad with profile.GestureModeMapping(o)
// (every control sends a unique key, F13..F18), then run an Engine:
//
//	em, err := vkbd.New()                       // uinput, else ydotool
//	eng := gesture.NewEngine(gesture.EvdevSource(), em, gesture.DefaultConfig())
//	eng.SetProfile(activeProfile)               // any time, safe from any goroutine
//	if err := eng.Start(ctx); err != nil { ... } // e.g. evdev.ErrPermission, evdev.ErrNotFound
//	for ev := range eng.Events() { ... }        // live monitor, closed after Stop
//	eng.Stop()                                  // ungrabs the pad, closes Events
//
// Engine.Events delivers Event values: a raw control press/release (Input
// non-nil, for animating the pad), a recognised Gesture (ev.IsGesture())
// with the Action that was emitted (empty if nothing is mapped) and any emit
// error, or a Status change (StatusRunning, StatusDisconnected while the pad
// is gone; the engine re-opens it automatically). Events are dropped rather
// than blocking the engine when nobody reads them.
//
// Plain mode (no gesture mode) can still show live key presses with an
// Observer, which reads the pad WITHOUT grabbing it, so keys keep reaching
// the focused app:
//
//	km, rep, err := gesture.PlainKeyMap(activeProfile, orientation)
//	if rep.Ambiguous() { ... }                  // warn: rep.Collisions, rep.Unmapped
//	obs := gesture.NewObserver(gesture.EvdevObserveSource(), km)
//	if err := obs.Start(ctx); err != nil { ... }
//	for ev := range obs.Events() { ... }        // ControlEvent{Control, Down, Time} or Status
//	obs.SetKeyMap(newKm)                        // after a profile change
//	obs.Stop()
//
// While an Engine holds the grab the Observer sees nothing; use the raw
// Input events of Engine.Events instead.
//
// Profiles configure gestures with Profile.Gestures (control -> kind ->
// action, names in internal/profile/gestures.go). A tap falls back to the
// plain key action and a rotate to the plain knob action, so every profile
// works in gesture mode. Recognition delays depend on what is configured:
// a tap waits for the double-tap window only if the control has a doubletap
// action, and a hold waits for release if a longhold action exists.
//
// Recognizer is the pure state machine underneath (no goroutines, no I/O,
// time comes from the inputs). Call Feed for every input and Tick whenever
// the deadline from Next passes.
package gesture

import "time"

// Default thresholds (see docs/gestures.md).
const (
	DefaultHold            = 450 * time.Millisecond
	DefaultLongHold        = 1500 * time.Millisecond
	DefaultDoubleTap       = 300 * time.Millisecond
	DefaultChordWindow     = 80 * time.Millisecond
	DefaultCoincidence     = 2 * time.Millisecond
	DefaultPressRotateIdle = 500 * time.Millisecond
)

// Config holds recogniser thresholds. Zero durations mean the default.
type Config struct {
	Hold            time.Duration // press at least this long is a hold
	LongHold        time.Duration // press at least this long is a long hold
	DoubleTap       time.Duration // max gap from first release to second press
	ChordWindow     time.Duration // max gap between the two key presses of a chord
	Coincidence     time.Duration // knob-press release vs detent: press+rotate if |Δt| <= this
	PressRotateIdle time.Duration // press+rotate ends after this long without a detent

	// KnobEagerHold makes the knob press emit hold at the threshold like the
	// keys. Off by default: the knob hold is decided on release, so holding
	// the knob and then rotating emits only press+rotate (the firmware
	// reports a genuine release only when no rotation happened).
	KnobEagerHold bool

	// Has reports whether control has an action for kind. It decides which
	// gestures are worth waiting for (double-tap delays the tap, long hold
	// delays the hold) and whether chords are recognised. Nil means
	// everything is configured, which is what a pure monitor wants.
	Has func(control Control, kind Kind) bool
}

// DefaultConfig returns the default thresholds with Has = nil.
func DefaultConfig() Config {
	return Config{
		Hold:            DefaultHold,
		LongHold:        DefaultLongHold,
		DoubleTap:       DefaultDoubleTap,
		ChordWindow:     DefaultChordWindow,
		Coincidence:     DefaultCoincidence,
		PressRotateIdle: DefaultPressRotateIdle,
	}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	for _, f := range []struct{ v, def *time.Duration }{
		{&c.Hold, &d.Hold}, {&c.LongHold, &d.LongHold}, {&c.DoubleTap, &d.DoubleTap},
		{&c.ChordWindow, &d.ChordWindow}, {&c.Coincidence, &d.Coincidence},
		{&c.PressRotateIdle, &d.PressRotateIdle},
	} {
		if *f.v <= 0 {
			*f.v = *f.def
		}
	}
	return c
}

// press is the state of one pressable control (keys, knob press).
type press struct {
	down     bool
	downAt   time.Time
	holdDone bool // hold threshold processed for this press
	longDone bool // long-hold threshold processed
	emitted  bool // hold/longhold already emitted for this press
	consumed bool // part of a chord or press+rotate: no own gesture

	relPending bool // knob only: release waiting for the coincidence window
	relAt      time.Time
	relDur     time.Duration

	tapPending bool // first tap waiting for a possible second
	tapAt      time.Time
	second     bool // current press is a possible second tap
}

var pressOrder = []Control{Key1, Key2, Key3, KnobPress}

// Recognizer is the gesture state machine. Not safe for concurrent use.
type Recognizer struct {
	cfg      Config
	st       map[Control]*press
	prActive bool
	prLast   time.Time
	out      []Gesture
}

// NewRecognizer returns a recogniser; zero durations in cfg use the defaults.
func NewRecognizer(cfg Config) *Recognizer {
	r := &Recognizer{cfg: cfg.withDefaults(), st: map[Control]*press{}}
	for _, c := range pressOrder {
		r.st[c] = &press{}
	}
	return r
}

// SetHas replaces the configured-gesture predicate (e.g. on profile change).
func (r *Recognizer) SetHas(has func(Control, Kind) bool) { r.cfg.Has = has }

// Config returns the effective configuration.
func (r *Recognizer) Config() Config { return r.cfg }

func (r *Recognizer) has(c Control, k Kind) bool { return r.cfg.Has == nil || r.cfg.Has(c, k) }

func (r *Recognizer) eager(c Control) bool { return c != KnobPress || r.cfg.KnobEagerHold }

func (r *Recognizer) emit(c Control, k Kind, at time.Time) {
	r.out = append(r.out, Gesture{Control: c, Kind: k, Time: at})
}

func (r *Recognizer) take() []Gesture {
	out := r.out
	r.out = nil
	return out
}

type deadline struct {
	at  time.Time
	run func(at time.Time)
}

// next finds the earliest pending deadline.
func (r *Recognizer) next() (deadline, bool) {
	var best deadline
	found := false
	consider := func(at time.Time, run func(time.Time)) {
		if !found || at.Before(best.at) {
			best, found = deadline{at, run}, true
		}
	}
	for _, c := range pressOrder {
		s := r.st[c]
		if s.down && !s.consumed {
			if !s.holdDone {
				consider(s.downAt.Add(r.cfg.Hold), func(at time.Time) { r.onHold(c, at) })
			} else if !s.longDone {
				consider(s.downAt.Add(r.cfg.LongHold), func(at time.Time) { r.onLong(c, at) })
			}
		}
		if s.relPending {
			// Strictly after the window: a detent exactly at +Coincidence still counts.
			consider(s.relAt.Add(r.cfg.Coincidence+time.Nanosecond), func(time.Time) { r.resolveRelease(c) })
		}
		if s.tapPending && !s.down {
			consider(s.tapAt.Add(r.cfg.DoubleTap), func(at time.Time) {
				s.tapPending = false
				r.emit(c, Tap, at)
			})
		}
	}
	if r.prActive {
		consider(r.prLast.Add(r.cfg.PressRotateIdle), func(time.Time) { r.prActive = false })
	}
	return best, found
}

// Next returns the next deadline at which Tick must be called, if any.
func (r *Recognizer) Next() (time.Time, bool) {
	d, ok := r.next()
	return d.at, ok
}

// Tick processes every deadline up to and including now.
func (r *Recognizer) Tick(now time.Time) []Gesture {
	r.tick(now)
	return r.take()
}

func (r *Recognizer) tick(now time.Time) {
	for {
		d, ok := r.next()
		if !ok || d.at.After(now) {
			return
		}
		d.run(d.at)
	}
}

// PressRotating reports whether the knob is in press+rotate mode.
func (r *Recognizer) PressRotating() bool { return r.prActive }

// Feed processes one input (after any deadlines before its time) and returns
// the gestures decided so far. Unknown controls and key repeats are ignored.
func (r *Recognizer) Feed(in Input) []Gesture {
	r.tick(in.Time)
	switch {
	case in.Control.isDetent():
		if in.Down {
			r.detent(in.Control, in.Time)
		}
	case in.Control.isPress():
		if in.Down {
			r.down(in.Control, in.Time)
		} else {
			r.up(in.Control, in.Time)
		}
	}
	return r.take()
}

func (r *Recognizer) flushTap(c Control, at time.Time) {
	s := r.st[c]
	if s.tapPending {
		s.tapPending = false
		r.emit(c, Tap, at)
	}
	s.second = false
}

func (r *Recognizer) onHold(c Control, at time.Time) {
	s := r.st[c]
	s.holdDone = true
	if !r.has(c, Hold) {
		return
	}
	if s.second {
		r.flushTap(c, at)
	}
	if r.eager(c) && !r.has(c, LongHold) {
		s.emitted = true
		r.emit(c, Hold, at)
	}
}

func (r *Recognizer) onLong(c Control, at time.Time) {
	s := r.st[c]
	s.longDone = true
	if !r.has(c, LongHold) {
		return
	}
	if s.second {
		r.flushTap(c, at)
	}
	if r.eager(c) && !s.emitted {
		s.emitted = true
		r.emit(c, LongHold, at)
	}
}

func (r *Recognizer) down(c Control, at time.Time) {
	s := r.st[c]
	if s.down {
		return
	}
	if c == KnobPress {
		r.prActive = false
		if s.relPending {
			r.resolveRelease(c)
		}
	}
	*s = press{down: true, downAt: at, tapPending: s.tapPending, tapAt: s.tapAt}
	if c.isKey() {
		for _, o := range pressOrder[:3] {
			other := r.st[o]
			chord := ChordOf(o, c)
			if o == c || !other.down || other.consumed || other.emitted || other.holdDone ||
				at.Sub(other.downAt) > r.cfg.ChordWindow || !r.has(chord, Chord) {
				continue
			}
			r.flushTap(o, at)
			r.flushTap(c, at)
			other.consumed, s.consumed = true, true
			r.emit(chord, Chord, at)
			return
		}
	}
	if s.tapPending {
		s.second = true
	}
}

func (r *Recognizer) up(c Control, at time.Time) {
	s := r.st[c]
	if !s.down {
		return
	}
	s.down = false
	if s.consumed {
		return
	}
	d := at.Sub(s.downAt)
	if c == KnobPress {
		// Might be the firmware's forced release at the first detent, so it
		// is only decided after the coincidence window.
		s.relPending, s.relAt, s.relDur = true, at, d
		return
	}
	if !s.emitted {
		r.resolve(c, at, d)
	}
}

func (r *Recognizer) resolveRelease(c Control) {
	s := r.st[c]
	s.relPending = false
	if !s.emitted {
		r.resolve(c, s.relAt, s.relDur)
	}
}

// resolve decides the gesture of a finished press of duration d.
func (r *Recognizer) resolve(c Control, at time.Time, d time.Duration) {
	s := r.st[c]
	kind := Tap
	switch {
	case d >= r.cfg.LongHold && r.has(c, LongHold):
		kind = LongHold
	case d >= r.cfg.Hold && r.has(c, Hold):
		kind = Hold
	}
	if kind != Tap {
		r.flushTap(c, at)
		r.emit(c, kind, at)
		return
	}
	switch {
	case !r.has(c, DoubleTap):
		r.emit(c, Tap, at)
	case s.second:
		s.tapPending, s.second = false, false
		r.emit(c, DoubleTap, at)
	default:
		s.tapPending, s.tapAt = true, at
	}
}

func (r *Recognizer) detent(c Control, at time.Time) {
	kp := r.st[KnobPress]
	switch {
	case kp.relPending && at.Sub(kp.relAt) <= r.cfg.Coincidence:
		kp.relPending = false
		r.flushTap(KnobPress, at)
		r.prActive = true
	case kp.down:
		// Detent seen before the forced release: same meaning.
		kp.consumed = true
		r.flushTap(KnobPress, at)
		r.prActive = true
	}
	if r.prActive {
		r.prLast = at
		r.emit(c, PressRotate, at)
		return
	}
	r.emit(c, Rotate, at)
}

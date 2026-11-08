package ui

import (
	"sync"
	"time"
)

// PadInputEvent is one physical input from the pad, as read from the OS
// (evdev). Control is key1, key2, key3, knob-ccw, knob-press or knob-cw
// (the names used by internal/gesture).
type PadInputEvent struct {
	Control string
	Down    bool
	Time    time.Time
}

// PadInput delivers live pad input for on-screen feedback. Events is closed
// when the source stops.
type PadInput interface {
	Events() <-chan PadInputEvent
}

// Input control names.
const (
	InputKey1      = "key1"
	InputKey2      = "key2"
	InputKey3      = "key3"
	InputKnobCCW   = "knob-ccw"
	InputKnobPress = "knob-press"
	InputKnobCW    = "knob-cw"
)

// inputControl maps an input control name to the drawn element.
func inputControl(name string) (controlID, bool) {
	switch name {
	case InputKey1:
		return ctlKey1, true
	case InputKey2:
		return ctlKey2, true
	case InputKey3:
		return ctlKey3, true
	case InputKnobCCW:
		return ctlKnobCCW, true
	case InputKnobPress:
		return ctlKnobPress, true
	case InputKnobCW:
		return ctlKnobCW, true
	}
	return 0, false
}

// detentDegrees is how far the knob indicator ticks per detent.
const detentDegrees = 30

// detentAngle returns the indicator tick for a knob turn in degrees
// (negative = counter-clockwise), or 0 for anything that is not a turn.
func detentAngle(c controlID) float64 {
	switch c {
	case ctlKnobCCW:
		return -detentDegrees
	case ctlKnobCW:
		return detentDegrees
	}
	return 0
}

// tickAngle is the eased indicator angle at progress p (0..1) of a detent
// animation: out to the full tick in the first 40%, then eased back to rest.
func tickAngle(full float64, p float32) float64 {
	switch {
	case p <= 0:
		return 0
	case p < 0.4:
		x := float64(p / 0.4)
		return full * (1 - (1-x)*(1-x))
	case p < 1:
		x := float64((p - 0.4) / 0.6)
		return full * (1 - x*x)
	}
	return 0
}

// inputCaption is the "last input" text, e.g. "key 1 · tap" or "knob ↻".
func inputCaption(c controlID) string {
	switch c {
	case ctlKey1, ctlKey2, ctlKey3:
		return "key " + string(rune('1'+int(c-ctlKey1))) + " · tap"
	case ctlKnobCCW:
		return "knob ↺"
	case ctlKnobCW:
		return "knob ↻"
	case ctlKnobPress:
		return "knob · press"
	}
	return ""
}

// fakeInput is a dev-only PadInput (AGENTPAD_FAKE_INPUT=1) that replays a
// demo sequence so the live feedback can be seen without a real reader.
type fakeInput struct {
	ch   chan PadInputEvent
	stop chan struct{}
	once sync.Once
}

// NewFakeInput starts a demo input source cycling through every control.
func NewFakeInput(step time.Duration) PadInput {
	f := &fakeInput{ch: make(chan PadInputEvent), stop: make(chan struct{})}
	go f.run(step)
	return f
}

func (f *fakeInput) Events() <-chan PadInputEvent { return f.ch }

// Close stops the demo.
func (f *fakeInput) Close() { f.once.Do(func() { close(f.stop) }) }

func (f *fakeInput) run(step time.Duration) {
	defer close(f.ch)
	seq := []string{InputKey1, InputKey2, InputKey3, InputKnobCW, InputKnobCW, InputKnobCCW, InputKnobPress}
	for i := 0; ; i++ {
		name := seq[i%len(seq)]
		for _, down := range []bool{true, false} {
			select {
			case <-f.stop:
				return
			case f.ch <- PadInputEvent{Control: name, Down: down, Time: time.Now()}:
			}
			select {
			case <-f.stop:
				return
			case <-time.After(step / 3):
			}
		}
		select {
		case <-f.stop:
			return
		case <-time.After(step):
		}
	}
}

package gesture

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func at(ms float64) time.Time { return t0.Add(time.Duration(ms * float64(time.Millisecond))) }

func dn(ms float64, c Control) Input { return Input{Control: c, Down: true, Time: at(ms)} }
func up(ms float64, c Control) Input { return Input{Control: c, Down: false, Time: at(ms)} }

// tap returns a press+release pair.
func tap(ms, dur float64, c Control) []Input { return []Input{dn(ms, c), up(ms+dur, c)} }

func seq(parts ...any) []Input {
	var out []Input
	for _, p := range parts {
		switch v := p.(type) {
		case Input:
			out = append(out, v)
		case []Input:
			out = append(out, v...)
		}
	}
	return out
}

func format(gs []Gesture) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		ms := float64(g.Time.Sub(t0)) / float64(time.Millisecond)
		out = append(out, strconv.FormatFloat(ms, 'f', -1, 64)+" "+g.String())
	}
	return out
}

// runLazy feeds all inputs and flushes at the end (deadlines are processed
// lazily by Feed).
func runLazy(r *Recognizer, in []Input) []Gesture {
	var out []Gesture
	for _, i := range in {
		out = append(out, r.Feed(i)...)
	}
	return append(out, r.Tick(at(1e9))...)
}

// runTimed behaves like the engine: before each input, Tick at every
// deadline reported by Next that has passed.
func runTimed(r *Recognizer, in []Input) []Gesture {
	var out []Gesture
	drain := func(until time.Time) {
		for {
			d, ok := r.Next()
			if !ok || d.After(until) {
				return
			}
			out = append(out, r.Tick(d)...)
		}
	}
	for _, i := range in {
		drain(i.Time.Add(-time.Nanosecond))
		out = append(out, r.Feed(i)...)
	}
	drain(at(1e9))
	if _, ok := r.Next(); ok {
		panic("deadline left after drain")
	}
	return out
}

// only builds a Has predicate from "control kind" pairs; taps and rotates
// are always configured (profile fallback).
func only(pairs ...string) func(Control, Kind) bool {
	return func(c Control, k Kind) bool {
		if k == Tap || k == Rotate {
			return true
		}
		return slices.Contains(pairs, string(c)+" "+string(k))
	}
}

func TestRecognizer(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		in   []Input
		want []string
	}{
		{"tap", Config{Has: only()}, tap(0, 80, Key1), []string{"80 key1 tap"}},
		{"long press without hold action is a tap", Config{Has: only()}, tap(0, 2000, Key1), []string{"2000 key1 tap"}},
		{"tap just below hold", Config{Has: only("key1 hold")}, tap(0, 449, Key1), []string{"449 key1 tap"}},
		{"hold at threshold", Config{Has: only("key1 hold")}, tap(0, 450, Key1), []string{"450 key1 hold"}},
		{"hold fires while pressed, once", Config{Has: only("key1 hold")}, tap(0, 3000, Key1), []string{"450 key1 hold"}},
		{"hold waits for release when long hold is set", Config{Has: only("key1 hold", "key1 longhold")},
			tap(0, 900, Key1), []string{"900 key1 hold"}},
		{"long hold fires at threshold", Config{Has: only("key1 hold", "key1 longhold")},
			tap(0, 5000, Key1), []string{"1500 key1 longhold"}},
		{"long hold only: short-ish press is a tap", Config{Has: only("key1 longhold")},
			tap(0, 900, Key1), []string{"900 key1 tap"}},
		{"long hold only: long press", Config{Has: only("key1 longhold")},
			tap(0, 1600, Key1), []string{"1500 key1 longhold"}},
		{"double tap without action: two taps immediately", Config{Has: only()},
			seq(tap(0, 50, Key3), tap(150, 50, Key3)), []string{"50 key3 tap", "200 key3 tap"}},
		{"double tap", Config{Has: only("key3 doubletap")},
			seq(tap(0, 50, Key3), tap(150, 50, Key3)), []string{"200 key3 doubletap"}},
		{"single tap delayed by window", Config{Has: only("key3 doubletap")},
			tap(0, 50, Key3), []string{"350 key3 tap"}},
		{"second tap after window", Config{Has: only("key3 doubletap")},
			seq(tap(0, 50, Key3), tap(351, 50, Key3)), []string{"350 key3 tap", "701 key3 tap"}},
		{"second press becomes hold", Config{Has: only("key3 doubletap", "key3 hold")},
			seq(tap(0, 50, Key3), tap(150, 1000, Key3)), []string{"600 key3 tap", "600 key3 hold"}},
		{"triple tap", Config{Has: only("key3 doubletap")},
			seq(tap(0, 50, Key3), tap(150, 50, Key3), tap(300, 50, Key3)),
			[]string{"200 key3 doubletap", "650 key3 tap"}},
		{"double tap on other key does not mix", Config{Has: only("key3 doubletap")},
			seq(tap(0, 50, Key3), tap(150, 50, Key1)), []string{"200 key1 tap", "350 key3 tap"}},
		{"chord 1+3", Config{}, seq(dn(0, Key1), dn(40, Key3), up(200, Key1), up(210, Key3)),
			[]string{"40 key1+key3 chord"}},
		{"chord reverse order is canonical", Config{}, seq(dn(0, Key3), dn(80, Key1), up(200, Key3), up(210, Key1)),
			[]string{"80 key1+key3 chord"}},
		{"chord held long: no hold", Config{}, seq(dn(0, Key2), dn(10, Key1), up(3000, Key2), up(3000, Key1)),
			[]string{"10 key1+key2 chord"}},
		{"chord window missed", Config{Has: only("key1+key3 chord")},
			seq(dn(0, Key1), dn(81, Key3), up(200, Key1), up(210, Key3)),
			[]string{"200 key1 tap", "210 key3 tap"}},
		{"chord not configured", Config{Has: only()},
			seq(dn(0, Key1), dn(20, Key3), up(100, Key1), up(110, Key3)),
			[]string{"100 key1 tap", "110 key3 tap"}},
		{"chord only between keys", Config{}, seq(dn(0, KnobPress), dn(10, Key1), up(100, Key1), up(120, KnobPress)),
			[]string{"400 key1 tap", "420 knob-press tap"}},
		{"rotation without press", Config{}, seq(tap(0, 8, KnobCW), tap(100, 8, KnobCW), tap(300, 8, KnobCCW)),
			[]string{"0 knob-cw rotate", "100 knob-cw rotate", "300 knob-ccw rotate"}},
		{"knob tap waits for coincidence window only", Config{Has: only()}, tap(0, 100, KnobPress),
			[]string{"100 knob-press tap"}},
		{"knob press then detent 3 ms later is not press-rotate", Config{Has: only()},
			seq(tap(0, 100, KnobPress), tap(103, 8, KnobCW)),
			[]string{"100 knob-press tap", "103 knob-cw rotate"}},
		{"press-rotate, coincident release", Config{},
			seq(dn(0, KnobPress), up(200, KnobPress), tap(200, 8, KnobCW), tap(400, 8, KnobCW)),
			[]string{"200 knob-cw pressrotate", "400 knob-cw pressrotate"}},
		{"press-rotate, release 2 ms before detent", Config{},
			seq(dn(0, KnobPress), up(198, KnobPress), tap(200, 8, KnobCCW)),
			[]string{"200 knob-ccw pressrotate"}},
		{"press-rotate, detent before release", Config{},
			seq(dn(0, KnobPress), dn(200, KnobCW), up(200, KnobPress), up(208, KnobCW)),
			[]string{"200 knob-cw pressrotate"}},
		{"press-rotate ends by idle", Config{},
			seq(dn(0, KnobPress), up(200, KnobPress), tap(200, 8, KnobCW), tap(699, 8, KnobCW), tap(1200, 8, KnobCW)),
			[]string{"200 knob-cw pressrotate", "699 knob-cw pressrotate", "1200 knob-cw rotate"}},
		{"press-rotate ends by new knob press", Config{Has: only()},
			seq(dn(0, KnobPress), up(200, KnobPress), tap(200, 8, KnobCW), tap(300, 50, KnobPress), tap(400, 8, KnobCW)),
			[]string{"200 knob-cw pressrotate", "350 knob-press tap", "400 knob-cw rotate"}},
		{"knob hold decided on release", Config{Has: only("knob-press hold")}, tap(0, 800, KnobPress),
			[]string{"800 knob-press hold"}},
		{"knob long press then rotate: no hold", Config{Has: only("knob-press hold")},
			seq(dn(0, KnobPress), up(900, KnobPress), tap(900, 8, KnobCW)),
			[]string{"900 knob-cw pressrotate"}},
		{"knob eager hold then rotate", Config{Has: only("knob-press hold"), KnobEagerHold: true},
			seq(dn(0, KnobPress), up(900, KnobPress), tap(900, 8, KnobCW)),
			[]string{"450 knob-press hold", "900 knob-cw pressrotate"}},
		{"knob double tap", Config{Has: only("knob-press doubletap")},
			seq(tap(0, 60, KnobPress), tap(200, 60, KnobPress)), []string{"260 knob-press doubletap"}},
		{"interleaved keys", Config{Has: only("key1 hold")},
			seq(dn(0, Key1), dn(100, Key2), up(150, Key2), tap(300, 8, KnobCW), up(600, Key1), tap(700, 50, Key3)),
			[]string{"150 key2 tap", "300 knob-cw rotate", "450 key1 hold", "750 key3 tap"}},
		{"rotate while key held", Config{Has: only("key2 hold")},
			seq(dn(0, Key2), tap(100, 8, KnobCCW), up(200, Key2)),
			[]string{"100 knob-ccw rotate", "200 key2 tap"}},
		{"stray release ignored", Config{}, seq(up(10, Key1), up(20, KnobPress), up(30, KnobCW)), nil},
		{"repeated down ignored", Config{Has: only()}, seq(dn(0, Key1), dn(30, Key1), up(60, Key1)),
			[]string{"60 key1 tap"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lazy := format(runLazy(NewRecognizer(tt.cfg), tt.in))
			timed := format(runTimed(NewRecognizer(tt.cfg), tt.in))
			if !reflect.DeepEqual(lazy, tt.want) && (len(lazy) != 0 || len(tt.want) != 0) {
				t.Errorf("lazy:\n got %q\nwant %q", lazy, tt.want)
			}
			if !reflect.DeepEqual(timed, lazy) {
				t.Errorf("timed differs from lazy:\ntimed %q\n lazy %q", timed, lazy)
			}
		})
	}
}

func loadTrace(t *testing.T, path string, keys map[string]Control) []Input {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []Input
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var ts float64
		var key, state string
		if _, err := fmt.Sscanf(line, "+%fs %s %s", &ts, &key, &state); err != nil {
			t.Fatalf("bad line %q: %v", line, err)
		}
		c, ok := keys[key]
		if !ok {
			t.Fatalf("unknown key %s", key)
		}
		// Round to whole microseconds like the kernel timestamps.
		out = append(out, Input{Control: c, Down: state == "pressed", Time: t0.Add(time.Duration(ts*1e6) * time.Microsecond)})
	}
	return out
}

func TestTrace8890(t *testing.T) {
	in := loadTrace(t, "testdata/trace-8890.txt", map[string]Control{
		"KEY_ENTER": KnobPress, "KEY_DOWN": KnobCW, "KEY_UP": KnobCCW,
	})
	if len(in) != 14 {
		t.Fatalf("parsed %d events", len(in))
	}
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			// Everything configured (monitor view). The last detent comes
			// 792 ms after the previous one, past the 500 ms idle limit, so
			// it is a plain rotate.
			"all gestures", DefaultConfig(),
			[]string{
				"3056 knob-press longhold",
				"5896 knob-cw pressrotate", "6104 knob-cw pressrotate", "6600 knob-ccw pressrotate",
				"7392 knob-cw rotate",
				"13872 knob-press hold",
			},
		},
		{
			"plain profile (taps and rotates only)", Config{Has: only()},
			[]string{
				"3056 knob-press tap",
				"5896 knob-cw pressrotate", "6104 knob-cw pressrotate", "6600 knob-ccw pressrotate",
				"7392 knob-cw rotate",
				"13872 knob-press tap",
			},
		},
		{
			"eager knob hold", Config{KnobEagerHold: true, Has: only("knob-press hold")},
			[]string{
				"450 knob-press hold",
				"5314 knob-press hold",
				"5896 knob-cw pressrotate", "6104 knob-cw pressrotate", "6600 knob-ccw pressrotate",
				"7392 knob-cw rotate",
				"13546 knob-press hold",
			},
		},
		{
			"longer idle keeps the whole rotation", Config{PressRotateIdle: time.Second},
			[]string{
				"3056 knob-press longhold",
				"5896 knob-cw pressrotate", "6104 knob-cw pressrotate", "6600 knob-ccw pressrotate",
				"7392 knob-cw pressrotate",
				"13872 knob-press hold",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lazy := format(runLazy(NewRecognizer(tt.cfg), in))
			if !reflect.DeepEqual(lazy, tt.want) {
				t.Errorf("got  %q\nwant %q", lazy, tt.want)
			}
			if timed := format(runTimed(NewRecognizer(tt.cfg), in)); !reflect.DeepEqual(timed, lazy) {
				t.Errorf("timed %q", timed)
			}
		})
	}
}

func TestDefaultsAndNext(t *testing.T) {
	r := NewRecognizer(Config{Hold: 100 * time.Millisecond, Has: only("key1 hold")})
	c := r.Config()
	if c.Hold != 100*time.Millisecond || c.LongHold != DefaultLongHold || c.DoubleTap != DefaultDoubleTap ||
		c.ChordWindow != DefaultChordWindow || c.Coincidence != DefaultCoincidence || c.PressRotateIdle != DefaultPressRotateIdle {
		t.Fatalf("defaults: %+v", c)
	}
	if _, ok := r.Next(); ok {
		t.Fatal("idle recogniser has a deadline")
	}
	r.Feed(dn(0, Key1))
	if d, ok := r.Next(); !ok || !d.Equal(at(100)) {
		t.Fatalf("Next = %v %v", d, ok)
	}
	if g := r.Tick(at(99)); len(g) != 0 {
		t.Fatalf("early tick: %v", g)
	}
	if g := format(r.Tick(at(100))); !reflect.DeepEqual(g, []string{"100 key1 hold"}) {
		t.Fatalf("tick: %v", g)
	}
	r.SetHas(only())
	r.Feed(up(120, Key1))
	r.Feed(dn(200, KnobPress))
	r.Feed(up(250, KnobPress))
	r.Feed(dn(250, KnobCW))
	if !r.PressRotating() {
		t.Fatal("expected press-rotate")
	}
}

func TestControlsMatchProfile(t *testing.T) {
	var got []string
	for code := KeyF13; code <= KeyF18; code++ {
		c, ok := ControlForKey(code)
		if !ok {
			t.Fatalf("code %d unmapped", code)
		}
		got = append(got, string(c))
	}
	if !reflect.DeepEqual(got, profile.GestureControls()) {
		t.Fatalf("controls %v", got)
	}
	if _, ok := ControlForKey(28); ok {
		t.Fatal("KEY_ENTER mapped")
	}
	// The keys programmed by GestureModeMapping are f13..f18 in control order.
	g := profile.GestureModeProfile()
	if g.Keys != [3]string{"f13", "f14", "f15"} || g.Knob != (profile.Knob{CCW: "f16", Press: "f17", CW: "f18"}) {
		t.Fatalf("gesture mode profile %+v", g)
	}
	if ChordOf(Key3, Key1) != "key1+key3" || ChordOf(Key1, Key1) != "" || ChordOf(Key1, KnobPress) != "" {
		t.Fatal("ChordOf")
	}
	if !ChordOf(Key1, Key2).IsChord() || Key1.IsChord() {
		t.Fatal("IsChord")
	}
	for _, k := range []Kind{Tap, Hold, LongHold, DoubleTap} {
		if !slices.Contains(profile.GestureKinds(string(Key1)), string(k)) {
			t.Errorf("kind %s", k)
		}
	}
}

func TestPlainKeyMapScriptKeys(t *testing.T) {
	p := profile.Profile{Name: "s", Keys: [3]string{"run:deploy", "a", "run:mic"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"}}
	km, rep, err := PlainKeyMap(p, profile.OrientNormal)
	if err != nil {
		t.Fatal(err)
	}
	// KEY_F19 = 189, KEY_F20 = 190
	if km[189] != Key1 || km[190] != Key3 || len(rep.Unmapped) != 0 {
		t.Fatalf("km = %v, unmapped %v", km, rep.Unmapped)
	}
}

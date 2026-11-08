package settings

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
)

type change struct {
	s   Settings
	err error
}

// watchT starts a watcher; poll forces the polling fallback.
func watchT(t *testing.T, s *Store, poll bool) <-chan change {
	t.Helper()
	s.forcePoll = poll
	if poll {
		s.pollInterval = 100 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan change, 16)
	s.Watch(ctx, func(st Settings, err error) { ch <- change{st, err} })
	return ch
}

func next(t *testing.T, ch <-chan change) change {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no change reported within 5s")
		return change{}
	}
}

// quiet asserts nothing is reported for d.
func quiet(t *testing.T, ch <-chan change, d time.Duration) {
	t.Helper()
	select {
	case c := <-ch:
		t.Fatalf("unexpected change: %+v", c)
	case <-time.After(d):
	}
}

// watchModes runs a test with inotify and with the polling fallback.
var watchModes = []bool{false, true}

func modeName(poll bool) string {
	if poll {
		return "poll"
	}
	return "inotify"
}

func TestWatchExternalEdit(t *testing.T) {
	for _, poll := range watchModes {
		t.Run(modeName(poll), func(t *testing.T) {
			s := openT(t, t.TempDir())
			ch := watchT(t, s, poll)
			writeT(t, s.Path(), "schema: 1\npad:\n  orientation: clockwise\n")
			c := next(t, ch)
			if c.err != nil || c.s.Pad.Orientation != profile.OrientClockwise {
				t.Fatalf("change = %+v", c)
			}
			if s.Get().Pad.Orientation != profile.OrientClockwise {
				t.Fatal("store not updated")
			}
		})
	}
}

func TestWatchInvalidEditKeepsLastGood(t *testing.T) {
	for _, poll := range watchModes {
		t.Run(modeName(poll), func(t *testing.T) {
			s := openT(t, t.TempDir())
			ch := watchT(t, s, poll)
			writeT(t, s.Path(), "schema: 1\npad:\n  orientation: sideways\n")
			c := next(t, ch)
			var ve *ValidationError
			if !errors.As(c.err, &ve) || ve.Errors[0].Path != "pad.orientation" || ve.Errors[0].Line != 3 {
				t.Fatalf("err = %v", c.err)
			}
			if c.s.Pad.Orientation != profile.OrientNormal || s.Get().Pad.Orientation != profile.OrientNormal {
				t.Fatal("invalid orientation adopted")
			}
			// fixing the file recovers
			writeT(t, s.Path(), "schema: 1\npad:\n  orientation: upsidedown\n")
			if c := next(t, ch); c.err != nil || c.s.Pad.Orientation != profile.OrientUpsideDown {
				t.Fatalf("after fix = %+v", c)
			}
		})
	}
}

func TestWatchIgnoresOwnWrites(t *testing.T) {
	for _, poll := range watchModes {
		t.Run(modeName(poll), func(t *testing.T) {
			s := openT(t, t.TempDir())
			ch := watchT(t, s, poll)
			for _, th := range []string{ThemeDark, ThemeLight} {
				if err := s.Update(func(x *Settings) { x.UI.Theme = th }); err != nil {
					t.Fatal(err)
				}
			}
			quiet(t, ch, 1500*time.Millisecond)
		})
	}
}

func TestWatchDebouncesBursts(t *testing.T) {
	s := openT(t, t.TempDir())
	ch := watchT(t, s, false)
	for i := 1; i <= 5; i++ {
		writeT(t, s.Path(), fmt.Sprintf("schema: 1\ngestures:\n  hold_ms: %d\n", 400+i))
	}
	c := next(t, ch)
	if c.err != nil || c.s.Gestures.HoldMs != 405 {
		t.Fatalf("change = %+v", c)
	}
	quiet(t, ch, 1500*time.Millisecond)
}

func TestWatchStopsOnCancel(t *testing.T) {
	s := openT(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan change, 4)
	s.Watch(ctx, func(st Settings, err error) { ch <- change{st, err} })
	cancel()
	time.Sleep(300 * time.Millisecond)
	writeT(t, s.Path(), "schema: 1\nui: {theme: dark}\n")
	quiet(t, ch, 1500*time.Millisecond)
}

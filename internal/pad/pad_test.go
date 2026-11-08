package pad

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeDev(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for f, v := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectIn(t *testing.T) {
	root := t.TempDir()
	writeDev(t, root, "usb1", map[string]string{"idVendor": "1d6b\n", "idProduct": "0002\n"})
	writeDev(t, root, "1-1:1.0", map[string]string{"bInterfaceClass": "03\n"})
	writeDev(t, root, "1-2", map[string]string{"idVendor": "1189\n", "idProduct": "1234\n"})
	writeDev(t, root, "1-4", map[string]string{"idVendor": "1189\n", "idProduct": "8890\n", "product": "wch.cn CH57x\n"})
	writeDev(t, root, "3-1", map[string]string{"idVendor": "1189\n", "idProduct": "8840\n"})

	got, err := DetectIn(root)
	if err != nil {
		t.Fatal(err)
	}
	want := Device{VendorID: 0x1189, ProductID: 0x8890, BusPath: "1-4", Product: "wch.cn CH57x"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	empty := t.TempDir()
	writeDev(t, empty, "1-2", map[string]string{"idVendor": "1189\n", "idProduct": "1234\n"})
	writeDev(t, empty, "1-3", map[string]string{"idVendor": "zz\n", "idProduct": "8890\n"})
	if _, err := DetectIn(empty); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no pad: got %v", err)
	}
	if _, err := DetectIn(filepath.Join(empty, "missing")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("missing root: got %v", err)
	}

	alt := t.TempDir()
	writeDev(t, alt, "2-1", map[string]string{"idVendor": "1189", "idProduct": "8842"})
	if d, err := DetectIn(alt); err != nil || d.ProductID != 0x8842 || d.Product != "" {
		t.Fatalf("8842: %+v %v", d, err)
	}
}

type call struct {
	args  []string
	stdin string
}

func fakeWriter(stderr string, runErr error, calls *[]call) *ToolWriter {
	return &ToolWriter{
		lookPath: func(string) (string, error) { return "/fake/ch57x-keyboard-tool", nil },
		run: func(_ context.Context, path string, args []string, stdin []byte) (string, error) {
			*calls = append(*calls, call{args, string(stdin)})
			return stderr, runErr
		},
	}
}

func TestToolWriterArgs(t *testing.T) {
	var calls []call
	w := fakeWriter("", nil, &calls)
	if err := w.Upload([]byte("doc")); err != nil {
		t.Fatal(err)
	}
	if err := w.SetLED(m(2)); err != nil {
		t.Fatal(err)
	}
	w.ProductID = 0x8890
	_ = w.SetLED(m(0))
	w.ProductID = 0x8840
	_ = w.SetLED(m(1))
	want := []call{
		{[]string{"upload"}, "doc"},
		{[]string{"led", "2"}, ""},
		{[]string{"led", "0"}, ""},
		{[]string{"--product-id", "34880", "led", "1"}, ""},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestToolWriterErrors(t *testing.T) {
	noPad := "Error: find USB device\n\nCaused by:\n    CH57x keyboard device not found. Use --vendor-id and --product-id to override settings.\n"
	exitErr := errors.New("exit status 1")
	tests := []struct {
		name    string
		stderr  string
		want    error
		contain string
	}{
		{"no pad", noPad, ErrNotConnected, "device not found"},
		{"no such device", "Error: No such device (it may have been disconnected)", ErrNotConnected, ""},
		{"access denied", "Error: open USB device\n\nCaused by:\n    Access denied (insufficient permissions)\n", ErrPermission, "Access denied"},
		{"other", "  Error: something odd  \n", nil, "something odd"},
		{"empty stderr", "", nil, "exit status 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			err := fakeWriter(tt.stderr, exitErr, &calls).Upload([]byte("x"))
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
			if tt.want == nil && (errors.Is(err, ErrNotConnected) || errors.Is(err, ErrPermission)) {
				t.Fatalf("misclassified: %v", err)
			}
			if !strings.Contains(err.Error(), tt.contain) {
				t.Fatalf("%q does not contain %q", err, tt.contain)
			}
		})
	}
}

func TestToolWriterMissing(t *testing.T) {
	w := &ToolWriter{lookPath: func(string) (string, error) { return "", exec.ErrNotFound }}
	for _, err := range []error{w.Available(), w.Upload(nil), w.SetLED(m(1))} {
		if !errors.Is(err, ErrToolMissing) {
			t.Fatalf("got %v", err)
		}
	}
	w = &ToolWriter{Path: filepath.Join(t.TempDir(), "nope")}
	if err := w.Available(); !errors.Is(err, ErrToolMissing) {
		t.Fatalf("explicit path: got %v", err)
	}
}

// TestToolWriterRealNoPad runs the installed tool; no pad is expected to be attached.
func TestToolWriterRealNoPad(t *testing.T) {
	if _, err := exec.LookPath(toolName); err != nil {
		t.Skip("tool not installed")
	}
	if _, err := Detect(); err == nil {
		t.Skip("a pad is connected")
	}
	if err := NewToolWriter().SetLED(m(0)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("got %v", err)
	}
}

type fakeLED struct {
	mu   sync.Mutex
	got  []int
	fail bool
	done chan struct{}
}

func (f *fakeLED) Available() error      { return nil }
func (f *fakeLED) Upload(_ []byte) error { return nil }
func (f *fakeLED) SetLED(s LEDSetting) error {
	n := s.Mode
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, n)
	if f.done != nil {
		select {
		case f.done <- struct{}{}:
		default:
		}
	}
	if f.fail {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeLED) writes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.got...)
}

// fakeClock drives LEDController deterministically.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
	fn  func()
	at  time.Time
}

type fakeTimer struct{ c *fakeClock }

func (t fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	had := t.c.fn != nil
	t.c.fn = nil
	return had
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) stopper {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fn, c.at = f, c.now.Add(d)
	return fakeTimer{c}
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var fn func()
	if c.fn != nil && !c.now.Before(c.at) {
		fn, c.fn = c.fn, nil
	}
	c.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func newTestLED(w Writer) (*LEDController, *fakeClock) {
	clk := &fakeClock{now: time.Unix(1000, 0)}
	c := NewLEDController(w, 2*time.Second)
	c.now, c.afterF = clk.Now, clk.AfterFunc
	return c, clk
}

func TestLEDChangeOnly(t *testing.T) {
	f := &fakeLED{}
	c, clk := newTestLED(f)
	if curMode(c) != -1 {
		t.Fatal("initial Current should be -1")
	}
	_ = c.Set(m(1))
	clk.Advance(5 * time.Second)
	_ = c.Set(m(1))
	_ = c.Set(m(1))
	clk.Advance(5 * time.Second)
	_ = c.Set(m(0))
	if got := f.writes(); !reflect.DeepEqual(got, []int{1, 0}) {
		t.Fatalf("writes = %v", got)
	}
	if curMode(c) != 0 {
		t.Fatalf("Current = %d", curMode(c))
	}
}

func TestLEDDeferLatestWins(t *testing.T) {
	f := &fakeLED{}
	c, clk := newTestLED(f)
	_ = c.Set(m(1))
	clk.Advance(500 * time.Millisecond)
	_ = c.Set(m(2))
	_ = c.Set(m(3))
	_ = c.Set(m(4))
	if got := f.writes(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("before interval: %v", got)
	}
	clk.Advance(1 * time.Second) // 1.5s since write
	if len(f.writes()) != 1 {
		t.Fatal("fired too early")
	}
	clk.Advance(500 * time.Millisecond)
	if got := f.writes(); !reflect.DeepEqual(got, []int{1, 4}) {
		t.Fatalf("after interval: %v", got)
	}
	if curMode(c) != 4 {
		t.Fatalf("Current = %d", curMode(c))
	}

	// Deferred request that returns to the current mode writes nothing.
	clk.Advance(100 * time.Millisecond)
	_ = c.Set(m(5))
	_ = c.Set(m(4))
	clk.Advance(3 * time.Second)
	if got := f.writes(); !reflect.DeepEqual(got, []int{1, 4}) {
		t.Fatalf("revert: %v", got)
	}
}

func TestLEDErrorAndClose(t *testing.T) {
	f := &fakeLED{fail: true}
	c, clk := newTestLED(f)
	if err := c.Set(m(1)); err == nil {
		t.Fatal("expected immediate error")
	}
	if curMode(c) != -1 {
		t.Fatal("Current changed on failure")
	}
	_ = c.Set(m(2)) // within interval: deferred
	c.Close()
	clk.Advance(5 * time.Second)
	_ = c.Set(m(3))
	if got := f.writes(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("writes = %v", got)
	}
}

// TestLEDRealTimer exercises the real time.AfterFunc path under -race.
func TestLEDRealTimer(t *testing.T) {
	f := &fakeLED{done: make(chan struct{}, 4)}
	c := NewLEDController(f, 50*time.Millisecond)
	defer c.Close()
	_ = c.Set(m(1))
	<-f.done
	var wg sync.WaitGroup
	for i := 2; i <= 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = c.Set(m(i)) }()
	}
	wg.Wait()
	_ = c.Set(m(9))
	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("deferred write never happened")
	}
	if got := f.writes(); len(got) != 2 || got[1] != 9 || curMode(c) != 9 {
		t.Fatalf("writes = %v current = %d", got, curMode(c))
	}
}

func m(n int) LEDSetting { return LEDSetting{Mode: n} }

func curMode(c *LEDController) int {
	s, ok := c.Current()
	if !ok {
		return -1
	}
	return s.Mode
}

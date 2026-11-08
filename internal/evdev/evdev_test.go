package evdev

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

// raw encodes one input_event with a 64-bit timeval.
func raw(sec, usec int64, typ, code uint16, val int32) []byte {
	b := make([]byte, 24)
	binary.LittleEndian.PutUint64(b[0:], uint64(sec))
	binary.LittleEndian.PutUint64(b[8:], uint64(usec))
	binary.LittleEndian.PutUint16(b[16:], typ)
	binary.LittleEndian.PutUint16(b[18:], code)
	binary.LittleEndian.PutUint32(b[20:], uint32(val))
	return b
}

func raw32(sec, usec int32, typ, code uint16, val int32) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:], uint32(sec))
	binary.LittleEndian.PutUint32(b[4:], uint32(usec))
	binary.LittleEndian.PutUint16(b[8:], typ)
	binary.LittleEndian.PutUint16(b[10:], code)
	binary.LittleEndian.PutUint32(b[12:], uint32(val))
	return b
}

func TestEventSize(t *testing.T) {
	if EventSize != 24 && EventSize != 16 {
		t.Fatalf("EventSize = %d", EventSize)
	}
	if eviocgrab != 0x40044590 {
		t.Fatalf("EVIOCGRAB = %#x", eviocgrab)
	}
}

func TestParseEvents(t *testing.T) {
	// A real-looking burst for a knob detent: MSC_SCAN, KEY down, SYN, then up.
	var b []byte
	b = append(b, raw(1762600000, 896000, 0x04, 0x04, 0x7006b)...) // EV_MSC scan
	b = append(b, raw(1762600000, 896000, 0x01, 188, 1)...)        // KEY_F18 down
	b = append(b, raw(1762600000, 896000, 0x00, 0, 0)...)          // SYN_REPORT
	b = append(b, raw(1762600000, 904000, 0x01, 188, 2)...)        // repeat: dropped
	b = append(b, raw(1762600000, 904000, 0x01, 188, 0)...)        // up
	b = append(b, 1, 2, 3)                                         // partial trailing event
	got := ParseEvents(b, 16)
	want := []KeyEvent{
		{Code: 188, Down: true, Time: time.Unix(1762600000, 896000000)},
		{Code: 188, Down: false, Time: time.Unix(1762600000, 904000000)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	got32 := ParseEvents(append(raw32(5, 1, 0x01, 183, 1), raw32(5, 9, 0x01, 183, 0)...), 8)
	if len(got32) != 2 || got32[0].Code != 183 || !got32[0].Down || got32[1].Down || !got32[1].Time.Equal(time.Unix(5, 9000)) {
		t.Fatalf("32-bit: %+v", got32)
	}
	if ParseEvents(nil, 16) != nil {
		t.Fatal("nil input")
	}
}

type chunkReader struct {
	chunks [][]byte
	end    error
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, r.end
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

func TestReadLoop(t *testing.T) {
	r := &chunkReader{
		chunks: [][]byte{raw(1, 0, 1, 183, 1), append(raw(1, 5, 1, 183, 0), raw(1, 6, 0, 0, 0)...)},
		end:    syscall.ENODEV,
	}
	var got []KeyEvent
	err := readLoop(context.Background(), r, 16, func(e KeyEvent) { got = append(got, e) })
	if !errors.Is(err, ErrDisconnected) || len(got) != 2 {
		t.Fatalf("err=%v got=%v", err, got)
	}
	err = readLoop(context.Background(), &chunkReader{end: io.ErrUnexpectedEOF}, 16, func(KeyEvent) {})
	if err == nil || errors.Is(err, ErrDisconnected) {
		t.Fatalf("other error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := readLoop(ctx, &chunkReader{end: os.ErrDeadlineExceeded}, 16, func(KeyEvent) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

// fakeSys builds /sys/class/input/eventN/device/id/{vendor,product}.
func fakeSys(t *testing.T, nodes map[string][2]string) string {
	t.Helper()
	root := t.TempDir()
	for name, id := range nodes {
		dir := filepath.Join(root, name, "device", "id")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if id[0] != "" {
			_ = os.WriteFile(filepath.Join(dir, "vendor"), []byte(id[0]+"\n"), 0o644)
		}
		if id[1] != "" {
			_ = os.WriteFile(filepath.Join(dir, "product"), []byte(id[1]+"\n"), 0o644)
		}
	}
	return root
}

func TestDiscoverIn(t *testing.T) {
	root := fakeSys(t, map[string][2]string{
		"event10": {"1189", "8890"},
		"event9":  {"1189", "8890"},
		"event31": {"1189", "8890"},
		"event2":  {"046d", "c52b"}, // other vendor
		"event3":  {"1189", "1234"}, // other product
		"event4":  {"1189", ""},     // broken
		"mouse2":  {"1189", "8890"}, // not an event node
		"event5":  {"1189", "8842"},
	})
	got, err := DiscoverIn(root, "/dev/input")
	want := []string{"/dev/input/event5", "/dev/input/event9", "/dev/input/event10", "/dev/input/event31"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := DiscoverIn(fakeSys(t, map[string][2]string{"event1": {"046d", "c52b"}}), "/dev/input"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no pad: %v", err)
	}
	if _, err := DiscoverIn(filepath.Join(t.TempDir(), "missing"), "/dev/input"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing root: %v", err)
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "event0")
	if err := os.WriteFile(locked, nil, 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if _, err := Open(locked); !errors.Is(err, ErrPermission) {
			t.Fatalf("permission: %v", err)
		}
		if _, err := OpenPaths([]string{locked}); !errors.Is(err, ErrPermission) {
			t.Fatalf("OpenPaths permission: %v", err)
		}
	}
	if _, err := Open(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("missing node opened")
	}
	// A regular file opens but cannot be grabbed (ENOTTY); nothing stays open.
	plain := filepath.Join(dir, "event1")
	_ = os.WriteFile(plain, nil, 0o644)
	if _, err := OpenPaths([]string{plain}); err == nil {
		t.Fatal("grab on a regular file succeeded")
	}
	// Observe mode does not grab, so a regular file opens fine.
	obs, err := ObservePaths([]string{plain})
	if err != nil || !reflect.DeepEqual(obs.Paths(), []string{plain}) {
		t.Fatalf("observe: %v", err)
	}
	if err := obs.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ObservePaths([]string{plain, filepath.Join(dir, "nope")}); err == nil {
		t.Fatal("observe with a missing node succeeded")
	}
	d, err := Open(plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Release(); err != nil { // not grabbed: no-op
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (&Pad{}).Read(context.Background(), func(KeyEvent) {}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty pad read: %v", err)
	}
}

func TestDeviceReadFromPipe(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pw.Close() }()
	d := &Device{Path: "pipe", f: pr}
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan KeyEvent, 4)
	done := make(chan error, 1)
	go func() { done <- d.Read(ctx, func(e KeyEvent) { got <- e }) }()
	if EventSize == 24 {
		_, _ = pw.Write(bytes.Join([][]byte{raw(1, 0, 1, 184, 1), raw(1, 0, 0, 0, 0)}, nil))
		select {
		case e := <-got:
			if e.Code != 184 || !e.Down {
				t.Fatalf("event %+v", e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no event")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("read returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read not cancelled")
	}
	_ = d.Close()
}

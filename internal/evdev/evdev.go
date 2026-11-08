// Package evdev reads key events from the pad's Linux input nodes
// (/dev/input/event*). Two modes: exclusive (OpenPad: EVIOCGRAB, so the raw
// gesture-mode keys never reach other applications) and observe (ObservePad:
// no grab, keys still reach the focused app). Pure Go, no cgo.
package evdev

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Supported pads (vendor 0x1189); kept in sync with internal/pad.
const VendorID = 0x1189

var ProductIDs = []uint16{0x8890, 0x8840, 0x8842}

var (
	// ErrNotFound means no input node of a supported pad exists.
	ErrNotFound = errors.New("no input device of the pad found")
	// ErrPermission means the nodes exist but cannot be opened (udev rule missing).
	ErrPermission = errors.New("permission denied opening the pad's input devices (install the udev rule: just udev-install)")
	// ErrDisconnected means the pad went away while reading.
	ErrDisconnected = errors.New("pad disconnected")
)

// Linux input constants (input-event-codes.h, input.h).
const (
	evKey = 0x01

	// EVIOCGRAB = _IOW('E', 0x90, int)
	eviocgrab = 1<<30 | 4<<16 | 'E'<<8 | 0x90
)

// KeyEvent is a key press or release. Repeats (value 2) are dropped.
type KeyEvent struct {
	Code uint16
	Down bool
	Time time.Time // kernel timestamp (CLOCK_REALTIME)
}

// timevalSize is sizeof(struct timeval) on this platform (16 on 64-bit).
const timevalSize = int(unsafe.Sizeof(unix.Timeval{}))

// EventSize is sizeof(struct input_event): timeval + u16 type + u16 code + s32 value.
const EventSize = timevalSize + 8

// ParseEvents decodes raw input_event structs (native little endian, with a
// timeval of tvSize bytes: 16 on 64-bit, 8 on 32-bit) and returns key
// presses and releases. Trailing partial events are ignored.
func ParseEvents(b []byte, tvSize int) []KeyEvent {
	size := tvSize + 8
	var out []KeyEvent
	for ; len(b) >= size; b = b[size:] {
		typ := binary.LittleEndian.Uint16(b[tvSize:])
		code := binary.LittleEndian.Uint16(b[tvSize+2:])
		val := int32(binary.LittleEndian.Uint32(b[tvSize+4:]))
		if typ != evKey || (val != 0 && val != 1) {
			continue
		}
		var sec, usec int64
		if tvSize == 16 {
			sec = int64(binary.LittleEndian.Uint64(b[0:]))
			usec = int64(binary.LittleEndian.Uint64(b[8:]))
		} else {
			sec = int64(int32(binary.LittleEndian.Uint32(b[0:])))
			usec = int64(int32(binary.LittleEndian.Uint32(b[4:])))
		}
		out = append(out, KeyEvent{Code: code, Down: val == 1, Time: time.Unix(sec, usec*1000)})
	}
	return out
}

// Discover returns the pad's event nodes (e.g. /dev/input/event9), sorted.
func Discover() ([]string, error) { return DiscoverIn("/sys/class/input", "/dev/input") }

// DiscoverIn is Discover with custom sysfs and /dev roots (for tests). An
// event node matches when <sysRoot>/eventN/device/id/{vendor,product} name a
// supported pad.
func DiscoverIn(sysRoot, devRoot string) ([]string, error) {
	entries, err := os.ReadDir(sysRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	var out []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "event") {
			continue
		}
		id := filepath.Join(sysRoot, e.Name(), "device", "id")
		vid, ok1 := readHex(filepath.Join(id, "vendor"))
		pid, ok2 := readHex(filepath.Join(id, "product"))
		if ok1 && ok2 && vid == VendorID && slices.Contains(ProductIDs, pid) {
			out = append(out, filepath.Join(devRoot, e.Name()))
		}
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	sort.Slice(out, func(i, j int) bool { return eventNum(out[i]) < eventNum(out[j]) })
	return out, nil
}

func eventNum(p string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(p), "event"))
	return n
}

func readHex(path string) (uint16, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 16, 16)
	return uint16(v), err == nil
}

// Device is one opened input node.
type Device struct {
	Path string
	f    *os.File

	mu      sync.Mutex
	grabbed bool
}

// Open opens an input node read-only.
func Open(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("%w: %s", ErrPermission, path)
		}
		return nil, err
	}
	return &Device{Path: path, f: f}, nil
}

func ioctlInt(fd uintptr, req, val uintptr) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, req, val); errno != 0 {
		return errno
	}
	return nil
}

// Grab takes the device exclusively (EVIOCGRAB 1).
func (d *Device) Grab() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.ioctl(eviocgrab, 1); err != nil {
		return fmt.Errorf("grab %s: %w", d.Path, err)
	}
	d.grabbed = true
	return nil
}

// Release ungrabs the device (EVIOCGRAB 0). Safe to call when not grabbed.
func (d *Device) Release() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.grabbed {
		return nil
	}
	d.grabbed = false
	if err := d.ioctl(eviocgrab, 0); err != nil {
		return fmt.Errorf("release %s: %w", d.Path, err)
	}
	return nil
}

func (d *Device) ioctl(req, val uintptr) error {
	rc, err := d.f.SyscallConn()
	if err != nil {
		return err
	}
	var ierr error
	if err := rc.Control(func(fd uintptr) { ierr = ioctlInt(fd, req, val) }); err != nil {
		return err
	}
	return ierr
}

// Close releases the grab and closes the node.
func (d *Device) Close() error {
	rerr := d.Release()
	cerr := d.f.Close()
	if errors.Is(cerr, os.ErrClosed) {
		cerr = nil
	}
	return errors.Join(rerr, cerr)
}

// Read calls fn for each key event until ctx is done (returns ctx.Err()) or
// the device fails (ErrDisconnected when the pad is unplugged).
func (d *Device) Read(ctx context.Context, fn func(KeyEvent)) error {
	stop := context.AfterFunc(ctx, func() {
		// Unblocks the pending read; input nodes support the poller.
		if d.f.SetReadDeadline(time.Now()) != nil {
			_ = d.f.Close()
		}
	})
	defer stop()
	_ = d.f.SetReadDeadline(time.Time{}) // clear a deadline left by an earlier cancel
	return readLoop(ctx, d.f, timevalSize, fn)
}

func readLoop(ctx context.Context, r io.Reader, tvSize int, fn func(KeyEvent)) error {
	size := tvSize + 8
	buf := make([]byte, size*64)
	for {
		n, err := r.Read(buf)
		// evdev always returns whole events.
		for _, ev := range ParseEvents(buf[:n-n%size], tvSize) {
			fn(ev)
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, syscall.ENODEV) || errors.Is(err, io.EOF) {
				return ErrDisconnected
			}
			return fmt.Errorf("read input: %w", err)
		}
	}
}

// Pad is the set of the pad's grabbed input nodes.
type Pad struct {
	devs []*Device
}

// OpenPad discovers, opens and grabs every input node of the pad
// (exclusive: gesture mode). Raw keys no longer reach other applications.
func OpenPad() (*Pad, error) {
	paths, err := Discover()
	if err != nil {
		return nil, err
	}
	return OpenPaths(paths)
}

// ObservePad discovers and opens every input node of the pad without
// grabbing (observe mode): keys keep reaching the focused application and
// agentpad sees them too, e.g. to animate the pad drawing in plain mode.
// While gesture mode holds the grab, an observer receives nothing.
func ObservePad() (*Pad, error) {
	paths, err := Discover()
	if err != nil {
		return nil, err
	}
	return ObservePaths(paths)
}

// OpenPaths opens and grabs the given nodes; on error nothing stays open.
func OpenPaths(paths []string) (*Pad, error) { return openPaths(paths, true) }

// ObservePaths opens the given nodes without grabbing them.
func ObservePaths(paths []string) (*Pad, error) { return openPaths(paths, false) }

func openPaths(paths []string, grab bool) (*Pad, error) {
	p := &Pad{}
	for _, path := range paths {
		d, err := Open(path)
		if err == nil && grab {
			err = d.Grab()
			if err != nil {
				_ = d.Close()
			}
		}
		if err != nil {
			_ = p.Close()
			return nil, err
		}
		p.devs = append(p.devs, d)
	}
	return p, nil
}

// Paths returns the opened node paths.
func (p *Pad) Paths() []string {
	out := make([]string, len(p.devs))
	for i, d := range p.devs {
		out[i] = d.Path
	}
	return out
}

// Read merges key events from all nodes (fn is called from one goroutine at
// a time) until ctx is done or any node fails.
func (p *Pad) Read(ctx context.Context, fn func(KeyEvent)) error {
	if len(p.devs) == 0 {
		return ErrNotFound
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	errs := make(chan error, len(p.devs))
	for _, d := range p.devs {
		go func() {
			errs <- d.Read(ctx, func(ev KeyEvent) {
				mu.Lock()
				defer mu.Unlock()
				fn(ev)
			})
		}()
	}
	first := <-errs
	cancel()
	for range len(p.devs) - 1 {
		if err := <-errs; first == nil {
			first = err
		}
	}
	return first
}

// Close ungrabs and closes all nodes.
func (p *Pad) Close() error {
	var errs []error
	for _, d := range p.devs {
		errs = append(errs, d.Close())
	}
	p.devs = nil
	return errors.Join(errs...)
}

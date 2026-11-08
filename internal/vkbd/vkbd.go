// Package vkbd emits profile actions as key presses through a virtual
// keyboard: /dev/uinput (preferred, pure Go) or the ydotool CLI (needs a
// running ydotoold). Used by gesture mode.
package vkbd

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Emitter sends actions (profile action grammar) to the focused application.
type Emitter interface {
	Emit(action string) error
	Close() error
}

// ErrNoBackend means neither uinput nor ydotool can be used.
var ErrNoBackend = errors.New("no virtual keyboard: /dev/uinput not writable (install the udev rule: just udev-install) and ydotool/ydotoold not available")

// New returns a uinput emitter if /dev/uinput is writable, else a ydotool
// emitter if ydotool and its daemon socket are available.
func New() (Emitter, error) {
	return pick(func() (Emitter, error) { return NewUinput("/dev/uinput") },
		func() (Emitter, error) { return NewYdotool() })
}

func pick(backends ...func() (Emitter, error)) (Emitter, error) {
	errs := []error{ErrNoBackend}
	for _, b := range backends {
		e, err := b()
		if err == nil {
			return e, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

// ---- uinput ----

// Linux input / uinput constants (input-event-codes.h, uinput.h).
const (
	evSyn     = 0x00
	evKey     = 0x01
	synReport = 0
	busVirt   = 0x06

	uiSetEvBit   = 1<<30 | 4<<16 | 'U'<<8 | 100 // _IOW('U', 100, int)
	uiSetKeyBit  = 1<<30 | 4<<16 | 'U'<<8 | 101 // _IOW('U', 101, int)
	uiDevSetup   = 1<<30 | 92<<16 | 'U'<<8 | 3  // _IOW('U', 3, struct uinput_setup)
	uiDevCreate  = 'U'<<8 | 1                   // _IO('U', 1)
	uiDevDestroy = 'U'<<8 | 2                   // _IO('U', 2)
)

// settle is how long a fresh uinput device is given before the first event,
// so the compositor/libinput has picked it up and nothing is lost.
const settle = 300 * time.Millisecond

type uinputSetup struct {
	Bustype, Vendor, Product, Version uint16
	Name                              [80]byte
	FFEffectsMax                      uint32
}

// Uinput is a virtual keyboard on /dev/uinput.
type Uinput struct {
	mu      sync.Mutex
	f       *os.File
	w       io.Writer // event sink; f in production
	tvSize  int
	readyAt time.Time
	sleep   func(time.Duration)
}

// NewUinput creates the virtual keyboard "agentpad gestures" on path.
func NewUinput(path string) (*Uinput, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("uinput: %w (no write access to %s)", err, path)
		}
		return nil, fmt.Errorf("uinput: %w", err)
	}
	if err := setupUinput(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("uinput setup: %w", err)
	}
	return &Uinput{f: f, w: f, tvSize: int(unsafe.Sizeof(unix.Timeval{})),
		readyAt: time.Now().Add(settle), sleep: time.Sleep}, nil
}

func setupUinput(f *os.File) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ierr error
	cerr := rc.Control(func(fd uintptr) {
		ioctl := func(req, arg uintptr) error {
			if _, _, e := unix.Syscall(unix.SYS_IOCTL, fd, req, arg); e != 0 {
				return fmt.Errorf("ioctl %#x: %w", req, e)
			}
			return nil
		}
		if ierr = ioctl(uiSetEvBit, evKey); ierr != nil {
			return
		}
		for _, c := range AllCodes() {
			if ierr = ioctl(uiSetKeyBit, uintptr(c)); ierr != nil {
				return
			}
		}
		s := uinputSetup{Bustype: busVirt, Vendor: 0x1189, Product: 0x0001, Version: 1}
		copy(s.Name[:], "agentpad gestures")
		if _, _, e := unix.Syscall(unix.SYS_IOCTL, fd, uiDevSetup, uintptr(unsafe.Pointer(&s))); e != 0 {
			ierr = fmt.Errorf("UI_DEV_SETUP: %w", e)
			return
		}
		ierr = ioctl(uiDevCreate, 0)
	})
	return errors.Join(cerr, ierr)
}

// encode writes one input_event with a zero timestamp (the kernel sets it).
func encode(buf *bytes.Buffer, tvSize int, typ, code uint16, val int32) {
	buf.Write(make([]byte, tvSize))
	_ = binary.Write(buf, binary.LittleEndian, typ)
	_ = binary.Write(buf, binary.LittleEndian, code)
	_ = binary.Write(buf, binary.LittleEndian, val)
}

// encodeEvents renders key events, each followed by SYN_REPORT.
func encodeEvents(evs []KeyEvent, tvSize int) []byte {
	var buf bytes.Buffer
	for _, e := range evs {
		v := int32(0)
		if e.Down {
			v = 1
		}
		encode(&buf, tvSize, evKey, e.Code, v)
		encode(&buf, tvSize, evSyn, synReport, 0)
	}
	return buf.Bytes()
}

// Emit types action.
func (u *Uinput) Emit(action string) error {
	combos, err := Translate(action)
	if err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.w == nil {
		return errors.New("uinput: closed")
	}
	if d := time.Until(u.readyAt); d > 0 {
		u.sleep(d)
	}
	_, err = u.w.Write(encodeEvents(Events(combos), u.tvSize))
	return err
}

// Close destroys the virtual keyboard.
func (u *Uinput) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.w = nil
	if u.f == nil {
		return nil
	}
	f := u.f
	u.f = nil
	if rc, err := f.SyscallConn(); err == nil {
		_ = rc.Control(func(fd uintptr) { _, _, _ = unix.Syscall(unix.SYS_IOCTL, fd, uiDevDestroy, 0) })
	}
	return f.Close()
}

// ---- ydotool ----

// Ydotool emits keys with `ydotool key <code>:1 <code>:0 ...`.
type Ydotool struct {
	Path   string // ydotool binary
	Socket string // ydotoold socket, passed as YDOTOOL_SOCKET

	run func(ctx context.Context, path string, args, env []string) error
}

const ydotoolTimeout = 5 * time.Second

// NewYdotool finds ydotool on PATH and a ydotoold socket ($YDOTOOL_SOCKET,
// /run/ydotoold.socket, $XDG_RUNTIME_DIR/.ydotool_socket).
func NewYdotool() (*Ydotool, error) {
	path, err := exec.LookPath("ydotool")
	if err != nil {
		return nil, fmt.Errorf("ydotool: %w", err)
	}
	sock := findSocket(socketCandidates(os.Getenv("YDOTOOL_SOCKET"), os.Getenv("XDG_RUNTIME_DIR")))
	if sock == "" {
		return nil, errors.New("ydotool: no ydotoold socket found (is ydotoold running?)")
	}
	return &Ydotool{Path: path, Socket: sock}, nil
}

func socketCandidates(env, runtimeDir string) []string {
	var c []string
	if env != "" {
		c = append(c, env)
	}
	c = append(c, "/run/ydotoold.socket")
	if runtimeDir != "" {
		c = append(c, filepath.Join(runtimeDir, ".ydotool_socket"))
	}
	return c
}

func findSocket(candidates []string) string {
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.Mode()&fs.ModeSocket != 0 {
			return p
		}
	}
	return ""
}

// ydotoolArgs builds the argv after the binary for an action.
func ydotoolArgs(action string) ([]string, error) {
	combos, err := Translate(action)
	if err != nil {
		return nil, err
	}
	args := []string{"key"}
	for _, e := range Events(combos) {
		v := "0"
		if e.Down {
			v = "1"
		}
		args = append(args, strconv.Itoa(int(e.Code))+":"+v)
	}
	return args, nil
}

// Emit types action through ydotool.
func (y *Ydotool) Emit(action string) error {
	args, err := ydotoolArgs(action)
	if err != nil {
		return err
	}
	var env []string
	if y.Socket != "" {
		env = []string{"YDOTOOL_SOCKET=" + y.Socket}
	}
	run := y.run
	if run == nil {
		run = execRun
	}
	ctx, cancel := context.WithTimeout(context.Background(), ydotoolTimeout)
	defer cancel()
	if err := run(ctx, y.Path, args, env); err != nil {
		return fmt.Errorf("ydotool %s: %w", action, err)
	}
	return nil
}

// Close is a no-op.
func (y *Ydotool) Close() error { return nil }

func execRun(ctx context.Context, path string, args, env []string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}
	return err
}

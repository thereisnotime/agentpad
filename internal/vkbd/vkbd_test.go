package vkbd

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestIoctlNumbers(t *testing.T) {
	if unsafe.Sizeof(uinputSetup{}) != 92 {
		t.Fatalf("sizeof(uinput_setup) = %d", unsafe.Sizeof(uinputSetup{}))
	}
	want := map[string][2]uint32{
		"UI_SET_EVBIT": {uiSetEvBit, 0x40045564}, "UI_SET_KEYBIT": {uiSetKeyBit, 0x40045565},
		"UI_DEV_SETUP": {uiDevSetup, 0x405c5503}, "UI_DEV_CREATE": {uiDevCreate, 0x5501},
		"UI_DEV_DESTROY": {uiDevDestroy, 0x5502},
	}
	for name, v := range want {
		if v[0] != v[1] {
			t.Errorf("%s = %#x, want %#x", name, v[0], v[1])
		}
	}
}

func TestTranslate(t *testing.T) {
	tests := []struct {
		action string
		want   []Combo
	}{
		{"a", []Combo{{Key: 30}}},
		{"1", []Combo{{Key: 2}}},
		{"escape", []Combo{{Key: 1}}},
		{"ctrl-c", []Combo{{Mods: []uint16{29}, Key: 46}}},
		{"shift-tab", []Combo{{Mods: []uint16{42}, Key: 15}}},
		{"ctrl-shift-a", []Combo{{Mods: []uint16{29, 42}, Key: 30}}},
		{"rwin-f24", []Combo{{Mods: []uint16{126}, Key: 194}}},
		{"ctrl-a,b", []Combo{{Mods: []uint16{29}, Key: 30}, {Key: 48}}},
		{"escape,escape", []Combo{{Key: 1}, {Key: 1}}},
		{"ctrl-shift", []Combo{{Mods: []uint16{29, 42}}}},
		{"volumeup", []Combo{{Key: 115}}},
		{"prev", []Combo{{Key: 165}}},
		{"<4>", []Combo{{Key: 30}}},
		{"ctrl-<40>", []Combo{{Mods: []uint16{29}, Key: 28}}},
		{"f13", []Combo{{Key: 183}}},
	}
	for _, tt := range tests {
		got, err := Translate(tt.action)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q = %v, %v; want %v", tt.action, got, err, tt.want)
		}
	}
	for _, a := range []string{"click", "wheel(-5)", "ctrl-move(1,-1)", "drag(left,0,5)", "rclick", "<200>"} {
		if _, err := Translate(a); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%q: want ErrUnsupported, got %v", a, err)
		}
	}
	for _, a := range []string{"", "foo", "ctrl-foo", "ctrl-volumeup", "a,,b", "<999>"} {
		if _, err := Translate(a); err == nil {
			t.Errorf("%q: want error", a)
		}
	}
}

// Every action the profile grammar accepts translates, except mouse actions.
func TestTranslateCoversProfileGrammar(t *testing.T) {
	for _, a := range profile.SupportedKeys() {
		_, err := Translate(a)
		mouse := strings.ContainsAny(a, "(") || strings.Contains(a, "click") || strings.HasPrefix(a, "wheel")
		if mouse != errors.Is(err, ErrUnsupported) || (!mouse && err != nil) {
			t.Errorf("%q: %v", a, err)
		}
	}
}

// keyMacros names the input-event-codes.h macro for each key name.
var keyMacros = map[string]string{
	"enter": "KEY_ENTER", "escape": "KEY_ESC", "backspace": "KEY_BACKSPACE", "tab": "KEY_TAB",
	"space": "KEY_SPACE", "minus": "KEY_MINUS", "equal": "KEY_EQUAL", "leftbracket": "KEY_LEFTBRACE",
	"rightbracket": "KEY_RIGHTBRACE", "backslash": "KEY_BACKSLASH", "nonushash": "KEY_BACKSLASH",
	"semicolon": "KEY_SEMICOLON", "quote": "KEY_APOSTROPHE", "grave": "KEY_GRAVE", "comma": "KEY_COMMA",
	"dot": "KEY_DOT", "slash": "KEY_SLASH", "capslock": "KEY_CAPSLOCK", "printscreen": "KEY_SYSRQ",
	"macbrightnessdown": "KEY_SCROLLLOCK", "macbrightnessup": "KEY_PAUSE", "insert": "KEY_INSERT",
	"home": "KEY_HOME", "pageup": "KEY_PAGEUP", "delete": "KEY_DELETE", "end": "KEY_END",
	"pagedown": "KEY_PAGEDOWN", "right": "KEY_RIGHT", "left": "KEY_LEFT", "down": "KEY_DOWN", "up": "KEY_UP",
	"numlock": "KEY_NUMLOCK", "numpadslash": "KEY_KPSLASH", "numpadasterisk": "KEY_KPASTERISK",
	"numpadminus": "KEY_KPMINUS", "numpadplus": "KEY_KPPLUS", "numpadenter": "KEY_KPENTER",
	"numpaddot": "KEY_KPDOT", "nonusbackslash": "KEY_102ND", "application": "KEY_COMPOSE",
	"power": "KEY_POWER", "numpadequal": "KEY_KPEQUAL",
	"ctrl": "KEY_LEFTCTRL", "shift": "KEY_LEFTSHIFT", "alt": "KEY_LEFTALT", "win": "KEY_LEFTMETA",
	"rctrl": "KEY_RIGHTCTRL", "rshift": "KEY_RIGHTSHIFT", "ralt": "KEY_RIGHTALT", "rwin": "KEY_RIGHTMETA",
	"next": "KEY_NEXTSONG", "previous": "KEY_PREVIOUSSONG", "stop": "KEY_STOPCD", "play": "KEY_PLAYPAUSE",
	"mute": "KEY_MUTE", "volumeup": "KEY_VOLUMEUP", "volumedown": "KEY_VOLUMEDOWN",
	"favorites": "KEY_FAVORITES", "calculator": "KEY_CALC", "screenlock": "KEY_SCREENLOCK",
}

func macroFor(name string) string {
	if m, ok := keyMacros[name]; ok {
		return m
	}
	if strings.HasPrefix(name, "numpad") {
		return "KEY_KP" + strings.TrimPrefix(name, "numpad")
	}
	return "KEY_" + strings.ToUpper(name)
}

// Cross-checks the tables against the system header when it is installed.
func TestCodesMatchKernelHeader(t *testing.T) {
	f, err := os.Open("/usr/include/linux/input-event-codes.h")
	if err != nil {
		t.Skip("kernel headers not installed")
	}
	defer func() { _ = f.Close() }()
	defs := map[string]string{}
	re := regexp.MustCompile(`^#define\s+(KEY_\w+)\s+(\w+)`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			defs[m[1]] = m[2]
		}
	}
	resolve := func(macro string) (uint16, bool) {
		for range 4 { // aliases like KEY_SCREENLOCK -> KEY_COFFEE
			v, ok := defs[macro]
			if !ok {
				return 0, false
			}
			if n, err := strconv.ParseUint(v, 0, 16); err == nil {
				return uint16(n), true
			}
			macro = v
		}
		return 0, false
	}
	check := func(name string, code uint16) {
		want, ok := resolve(macroFor(name))
		if !ok {
			t.Errorf("%s: macro %s not in header", name, macroFor(name))
		} else if want != code {
			t.Errorf("%s = %d, header %s = %d", name, code, macroFor(name), want)
		}
	}
	for name, k := range keys {
		check(name, k.linux)
	}
	for _, name := range []string{"ctrl", "shift", "alt", "win", "rctrl", "rshift", "ralt", "rwin"} {
		check(name, modifiers[name])
	}
	for name, code := range media {
		if name != "prev" {
			check(name, code)
		}
	}
}

func TestEventsAndEncoding(t *testing.T) {
	combos, _ := Translate("ctrl-shift-a,b")
	got := Events(combos)
	want := []KeyEvent{{29, true}, {42, true}, {30, true}, {30, false}, {42, false}, {29, false}, {48, true}, {48, false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events %v", got)
	}
	b := encodeEvents(want[:1], 16)
	if len(b) != 48 {
		t.Fatalf("encoded %d bytes", len(b))
	}
	if typ, code, val := binary.LittleEndian.Uint16(b[16:]), binary.LittleEndian.Uint16(b[18:]), binary.LittleEndian.Uint32(b[20:]); typ != 1 || code != 29 || val != 1 {
		t.Fatalf("key event %d %d %d", typ, code, val)
	}
	if typ, code := binary.LittleEndian.Uint16(b[40:]), binary.LittleEndian.Uint16(b[42:]); typ != 0 || code != 0 {
		t.Fatal("missing SYN_REPORT")
	}
	if len(AllCodes()) < 100 {
		t.Fatalf("AllCodes: %d", len(AllCodes()))
	}
}

type sink struct{ b []byte }

func (s *sink) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }

func TestUinputEmitWithFakeSink(t *testing.T) {
	s := &sink{}
	var slept time.Duration
	u2 := &Uinput{w: s, tvSize: 16, readyAt: time.Now().Add(time.Hour), sleep: func(d time.Duration) { slept = d }}
	if err := u2.Emit("escape"); err != nil {
		t.Fatal(err)
	}
	if slept < 59*time.Minute {
		t.Fatalf("did not wait for the device to settle: %v", slept)
	}
	if len(s.b) != 4*24 {
		t.Fatalf("wrote %d bytes", len(s.b))
	}
	if err := u2.Emit("click"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("mouse: %v", err)
	}
	if err := u2.Close(); err != nil {
		t.Fatal(err)
	}
	if err := u2.Emit("a"); err == nil {
		t.Fatal("emit after close")
	}
}

func TestNewUinputFailsWithoutDevice(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewUinput(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("opened missing uinput")
	}
	plain := filepath.Join(dir, "uinput")
	_ = os.WriteFile(plain, nil, 0o600)
	if _, err := NewUinput(plain); err == nil { // ioctls fail with ENOTTY
		t.Fatal("regular file accepted as uinput")
	}
}

func TestYdotool(t *testing.T) {
	args, err := ydotoolArgs("ctrl-c")
	if err != nil || !reflect.DeepEqual(args, []string{"key", "29:1", "46:1", "46:0", "29:0"}) {
		t.Fatalf("args %v %v", args, err)
	}
	var gotPath string
	var gotArgs, gotEnv []string
	y := &Ydotool{Path: "/usr/bin/ydotool", Socket: "/run/ydotoold.socket",
		run: func(_ context.Context, path string, args, env []string) error {
			gotPath, gotArgs, gotEnv = path, args, env
			return nil
		}}
	if err := y.Emit("volumeup"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/usr/bin/ydotool" || !reflect.DeepEqual(gotArgs, []string{"key", "115:1", "115:0"}) ||
		!reflect.DeepEqual(gotEnv, []string{"YDOTOOL_SOCKET=/run/ydotoold.socket"}) {
		t.Fatalf("ran %s %v %v", gotPath, gotArgs, gotEnv)
	}
	y.run = func(context.Context, string, []string, []string) error { return errors.New("boom") }
	if err := y.Emit("a"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("run error: %v", err)
	}
	if err := y.Emit("wheelup"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("mouse: %v", err)
	}
	if y.Close() != nil {
		t.Fatal("close")
	}
}

func TestFindSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "vk") // short path: unix socket names are limited
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("unix sockets unavailable:", err)
	}
	defer func() { _ = l.Close() }()
	plain := filepath.Join(dir, "plain")
	_ = os.WriteFile(plain, nil, 0o600)
	if got := findSocket([]string{filepath.Join(dir, "none"), plain, sock}); got != sock {
		t.Fatalf("findSocket = %q", got)
	}
	if got := findSocket([]string{plain}); got != "" {
		t.Fatalf("plain file accepted: %q", got)
	}
	c := socketCandidates("/x/env.sock", "/run/user/1000")
	if !reflect.DeepEqual(c, []string{"/x/env.sock", "/run/ydotoold.socket", "/run/user/1000/.ydotool_socket"}) {
		t.Fatalf("candidates %v", c)
	}
	if c := socketCandidates("", ""); !reflect.DeepEqual(c, []string{"/run/ydotoold.socket"}) {
		t.Fatalf("candidates %v", c)
	}
}

type fakeEmitter struct{ name string }

func (f *fakeEmitter) Emit(string) error { return nil }
func (f *fakeEmitter) Close() error      { return nil }

func TestPick(t *testing.T) {
	fail := func(msg string) func() (Emitter, error) {
		return func() (Emitter, error) { return nil, errors.New(msg) }
	}
	ok := func(name string) func() (Emitter, error) {
		return func() (Emitter, error) { return &fakeEmitter{name}, nil }
	}
	if e, err := pick(ok("uinput"), ok("ydotool")); err != nil || e.(*fakeEmitter).name != "uinput" {
		t.Fatal("prefers first")
	}
	if e, err := pick(fail("no uinput"), ok("ydotool")); err != nil || e.(*fakeEmitter).name != "ydotool" {
		t.Fatal("falls back")
	}
	_, err := pick(fail("no uinput"), fail("no ydotool"))
	if !errors.Is(err, ErrNoBackend) || !strings.Contains(err.Error(), "no uinput") || !strings.Contains(err.Error(), "no ydotool") {
		t.Fatalf("both fail: %v", err)
	}
}

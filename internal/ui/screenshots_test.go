//go:build screenshots

package ui

// Renders the README screenshots offscreen with Fyne's software painter:
//
//	SHOT_DIR=docs/screenshots go test -tags 'wayland screenshots' -run TestScreenshots ./internal/ui
//
// Nothing touches the real pad: the tool path does not exist and no
// receiver is started.

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/jump"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

type noJumper struct{}

func (noJumper) Jump(context.Context, events.Session) jump.Report { return jump.Report{} }
func (noJumper) Close() error                                     { return nil }

type silentInput struct{ ch chan PadInputEvent }

func (s silentInput) Events() <-chan PadInputEvent { return s.ch }

func TestScreenshots(t *testing.T) {
	dir := os.Getenv("SHOT_DIR")
	if dir == "" {
		t.Skip("SHOT_DIR not set")
	}
	tmp := t.TempDir()
	a := test.NewTempApp(t)
	u := newAppUI(a, Deps{
		Writer:  &pad.ToolWriter{Path: filepath.Join(tmp, "no-such-tool")},
		Store:   profile.NewFileStore(filepath.Join(tmp, "profiles")),
		Bin:     "/home/me/.local/bin/agentpad",
		Version: "v0.3.0",
		LogPath: "/home/me/.local/state/agentpad/events.jsonl",
		Addr:    "127.0.0.1:47321",
		Input:   silentInput{make(chan PadInputEvent)},
		Live:    LiveDevices{Disabled: true},
		Jumper:  noJumper{},
		Notifier: func(func(string)) (NotifyBackend, error) {
			return nil, errors.New("no notifications in screenshots")
		},
	})
	u.ledFollow.Store(false)
	t.Cleanup(u.shutdown)
	u.win.SetPadded(false)
	u.sessions.home = "/home/me"
	u.showDevice(pad.Device{VendorID: 0x1189, ProductID: 0x8890, BusPath: "1-4", Product: "CH57x pad"}, nil)
	u.pillRecv.set(colorSuccess, "Receiver live")
	u.pad.toolIcon.SetResource(theme.NewSuccessThemedResource(theme.ConfirmIcon()))
	u.pad.toolTitle.SetText("ch57x-keyboard-tool ready")
	u.pad.toolHint.Hide()

	now := time.Now()
	for _, e := range []events.Event{
		{Time: now.Add(-3 * time.Minute), Agent: "codex", SessionID: "019a6c2e-77", Kind: events.KindPrompt, Cwd: "/home/me/src/agentpad"},
		{Time: now.Add(-40 * time.Second), Agent: "opencode", SessionID: "ses_5f2a91", Kind: events.KindIdle, Cwd: "/home/me/src/website", Tool: "edit"},
		{Time: now.Add(-8 * time.Second), Agent: "claude", SessionID: "3f9c1d2e-aa41", Kind: events.KindToolPending, Tool: "Bash", Cwd: "/home/me/src/agentpad", Message: "Claude needs your permission to use Bash"},
	} {
		u.onEvent(e)
	}

	shot := func(name string, screen int, w, h float32, prep func()) {
		u.navigate(screen)
		if prep != nil {
			prep()
		}
		c := u.win.Canvas().(test.WindowlessCanvas)
		c.SetScale(2)
		u.win.Resize(fyne.NewSize(w, h))
		// let size-dependent layouts settle
		for range 3 {
			u.win.Content().Refresh()
			u.win.Resize(fyne.NewSize(w, h))
			c.Capture()
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(f, c.Capture()); err != nil {
			t.Fatal(err)
		}
	}

	live := func() {
		u.profiles.selectControl(ctlKnobPress)
		u.profiles.showCaption(inputCaption(ctlKnobCW))
		u.profiles.view.controls[ctlKnobCW].press = 1
		u.profiles.view.controls[ctlKnobPress].angle = 30
		u.profiles.view.controls[ctlKnobCW].Refresh()
		u.profiles.view.controls[ctlKnobPress].Refresh()
	}
	shot("profiles-wide", screenProfiles, 1200, 760, nil)
	shot("profiles-narrow", screenProfiles, 500, 1100, nil)
	shot("profiles-live", screenProfiles, 1200, 760, live)
	shot("profiles-vertical", screenProfiles, 1200, 900, func() {
		u.profiles.view.controls[ctlKnobCW].press = 0
		u.profiles.view.controls[ctlKnobPress].angle = 0
		u.profiles.rotate(profile.OrientClockwise)
		u.profiles.selectControl(ctlKey2)
	})
	u.profiles.rotate(profile.OrientNormal)
	shot("pad-wide", screenPad, 1200, 1180, nil)
	shot("pad-narrow", screenPad, 500, 1700, nil)
	shot("sessions-wide", screenSessions, 1200, 700, nil)
	shot("sessions-jump", screenSessions, 500, 900, nil)

	// gesture mode: inspector with gesture slots, monitor, mode card
	gst := liveStatus{Mode: ModeGesture, Reading: true, Backend: "uinput"}
	u.mode = ModeGesture
	u.profiles.setMode(ModeGesture)
	u.pad.showMode(ModeGesture, gst)
	for i, ev := range []gesture.Event{
		{Gesture: gesture.Gesture{Control: gesture.KnobCW, Kind: gesture.PressRotate}, Action: "shift-tab"},
		{Gesture: gesture.Gesture{Control: gesture.Key1, Kind: gesture.Tap}, Action: "1"},
		{Gesture: gesture.Gesture{Control: "key1+key3", Kind: gesture.Chord}},
		{Gesture: gesture.Gesture{Control: gesture.Key3, Kind: gesture.Hold}, Action: "ctrl-c"},
	} {
		ev.Time = time.Now().Add(time.Duration(i-4) * time.Second)
		u.profiles.monitorAdd(gestureLine(ev))
	}
	shot("profiles-gestures", screenProfiles, 1200, 1340, func() { u.profiles.selectControl(ctlKey3) })
	shot("pad-gesture", screenPad, 1200, 1180, nil)
	u.mode = ModePlain
	u.profiles.setMode(ModePlain)
	u.pad.showMode(ModePlain, liveStatus{Mode: ModePlain})
	shot("log-wide", screenLog, 1200, 600, nil)
	shot("setup-wide", screenSetup, 1200, 900, nil)
	shot("about-wide", screenAbout, 1200, 820, nil)

	// 884x LED panel
	u.showDevice(pad.Device{VendorID: 0x1189, ProductID: 0x8842, BusPath: "1-4", Product: "CH57x pad"}, nil)
	shot("pad-884x-wide", screenPad, 1200, 1100, nil)

	if err := os.MkdirAll(filepath.Join(dir, "..", "..", "assets"), 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "..", "..", "assets", "agentpad.svg"), padIconSVG(iconVariantApp), 0o644)
	}
	for v, n := range map[iconVariant]string{iconVariantIdle: "idle", iconVariantBusy: "busy", iconVariantAttention: "attention", iconVariantApp: "app"} {
		_ = os.WriteFile(filepath.Join(tmpOr(dir), "icon-"+n+".svg"), padIconSVG(v), 0o644)
	}
}

// tmpOr is where the per-state icon SVGs go (SHOT_ICONS, else the scratch dir).
func tmpOr(dir string) string {
	if d := os.Getenv("SHOT_ICONS"); d != "" {
		return d
	}
	return os.TempDir()
}

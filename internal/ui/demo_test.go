//go:build screenshots

package ui

// Renders the README demo as PNG frames, offscreen, with the real UI code:
//
//	DEMO_DIR=/abs/frames go test -tags 'screenshots' -run TestDemo ./internal/ui
//
// `just demo-gif` turns the frames into docs/demo/agentpad-demo.{gif,mp4}.
//
// Fyne's test driver finishes every animation the moment it starts, so the
// demo keeps its own clock: it watches the animations the UI starts (keycap
// press, knob tick, ring pulse, pill pulse, toast slide, caption fade) and
// ticks them itself for every frame. The two timers that run on wall-clock
// time (caption fade-out, toast hide) are cancelled and replayed on the demo
// clock, so frames never depend on how fast the machine renders.

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/jump"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// demoJumper pretends to focus the session's konsole tab and zellij pane.
type demoJumper struct{}

func (demoJumper) Jump(context.Context, events.Session) jump.Report {
	return jump.Report{Steps: []jump.Step{
		{Name: jump.StepTerminalTab, Backend: "konsole", Status: jump.StatusDone},
		{Name: jump.StepMultiplexer, Backend: "zellij", Status: jump.StatusDone},
	}}
}
func (demoJumper) Close() error { return nil }

const (
	demoFPS     = 12
	demoFrame   = time.Second / demoFPS
	captionHold = 1500 * time.Millisecond // tab_profiles.go showCaption
	toastHold   = 3500 * time.Millisecond // shell.go toaster.show
)

// animSlot follows one animation field of the UI; a new pointer means the
// UI started (or replaced) that animation.
type animSlot struct {
	get   func() *fyne.Animation
	cur   *fyne.Animation
	start time.Duration
	done  bool
}

type demoRun struct {
	t     *testing.T
	u     *appUI
	c     software.WindowlessCanvas
	dir   string
	frame int
	now   time.Duration
	slots []*animSlot
	// fadeSlot is the caption fade, which showCaption stops without
	// replacing (the test driver ignores Stop).
	fadeSlot *animSlot
	posted   chan func()

	captionGen int
	captionAt  time.Duration
	captionOn  bool
	toastGen   int
	toastAt    time.Duration
	toastOn    bool
}

func (d *demoRun) watch(get func() *fyne.Animation) *animSlot {
	s := &animSlot{get: get}
	d.slots = append(d.slots, s)
	return s
}

// sync picks up work posted from goroutines, newly started animations and
// the wall-clock timers.
func (d *demoRun) sync() {
	for {
		select {
		case f := <-d.posted:
			f()
			continue
		default:
		}
		break
	}
	p, ts := d.u.profiles, d.u.toasts
	if p.captionGen != d.captionGen { // showCaption ran: cancel its timer
		p.captionGen++
		d.fadeSlot.done = true // showCaption stopped the fade
		d.captionGen, d.captionAt, d.captionOn = p.captionGen, d.now, true
	}
	if d.captionOn && d.now-d.captionAt >= captionHold {
		d.captionOn = false
		// same fade as showCaption's timer
		p.fade = canvas.NewColorRGBAAnimation(colAccent, withAlpha(colAccent, 0), 400*time.Millisecond, func(c color.Color) {
			p.caption.Color = c
			p.caption.Refresh()
		})
		p.fade.Start()
	}
	if ts.gen != d.toastGen { // toaster.show ran: cancel its timer
		ts.gen++
		d.toastGen, d.toastAt, d.toastOn = ts.gen, d.now, true
	}
	if d.toastOn && d.now-d.toastAt >= toastHold {
		d.toastOn = false
		ts.box.Hide()
	}
	for _, s := range d.slots {
		if a := s.get(); a != s.cur {
			s.cur, s.start, s.done = a, d.now, false
		}
	}
}

// tick advances every watched animation to the demo clock.
func (d *demoRun) tick() {
	for _, s := range d.slots {
		a := s.cur
		if a == nil || s.done {
			continue
		}
		el := float32(d.now-s.start) / float32(a.Duration)
		curve := a.Curve
		if curve == nil {
			curve = fyne.AnimationLinear
		}
		switch {
		case a.RepeatCount == fyne.AnimationRepeatForever && a.AutoReverse:
			ph := el - 2*float32(int(el/2))
			if ph > 1 {
				ph = 2 - ph
			}
			a.Tick(curve(ph))
		case el >= 1:
			a.Tick(1)
			s.done = true
		default:
			a.Tick(curve(el))
		}
	}
}

// hold renders frames for dur of demo time.
func (d *demoRun) hold(dur time.Duration) {
	end := d.now + dur
	for d.now < end {
		d.sync()
		d.tick()
		d.capture()
		d.now += demoFrame
	}
}

// relayout lays the whole tree out again, top down. The desktop driver
// re-lays out parents whenever a child's min size changes (a button shown in
// the header, gesture slots added to the inspector); the test canvas does
// not, so the demo does it before every frame.
func relayout(o fyne.CanvasObject) {
	switch x := o.(type) {
	case *fyne.Container:
		if x.Layout != nil {
			x.Layout.Layout(x.Objects, x.Size())
		}
		for _, c := range x.Objects {
			relayout(c)
		}
	case fyne.Widget:
		r := test.WidgetRenderer(x)
		r.Layout(x.Size())
		for _, c := range r.Objects() {
			relayout(c)
		}
	}
}

// paint captures the window. The software painter replaces the clip of a
// scroll container with the clip of any scroller inside it (an entry's text)
// instead of intersecting them, so entry text scrolled up past the screen's
// top would be drawn over the header. When the screen is scrolled, the
// header rows come from a second capture without the screens, which is what
// the desktop driver shows.
func (d *demoRun) paint() image.Image {
	img := d.c.Capture()
	if d.u.profiles.content.(*container.Scroll).Offset.Y == 0 {
		return img
	}
	top := fyne.CurrentApp().Driver().AbsolutePositionForObject(d.u.screens).Y
	d.u.screens.Hide()
	head := d.c.Capture()
	d.u.screens.Show()
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, image.Point{}, draw.Src)
	r := image.Rect(0, 0, img.Bounds().Dx(), int(top*d.c.Scale()))
	draw.Draw(out, r, head, image.Point{}, draw.Src)
	return out
}

func (d *demoRun) capture() {
	for range 2 {
		relayout(d.u.win.Content())
	}
	f, err := os.Create(filepath.Join(d.dir, fmt.Sprintf("frame-%04d.png", d.frame)))
	if err != nil {
		d.t.Fatal(err)
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := errors.Join(enc.Encode(f, d.paint()), f.Close()); err != nil {
		d.t.Fatal(err)
	}
	d.frame++
}

// reveal scrolls the profiles screen over dur, easing, until the bottom of
// o is in view (with a margin), or back to the top when o is nil.
func (d *demoRun) reveal(o fyne.CanvasObject, dur time.Duration) {
	sc := d.u.profiles.content.(*container.Scroll)
	relayout(d.u.win.Content())
	from, to := sc.Offset.Y, float32(0)
	if o != nil {
		drv := fyne.CurrentApp().Driver()
		y := drv.AbsolutePositionForObject(o).Y - drv.AbsolutePositionForObject(sc).Y + sc.Offset.Y
		to = max(0, y+o.Size().Height+48-sc.Size().Height)
	}
	n := max(1, int(dur/demoFrame))
	for i := 1; i <= n; i++ {
		p := fyne.AnimationEaseInOut(float32(i) / float32(n))
		sc.Offset.Y = from + (to-from)*p
		sc.Refresh()
		d.hold(demoFrame)
	}
}

// press sends a live pad press and release like the reader goroutine does.
func (d *demoRun) press(control string, down time.Duration) {
	d.u.onLiveInput(PadInputEvent{Control: control, Down: true, Time: time.Now()})
	d.hold(down)
	d.u.onLiveInput(PadInputEvent{Control: control, Down: false, Time: time.Now()})
}

func TestDemo(t *testing.T) {
	dir := os.Getenv("DEMO_DIR")
	if dir == "" {
		t.Skip("DEMO_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	scale := float32(1.5)
	if s, err := strconv.ParseFloat(os.Getenv("DEMO_SCALE"), 32); err == nil && s > 0 {
		scale = float32(s)
	}

	u := newShotUI(t, demoJumper{})
	d := &demoRun{t: t, u: u, dir: dir, posted: make(chan func(), 64)}
	u.post = func(f func()) { d.posted <- f } // run posted work on the demo loop
	u.visible.Store(true)
	for _, id := range allControls {
		c := u.profiles.view.controls[id]
		d.watch(func() *fyne.Animation { return c.pressAnim })
		d.watch(func() *fyne.Animation { return c.tickAnim })
		d.watch(func() *fyne.Animation { return c.pulseAnim })
	}
	d.watch(func() *fyne.Animation { return u.pillAgents.pulse })
	d.watch(func() *fyne.Animation { return u.toasts.anim })
	d.fadeSlot = d.watch(func() *fyne.Animation { return u.profiles.fade })

	claude := func(kind events.Kind, tool, msg string) events.Event {
		return events.Event{
			Time: time.Now(), Agent: "claude", SessionID: "3f9c1d2e-aa41", Kind: kind, Tool: tool,
			Cwd: "/home/me/projects/api", Message: msg,
			Origin: &events.Origin{KonsoleSession: "/Sessions/3", ZellijSession: "api", ZellijPane: "2"},
		}
	}
	u.onEvent(events.Event{Time: time.Now().Add(-90 * time.Second), Agent: "codex", SessionID: "019a6c2e-77",
		Kind: events.KindIdle, Tool: "apply_patch", Cwd: "/home/me/projects/web"})
	u.onEvent(claude(events.KindPrompt, "", ""))
	u.onEvent(claude(events.KindToolDone, "Edit", ""))

	const w, h = 1200, 760
	d.c = u.win.Canvas().(software.WindowlessCanvas)
	d.c.SetScale(scale)
	u.navigate(screenSessions)
	for range 3 { // let size-dependent layouts settle
		u.win.Resize(fyne.NewSize(w, h))
		u.win.Content().Refresh()
		d.c.Capture()
	}

	// 1. sessions: Claude busy, Codex idle
	d.hold(1500 * time.Millisecond)

	// 2. Claude asks for permission to run Bash: pill pulses, card wants you
	u.onEvent(claude(events.KindToolPending, "Bash", "Claude needs your permission to use Bash"))
	d.hold(1800 * time.Millisecond)
	u.jumpNext() // the header Jump button (or a long press on the knob)
	time.Sleep(50 * time.Millisecond)
	d.hold(1200 * time.Millisecond)

	// 3. profiles: key 1 answers "yes", Claude gets back to work
	u.navigate(screenProfiles)
	d.hold(700 * time.Millisecond)
	d.press(InputKey1, 250*time.Millisecond)
	d.hold(350 * time.Millisecond)
	u.onEvent(claude(events.KindToolDone, "Bash", ""))
	d.hold(1000 * time.Millisecond)

	// 4. knob: two detents clockwise, one back
	d.press(InputKnobCW, 90*time.Millisecond)
	d.hold(330 * time.Millisecond)
	d.press(InputKnobCW, 90*time.Millisecond)
	d.hold(330 * time.Millisecond)
	d.press(InputKnobCCW, 90*time.Millisecond)
	d.hold(900 * time.Millisecond)

	// 5. gesture mode: key 3 hold sends ctrl-c
	u.modeChanged(ModeGesture)
	u.pad.showMode(ModeGesture, liveStatus{Mode: ModeGesture, Reading: true, Backend: "uinput"})
	u.profiles.selectControl(ctlKey3)
	d.hold(200 * time.Millisecond)
	d.reveal(u.profiles.monitorBox, 500*time.Millisecond)
	d.press(InputKey3, 600*time.Millisecond)
	u.onGesture(gesture.Event{Gesture: gesture.Gesture{Control: gesture.Key3, Kind: gesture.Hold}, Action: "ctrl-c", Time: time.Now()})
	d.hold(1300 * time.Millisecond)

	// 6. rotate the drawing clockwise, then back
	u.modeChanged(ModePlain)
	u.pad.showMode(ModePlain, liveStatus{Mode: ModePlain})
	u.profiles.selectControl(ctlKey1)
	u.profiles.rotate(profile.OrientClockwise)
	d.reveal(u.profiles.orientLb, 400*time.Millisecond)
	d.hold(1000 * time.Millisecond)
	u.profiles.rotate(profile.OrientNormal)
	d.reveal(nil, 400*time.Millisecond)

	// 7. back to the plain profile screen for a clean loop
	d.hold(1000 * time.Millisecond)
	t.Logf("%d frames at %d fps (%.1fs) in %s", d.frame, demoFPS, float64(d.frame)/demoFPS, dir)
}

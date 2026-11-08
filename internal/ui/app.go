// Package ui is the Fyne desktop UI of agentpad.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/jump"
	"github.com/thereisnotime/agentpad/internal/notify"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/vkbd"
)

const (
	prefLEDFollow     = "led.follow"
	prefAttentionMode = "led.attentionMode" // pre-v2 8890 attention mode, migrated once
	prefOrientation   = "orientation"
	prefActiveProfile = "profile.active"
	prefMode          = "mode"
	prefNotify        = "notify.attention"
	// gesture thresholds in milliseconds (0 or unset = default)
	prefGestureHold      = "gesture.holdMs"
	prefGestureLongHold  = "gesture.longholdMs"
	prefGestureDoubleTap = "gesture.doubletapMs"
	pollInterval         = 2 * time.Second
	sessionTTL           = 30 * time.Minute
	logRows              = 200
	deviceQueue          = 16
	liveQueue            = 64
	uploadTimeout        = 20 * time.Second
	jumpTimeout          = 5 * time.Second
)

// Screen indexes in navigation order.
const (
	screenPad = iota
	screenProfiles
	screenSessions
	screenLog
	screenSetup
	screenAbout
)

// Deps are the collaborators the UI needs. Zero values get sensible defaults.
type Deps struct {
	Writer  *pad.ToolWriter // device backend; nil means pad.NewToolWriter()
	Store   profile.Store   // nil means a file store under config.ConfigDir()
	Bin     string          // absolute agentpad path used in setup snippets
	Version string
	LogPath string // event log; empty means <StateDir>/events.jsonl
	Addr    string // event receiver address; empty means config.ListenAddr
	// Input is the live pad input used for on-screen feedback; nil disables it.
	Input PadInput
	// Settings stores user preferences; nil means Fyne preferences.
	Settings Settings
	// Live are the devices for live input and gesture mode; zero fields
	// mean the real evdev / uinput / ydotool. Ignored parts: when Input is
	// set, plain-mode live input comes from Input instead.
	Live LiveDevices
	// Jumper focuses waiting sessions; nil means jump.New().
	Jumper JumpBackend
	// Notifier connects the attention notifications; nil means notify.New.
	// It is called lazily, off the UI goroutine.
	Notifier func(onJump func(key string)) (NotifyBackend, error)
}

type appUI struct {
	app   fyne.App
	win   fyne.Window
	deps  Deps
	prefs Settings

	writer  *deviceWriter
	led     *pad.LEDController
	dev     *deviceWorker
	tracker *events.Tracker
	log     *events.Log
	srv     *events.Server
	recvErr error

	// Mirrors of preferences and device state, read from non-UI goroutines.
	ledFollow  atomic.Bool
	visible    atomic.Bool // window shown (not hidden to the tray)
	padPresent atomic.Bool
	policy     atomic.Pointer[LEDPolicy]
	notifyOn   atomic.Bool
	started    atomic.Bool // start ran: live input may open the pad
	// post runs f on the UI goroutine (fyne.Do; tests queue instead).
	post func(f func())
	// openFile opens a file in the user's editor (xdg-open; tests fake it).
	openFile func(path string) error

	live     *liveController
	liveJobs chan func()
	liveQuit chan struct{}
	liveDone chan struct{}
	notifier *attentionNotifier
	jumpMu   sync.Mutex
	lastJump string // key of the session jumped to last

	stop     chan struct{}
	stopOnce sync.Once

	// UI goroutine only.
	device     pad.Device
	deviceErr  error
	pillPad    *pill
	pillAgents *pill
	pillRecv   *pill
	toasts     *toaster
	nav        [][2]*navItem
	screens    *fyne.Container
	current    int
	jumpBtn    *widget.Button
	mode       Mode // applied input mode
	modeBusy   bool // a mode switch is running

	pad      *padTab
	profiles *profilesTab
	sessions *sessionsTab
	logView  *logTab
	tray     *tray
}

// Run builds the main window on a and blocks until the app quits.
func Run(a fyne.App, d Deps) {
	u := newAppUI(a, d)
	a.Lifecycle().SetOnStarted(u.start)
	u.win.ShowAndRun()
	u.shutdown()
}

func newAppUI(a fyne.App, d Deps) *appUI {
	if d.Writer == nil {
		d.Writer = pad.NewToolWriter()
	}
	if d.Store == nil {
		d.Store = profile.NewFileStore(filepath.Join(config.ConfigDir(), "profiles"))
	}
	if d.LogPath == "" {
		d.LogPath = filepath.Join(config.StateDir(), "events.jsonl")
	}
	if d.Addr == "" {
		d.Addr = config.ListenAddr
	}
	if d.Version == "" {
		d.Version = "dev"
	}
	a.Settings().SetTheme(appTheme{})
	u := &appUI{app: a, deps: d, stop: make(chan struct{}), deviceErr: pad.ErrNotConnected}
	if d.Settings == nil {
		d.Settings = newFynePrefs(a.Preferences())
	}
	u.deps.Settings = d.Settings
	u.prefs = d.Settings
	u.ledFollow.Store(u.prefs.LEDFollow())
	u.notifyOn.Store(u.prefs.NotifyAttention())
	u.post = fyne.Do
	u.openFile = xdgOpen
	u.mode = ModePlain
	if d.Jumper == nil {
		d.Jumper = jump.New()
	}
	u.deps.Jumper = d.Jumper
	if d.Notifier == nil {
		d.Notifier = func(onJump func(string)) (NotifyBackend, error) {
			n, err := notify.New(onJump)
			if err != nil {
				return nil, err
			}
			return n, nil
		}
	}
	u.deps.Notifier = d.Notifier

	u.writer = newDeviceWriter(d.Writer)
	u.led = pad.NewLEDController(u.writer, pollInterval)
	u.dev = newDeviceWorker(u.writer, u.led, deviceQueue)
	u.dev.start()
	u.tracker = events.NewTracker()
	live := d.Live
	if d.Input != nil {
		live.Disabled = true // the given input replaces the observer
	}
	u.live = newLiveController(live, u.uploadSync, liveCallbacks{
		Input:    u.onLiveInput,
		Gesture:  u.onGesture,
		Status:   func(st liveStatus) { u.post(func() { u.showLiveStatus(st) }) },
		Agentpad: func(a string) { u.post(func() { u.onAgentpadAction(a) }) },
		Unexpected: func(code uint16) {
			u.post(func() { u.profiles.showMismatch(vkbd.KeyName(code)) })
		},
	})
	u.liveJobs = make(chan func(), liveQueue)
	u.liveQuit = make(chan struct{})
	u.liveDone = make(chan struct{})
	go u.runLive()
	u.loadLEDPolicy(u.writer.product())

	u.win = a.NewWindow("agentpad")
	u.win.Resize(fyne.NewSize(1100, 720))
	// Dev only: AGENTPAD_WINDOW=500x800 opens the window at that size
	// (used for screenshots, since resizing isn't scriptable on Wayland).
	if w, h, ok := parseWindowSize(os.Getenv("AGENTPAD_WINDOW")); ok {
		u.win.Resize(fyne.NewSize(w, h))
	}
	u.win.SetMaster()

	u.toasts = newToaster()
	u.pillPad = newPill(func() { u.navigate(screenPad) })
	u.pillAgents = newPill(func() { u.navigate(screenSessions) })
	u.pillRecv = newPill(u.explainReceiver)
	u.pillRecv.set(colorIdle, "Receiver starting")
	u.jumpBtn = widget.NewButtonWithIcon("Jump", theme.NavigateNextIcon(), u.jumpNext)
	u.jumpBtn.Importance = widget.WarningImportance
	u.jumpBtn.Hide()

	u.pad = newPadTab(u)
	u.profiles = newProfilesTab(u)
	u.sessions = newSessionsTab(u)
	u.logView = newLogTab(u)

	u.win.SetContent(u.buildShell([]navScreen{
		{"Pad", theme.ComputerIcon(), u.pad.content},
		{"Profiles", theme.GridIcon(), u.profiles.content},
		{"Sessions", theme.ListIcon(), u.sessions.content},
		{"Log", theme.HistoryIcon(), u.logView.content},
		{"Setup", theme.SettingsIcon(), newSetupTab(u)},
		{"About", theme.InfoIcon(), newAboutTab(u)},
	}))

	// Dev only: AGENTPAD_SCREEN=pad|profiles|sessions|log|setup|about picks the first screen.
	if i := screenIndex(os.Getenv("AGENTPAD_SCREEN")); i >= 0 {
		u.navigate(i)
	}

	u.prefs.OnChange(u.settingsChanged)

	u.tray = newTray(u)
	u.showDevice(pad.Device{}, pad.ErrNotConnected)
	u.aggregateChanged()
	return u
}

// start runs on the UI goroutine once the app is up.
func (u *appUI) start() {
	log, err := events.OpenLog(u.deps.LogPath)
	if err != nil {
		u.toasts.show(toastError, fmt.Sprintf("Event log unavailable: %v", err))
	} else {
		u.log = log
		if past, err := log.Tail(logRows); err == nil {
			u.logView.load(past)
		}
	}

	u.notifier = newAttentionNotifier(func() (NotifyBackend, error) {
		return u.deps.Notifier(u.jumpToKey)
	}, u.notifyOn.Load)
	go u.notifier.run(u.stop)

	u.srv = events.NewServer(u.deps.Addr, u.onEvent)
	if err := u.srv.Start(); err != nil {
		u.srv = nil
		u.recvErr = err
		u.pillRecv.set(colorIdle, "Receiver off")
	} else {
		u.pillRecv.set(colorSuccess, "Receiver live")
	}

	u.visible.Store(true)
	u.pad.start()
	go u.loop()
	if u.deps.Input != nil {
		go u.readInput(u.deps.Input)
	}
	u.started.Store(true)
	u.setLiveProfile(u.activeProfile())
	if u.prefs.Mode() == ModeGesture {
		u.setMode(ModeGesture)
	}
}

// readInput forwards live pad input to the editor while the window is visible.
func (u *appUI) readInput(in PadInput) {
	for {
		select {
		case <-u.stop:
			return
		case ev, ok := <-in.Events():
			if !ok {
				return
			}
			if !u.visible.Load() {
				continue
			}
			fyne.Do(func() { u.profiles.feedback(ev) })
		}
	}
}

// explainReceiver tells the user why live events are off. UI goroutine.
func (u *appUI) explainReceiver() {
	if u.recvErr == nil {
		u.toasts.show(toastSuccess, "Receiving agent events on "+u.deps.Addr+".")
		return
	}
	dialog.ShowInformation("Live events are off",
		fmt.Sprintf("Could not listen on %s, so agent events won't show up here. "+
			"Is another agentpad already running?\n\n%v", u.deps.Addr, u.recvErr), u.win)
}

// loop drives periodic refreshes until shutdown. It never touches USB: the
// detection reads sysfs and LED changes only enqueue work for the device worker.
func (u *appUI) loop() {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-u.stop:
			return
		case now := <-t.C:
			u.detect()
			changed := u.tracker.Prune(sessionTTL, now)
			if changed {
				u.followLED()
				u.notifyWaiting()
			}
			if st := u.live.Status(); st.Mode == ModePlain && !st.Reading && !u.deps.Live.Disabled {
				u.tryLive(func() { _ = u.live.EnsureObserver() })
			}
			fyne.Do(func() {
				u.sessions.refresh()
				u.profiles.refreshMonitor(now)
				if changed {
					u.aggregateChanged()
				}
			})
		}
	}
}

// detect looks for the pad (sysfs only) and updates the views. Any goroutine.
func (u *appUI) detect() {
	dev, err := pad.Detect()
	present := err == nil
	u.padPresent.Store(present)
	if present {
		u.writer.setProduct(dev.ProductID)
	}
	fyne.Do(func() { u.showDevice(dev, err) })
}

// showDevice reflects detection results everywhere. UI goroutine.
func (u *appUI) showDevice(d pad.Device, err error) {
	prevPID := u.device.ProductID
	u.device, u.deviceErr = d, err
	if err != nil {
		u.pillPad.set(colorIdle, "No pad")
	} else {
		u.pillPad.set(colorSuccess, fmt.Sprintf("Pad %04x", d.ProductID))
		if d.ProductID != prevPID && ledFamily(d.ProductID) != ledFamily(u.ledProduct(prevPID)) {
			u.loadLEDPolicy(d.ProductID)
		}
	}
	u.pad.showDevice(d, err)
}

// ledProduct is the product whose LED caps apply (8890 when nothing detected).
func (u *appUI) ledProduct(pid uint16) uint16 {
	if pid == 0 {
		return 0x8890
	}
	return pid
}

// onEvent is called by the receiver for every event, on its goroutine. It
// only records the event and enqueues work; it never waits for the pad.
func (u *appUI) onEvent(e events.Event) {
	if u.log != nil {
		_ = u.log.Append(e)
	}
	changed := u.tracker.Apply(e)
	if changed {
		u.followLED()
	}
	u.notifyWaiting()
	fyne.Do(func() {
		u.logView.add(e)
		u.sessions.refresh()
		u.aggregateChanged() // also the waiting count, which can change on its own
	})
}

// followLED requests the LED setting for the current aggregate state if
// enabled. Non-blocking; safe to call from any goroutine.
func (u *appUI) followLED() {
	if !u.ledFollow.Load() || !u.padPresent.Load() {
		return
	}
	p := u.policy.Load()
	if p == nil {
		return
	}
	u.dev.Follow(ledPolicy(u.tracker.Aggregate(), *p), func(err error) {
		if err != nil {
			fyne.Do(func() { u.pad.setStatus("LED: " + writeErrMessage(err)) })
		}
	})
}

// loadLEDPolicy reads the agent LED policy for pid's model from settings.
func (u *appUI) loadLEDPolicy(pid uint16) {
	caps := pad.CapsFor(pid)
	p, _ := u.prefs.LEDPolicy(pid)
	if pad.ValidateLED(caps, p.Attention) != nil || pad.ValidateLED(caps, p.Otherwise) != nil {
		p = defaultLEDPolicy(pid)
	}
	u.policy.Store(&p)
}

// setLEDPolicy persists p for the current model and re-applies it. UI goroutine.
func (u *appUI) setLEDPolicy(p LEDPolicy) {
	u.policy.Store(&p)
	u.prefs.SetLEDPolicy(u.writer.product(), p)
	u.led.Forget()
	u.followLED()
}

// settingsChanged picks up changes made to the settings store (also from
// outside the app) without rebuilding what the user is editing. UI goroutine.
func (u *appUI) settingsChanged() {
	if on := u.prefs.LEDFollow(); on != u.ledFollow.Load() {
		u.ledFollow.Store(on)
		u.pad.led.follow.SetChecked(on)
	}
	before := u.policy.Load()
	u.loadLEDPolicy(u.writer.product())
	if after := u.policy.Load(); before == nil || *before != *after {
		u.pad.reloadLED()
	} else {
		u.pad.led.refreshLabels()
	}
	if o := u.prefs.Orientation(); o != u.profiles.orient {
		u.profiles.rotate(o)
	}
	u.tray.rebuild()
}

// applyProfile writes p (as layer 1, plus the next two profiles) to the pad
// through the device worker. done runs on the UI goroutine. UI goroutine.
func (u *appUI) applyProfile(p profile.Profile, all []profile.Profile, done func(names []string, err error)) {
	if err := profile.Validate(p); err != nil {
		done(nil, fmt.Errorf("fix this first: %w", err))
		return
	}
	layers := profile.Layers(replaceProfile(all, p), p.Name)
	doc, err := profile.MappingYAMLOriented(layers, u.profiles.orient)
	if err != nil {
		done(nil, fmt.Errorf("could not build the mapping: %w", err))
		return
	}
	names := make([]string, len(layers))
	for i, l := range layers {
		names[i] = fmt.Sprintf("%d = %s", i+1, l.Name)
	}
	if u.mode == ModeGesture || u.modeBusy {
		// the pad stays on the gesture mapping; the engine switches profiles
		u.prefs.SetActiveProfile(p.Name)
		u.tray.rebuild()
		u.setLiveProfile(p)
		done(nil, nil)
		return
	}
	u.dev.Upload(doc, func(err error) {
		fyne.Do(func() {
			if err == nil {
				u.prefs.SetActiveProfile(p.Name)
				u.tray.rebuild()
				u.setLiveProfile(p)
				u.profiles.clearMismatch()
			}
			done(names, err)
		})
	})
}

// activateProfile makes the stored profile name active and applies it (tray). UI goroutine.
func (u *appUI) activateProfile(name string) {
	p, err := u.deps.Store.Get(name)
	if err != nil {
		u.toasts.show(toastError, "Profile "+name+": "+err.Error())
		return
	}
	u.profiles.selectByName(name)
	u.toasts.show(toastInfo, "Applying "+name+"…")
	u.applyProfile(p, u.profiles.profiles, func(_ []string, err error) {
		if err != nil {
			u.toasts.show(toastError, "Could not apply "+name+": "+writeErrMessage(err))
			return
		}
		u.toasts.show(toastSuccess, "Active profile: "+name+".")
	})
}

// setLEDFollow persists the "LED follows agents" toggle. UI goroutine.
func (u *appUI) setLEDFollow(on bool) {
	u.ledFollow.Store(on)
	u.prefs.SetLEDFollow(on)
	u.tray.rebuild()
	if on {
		u.led.Forget()
		u.followLED()
	}
}

// aggregateChanged updates every view that shows the aggregate state. UI goroutine only.
func (u *appUI) aggregateChanged() {
	s := u.tracker.Aggregate()
	u.sessions.showAggregate(s)
	u.tray.show(s, len(u.tracker.Waiting()))
	u.showAgentsPill(s)
}

func (u *appUI) showAgentsPill(s events.State) {
	var n, busy, att int
	for _, x := range u.tracker.Sessions() {
		n++
		switch x.State {
		case events.StateBusy:
			busy++
		case events.StateAttention:
			att++
		}
	}
	u.pillAgents.setPulsing(s == events.StateAttention)
	if s == events.StateAttention {
		u.jumpBtn.Show()
	} else {
		u.jumpBtn.Hide()
	}
	switch s {
	case events.StateAttention:
		u.pillAgents.set(colorAttention, fmt.Sprintf("%d needs you", att))
	case events.StateBusy:
		u.pillAgents.set(colorBusy, fmt.Sprintf("%d busy", busy))
	default:
		if n == 0 {
			u.pillAgents.set(colorIdle, "Agents idle")
		} else {
			u.pillAgents.set(colorIdle, fmt.Sprintf("%s idle", plural(n, "agent")))
		}
	}
}

func (u *appUI) shutdown() {
	u.stopOnce.Do(func() {
		close(u.stop)
		if u.srv != nil {
			_ = u.srv.Close()
		}
		// Finish the running live job, then leave gesture mode for good: the
		// plain mapping goes back on the pad before the device worker stops.
		close(u.liveQuit)
		<-u.liveDone
		if err := u.live.Close(u.plainDoc()); err != nil {
			fmt.Fprintln(os.Stderr, "agentpad: could not restore the plain mapping:", err)
		}
		u.dev.Close()
		u.led.Close()
		if u.log != nil {
			_ = u.log.Close()
		}
		if u.notifier != nil {
			select {
			case <-u.notifier.done:
			case <-time.After(time.Second):
			}
		}
		_ = u.deps.Jumper.Close()
	})
}

// quit is used by the tray menu.
func (u *appUI) quit() {
	u.shutdown()
	u.app.Quit()
}

// runLive runs live-controller jobs one at a time, in order.
func (u *appUI) runLive() {
	defer close(u.liveDone)
	for {
		select {
		case <-u.liveQuit:
			return
		case f := <-u.liveJobs:
			f()
		}
	}
}

// liveDo queues f for the live goroutine. Any goroutine.
func (u *appUI) liveDo(f func()) {
	select {
	case u.liveJobs <- f:
	case <-u.liveQuit:
	}
}

// tryLive queues f unless the queue is full (periodic retries). Any goroutine.
func (u *appUI) tryLive(f func()) {
	select {
	case u.liveJobs <- f:
	default:
	}
}

// uploadSync uploads a mapping through the device worker and waits for it.
// Never call it on the UI goroutine.
func (u *appUI) uploadSync(doc []byte) error {
	ch := make(chan error, 1)
	u.dev.Upload(doc, func(err error) { ch <- err })
	select {
	case err := <-ch:
		return err
	case <-time.After(uploadTimeout):
		return errors.New("the pad did not answer in time")
	}
}

// activeProfile is the profile on the pad: the last applied one, else the
// first profile. UI goroutine (or after the UI stopped).
func (u *appUI) activeProfile() profile.Profile {
	name := u.prefs.ActiveProfile()
	for _, p := range u.profiles.profiles {
		if p.Name == name {
			return p
		}
	}
	if len(u.profiles.profiles) > 0 {
		return u.profiles.profiles[0]
	}
	return profile.Builtins()[0]
}

// plainDoc renders the plain mapping of the active profile (plus the next
// two as layers 2 and 3), falling back to the built-ins.
func (u *appUI) plainDoc() []byte {
	active := u.activeProfile()
	all := u.profiles.profiles
	if len(all) == 0 {
		all = profile.Builtins()
	}
	if doc, err := profile.MappingYAMLOriented(profile.Layers(replaceProfile(all, active), active.Name), u.profiles.orient); err == nil {
		return doc
	}
	doc, _ := profile.MappingYAMLOriented(profile.Layers(profile.Builtins(), ""), profile.OrientNormal)
	return doc
}

// setLiveProfile hands p and the orientation to live input / the gesture
// engine. UI goroutine.
func (u *appUI) setLiveProfile(p profile.Profile) {
	o := u.profiles.orient
	u.liveDo(func() {
		_, err := u.live.SetProfile(p, o)
		if u.started.Load() {
			_ = u.live.EnsureObserver()
		}
		if err != nil {
			u.post(func() { u.toasts.show(toastError, "Gesture mode: "+liveErrMessage(err)) })
		}
	})
}

// setMode switches between plain and gesture mode. UI goroutine.
func (u *appUI) setMode(m Mode) {
	if m == u.mode || u.modeBusy {
		u.pad.showMode(u.mode, u.live.Status())
		return
	}
	u.modeBusy = true
	u.pad.modeSwitching(m)
	if m == ModeGesture {
		cfg := u.prefs.GestureConfig().config()
		p, o := u.activeProfile(), u.profiles.orient
		u.liveDo(func() {
			_, _ = u.live.SetProfile(p, o)
			backend, err := u.live.EnterGesture(cfg)
			u.post(func() {
				u.modeBusy = false
				if err != nil {
					u.prefs.SetMode(ModePlain)
					u.modeChanged(ModePlain)
					u.pad.showModeError(err)
					u.toasts.show(toastError, "Gesture mode is off: "+liveErrMessage(err))
					return
				}
				u.prefs.SetMode(ModeGesture)
				u.modeChanged(ModeGesture)
				u.toasts.show(toastSuccess, "Gesture mode on, typing through "+backend+".")
			})
		})
		return
	}
	doc := u.plainDoc()
	u.liveDo(func() {
		err := u.live.EnterPlain(doc)
		u.post(func() {
			u.modeBusy = false
			u.prefs.SetMode(ModePlain)
			u.modeChanged(ModePlain)
			if err != nil {
				u.pad.showModeError(err)
				u.toasts.show(toastError, "Plain mode, but the plain mapping was not written: "+liveErrMessage(err))
				return
			}
			u.toasts.show(toastSuccess, "Plain mode: the pad works on its own again.")
		})
	})
}

// modeChanged reflects the applied mode everywhere. UI goroutine.
func (u *appUI) modeChanged(m Mode) {
	u.mode = m
	u.pad.showMode(m, u.live.Status())
	u.profiles.setMode(m)
	u.tray.rebuild()
}

// showLiveStatus reflects live input / gesture engine status. UI goroutine.
func (u *appUI) showLiveStatus(st liveStatus) {
	u.profiles.showLive(st)
	if !u.modeBusy {
		u.pad.showMode(u.mode, st)
	}
}

// onLiveInput animates the pad drawing. Reader goroutine.
func (u *appUI) onLiveInput(ev PadInputEvent) {
	if !u.visible.Load() {
		return
	}
	u.post(func() { u.profiles.feedback(ev) })
}

// onGesture adds a recognised gesture to the monitor. Engine goroutine.
func (u *appUI) onGesture(ev gesture.Event) {
	it := gestureLine(ev)
	u.post(func() { u.profiles.monitorAdd(it) })
}

// onAgentpadAction runs a reserved action from gesture mode. UI goroutine.
func (u *appUI) onAgentpadAction(a string) {
	switch a {
	case profile.ActionJump:
		u.jumpNext()
	case profile.ActionNextProfile:
		u.nextProfile()
	}
}

// nextProfile activates the profile after the active one. UI goroutine.
func (u *appUI) nextProfile() {
	list := u.profiles.profiles
	if len(list) == 0 {
		return
	}
	cur := u.activeProfile().Name
	for i, p := range list {
		if p.Name == cur {
			u.activateProfile(list[(i+1)%len(list)].Name)
			return
		}
	}
	u.activateProfile(list[0].Name)
}

// notifyWaiting hands the waiting sessions to the notifier. Any goroutine.
func (u *appUI) notifyWaiting() {
	if u.notifier != nil {
		u.notifier.update(u.tracker.Waiting())
	}
}

// setNotifyAttention persists the notifications toggle. UI goroutine.
func (u *appUI) setNotifyAttention(on bool) {
	u.notifyOn.Store(on)
	u.prefs.SetNotifyAttention(on)
	u.notifyWaiting()
}

// jumpNext jumps to the next waiting session, cycling oldest first. UI goroutine.
func (u *appUI) jumpNext() {
	u.jumpMu.Lock()
	last := u.lastJump
	u.jumpMu.Unlock()
	s, ok := u.tracker.NextWaiting(last)
	if !ok {
		u.toasts.show(toastInfo, "No agent is waiting for you.")
		return
	}
	u.jumpSession(s)
}

// jumpToKey jumps to the session with key (notification button). Any goroutine.
func (u *appUI) jumpToKey(key string) {
	for _, s := range u.tracker.Sessions() {
		if s.Key() == key {
			u.jumpSession(s)
			return
		}
	}
}

// jumpSession focuses s off the UI goroutine and toasts the result. Any goroutine.
func (u *appUI) jumpSession(s events.Session) {
	u.jumpMu.Lock()
	u.lastJump = s.Key()
	u.jumpMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), jumpTimeout)
		defer cancel()
		rep := u.deps.Jumper.Jump(ctx, s)
		kind, msg := jumpToast(s, rep)
		u.post(func() { u.toasts.show(kind, msg) })
	}()
}

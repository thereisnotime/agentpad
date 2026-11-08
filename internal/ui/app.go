// Package ui is the Fyne desktop UI of agentpad.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

const (
	prefLEDFollow     = "led.follow"
	prefAttentionMode = "led.attentionMode" // pre-v2 8890 attention mode, migrated once
	prefOrientation   = "orientation"
	pollInterval      = 2 * time.Second
	sessionTTL        = 30 * time.Minute
	logRows           = 200
	deviceQueue       = 16
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

	u.writer = newDeviceWriter(d.Writer)
	u.led = pad.NewLEDController(u.writer, pollInterval)
	u.dev = newDeviceWorker(u.writer, u.led, deviceQueue)
	u.dev.start()
	u.tracker = events.NewTracker()
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
			}
			fyne.Do(func() {
				u.sessions.refresh()
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
	fyne.Do(func() {
		u.logView.add(e)
		u.sessions.refresh()
		if changed {
			u.aggregateChanged()
		}
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
}

// setLEDFollow persists the "LED follows agents" toggle. UI goroutine.
func (u *appUI) setLEDFollow(on bool) {
	u.ledFollow.Store(on)
	u.prefs.SetLEDFollow(on)
	if on {
		u.led.Forget()
		u.followLED()
	}
}

// aggregateChanged updates every view that shows the aggregate state. UI goroutine only.
func (u *appUI) aggregateChanged() {
	s := u.tracker.Aggregate()
	u.sessions.showAggregate(s)
	u.tray.show(s)
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
		u.dev.Close()
		u.led.Close()
		if u.log != nil {
			_ = u.log.Close()
		}
	})
}

// quit is used by the tray menu.
func (u *appUI) quit() {
	u.shutdown()
	u.app.Quit()
}

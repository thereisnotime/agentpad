// Package ui is the Fyne desktop UI of agentpad.
package ui

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

const (
	prefLEDFollow     = "led.follow"
	prefAttentionMode = "led.attentionMode"
	pollInterval      = 2 * time.Second
	sessionTTL        = 30 * time.Minute
	logRows           = 200
)

// Deps are the collaborators the UI needs. Zero values get sensible defaults.
type Deps struct {
	Writer  *pad.ToolWriter // device backend; nil means pad.NewToolWriter()
	Store   profile.Store   // nil means a file store under config.ConfigDir()
	Bin     string          // absolute agentpad path used in setup snippets
	Version string
	LogPath string // event log; empty means <StateDir>/events.jsonl
	Addr    string // event receiver address; empty means config.ListenAddr
}

type appUI struct {
	app  fyne.App
	win  fyne.Window
	deps Deps

	writer  *deviceWriter
	led     *pad.LEDController
	tracker *events.Tracker
	log     *events.Log
	srv     *events.Server

	// Mirrors of preferences and device state, read from non-UI goroutines.
	ledFollow     atomic.Bool
	attentionMode atomic.Int32
	padPresent    atomic.Bool

	stop     chan struct{}
	stopOnce sync.Once

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
	u := &appUI{app: a, deps: d, stop: make(chan struct{})}
	u.ledFollow.Store(a.Preferences().BoolWithFallback(prefLEDFollow, true))
	u.attentionMode.Store(int32(a.Preferences().IntWithFallback(prefAttentionMode, 1)))

	u.writer = newDeviceWriter(d.Writer)
	u.led = pad.NewLEDController(u.writer, pollInterval)
	u.tracker = events.NewTracker()

	u.win = a.NewWindow("agentpad")
	u.win.Resize(fyne.NewSize(900, 620))
	u.win.SetMaster()

	u.pad = newPadTab(u)
	u.profiles = newProfilesTab(u)
	u.sessions = newSessionsTab(u)
	u.logView = newLogTab(u)

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Pad", theme.ComputerIcon(), u.pad.content),
		container.NewTabItemWithIcon("Profiles", theme.GridIcon(), u.profiles.content),
		container.NewTabItemWithIcon("Sessions", theme.ListIcon(), u.sessions.content),
		container.NewTabItemWithIcon("Log", theme.HistoryIcon(), u.logView.content),
		container.NewTabItemWithIcon("Setup", theme.SettingsIcon(), newSetupTab(u)),
	)
	u.win.SetContent(tabs)

	u.tray = newTray(u)
	return u
}

// start runs on the UI goroutine once the app is up.
func (u *appUI) start() {
	log, err := events.OpenLog(u.deps.LogPath)
	if err != nil {
		dialog.ShowError(fmt.Errorf("event log unavailable: %w", err), u.win)
	} else {
		u.log = log
		if past, err := log.Tail(logRows); err == nil {
			u.logView.load(past)
		}
	}

	u.srv = events.NewServer(u.deps.Addr, u.onEvent)
	if err := u.srv.Start(); err != nil {
		u.srv = nil
		dialog.ShowInformation("Live events disabled",
			fmt.Sprintf("Could not start the event receiver on %s. Is agentpad already running?\n\n%v", u.deps.Addr, err), u.win)
	}

	u.aggregateChanged()
	u.pad.start()
	go u.loop()
}

// loop drives periodic refreshes until shutdown.
func (u *appUI) loop() {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-u.stop:
			return
		case now := <-t.C:
			dev, err := pad.Detect()
			present := err == nil
			u.padPresent.Store(present)
			if present {
				u.writer.setProduct(dev.ProductID)
			}
			changed := u.tracker.Prune(sessionTTL, now)
			if changed {
				u.followLED()
			}
			fyne.Do(func() {
				u.pad.showDevice(dev, err)
				u.sessions.refresh()
				if changed {
					u.aggregateChanged()
				}
			})
		}
	}
}

// onEvent is called by the receiver for every event, on its goroutine.
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

// followLED requests the LED mode for the current aggregate state if enabled.
// Safe to call from any goroutine.
func (u *appUI) followLED() {
	if !u.ledFollow.Load() {
		return
	}
	if _, err := pad.Detect(); err != nil {
		return
	}
	mode := ledPolicy(u.tracker.Aggregate(), int(u.attentionMode.Load()))
	if err := u.led.Set(pad.LEDSetting{Mode: mode}); err != nil {
		fyne.Do(func() { u.pad.setStatus("LED: " + writeErrMessage(err)) })
	}
}

// aggregateChanged updates every view that shows the aggregate state. UI goroutine only.
func (u *appUI) aggregateChanged() {
	s := u.tracker.Aggregate()
	u.sessions.showAggregate(s)
	u.tray.show(s)
}

// setLEDFollow and setAttentionMode persist the LED preferences. UI goroutine only.
func (u *appUI) setLEDFollow(on bool) {
	u.ledFollow.Store(on)
	u.app.Preferences().SetBool(prefLEDFollow, on)
	if on {
		go u.followLED()
	}
}

func (u *appUI) setAttentionMode(n int) {
	u.attentionMode.Store(int32(n))
	u.app.Preferences().SetInt(prefAttentionMode, n)
	go u.followLED()
}

func (u *appUI) shutdown() {
	u.stopOnce.Do(func() {
		close(u.stop)
		if u.srv != nil {
			_ = u.srv.Close()
		}
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

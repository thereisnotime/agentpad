package ui

// User scripts and event hooks: the runner, the hook dispatcher, pad
// triggers and Run now.

import (
	"context"
	"fmt"
	"strings"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/scripts"
	"github.com/thereisnotime/agentpad/internal/settings"
)

// initScripts creates the runner and hook dispatcher and loads the scripts
// from settings.yaml. Called once from newAppUI, before the live controller.
func (u *appUI) initScripts() {
	u.runNow = map[scripts.RunID]bool{}
	u.runner = scripts.NewRunner(scripts.Options{LogPath: u.deps.ScriptsLog})
	_ = u.runner.Load()
	u.hooks = scripts.NewHooks(func(name string, p scripts.Payload) {
		u.runner.Run(context.Background(), name, p)
	})
	if u.files != nil {
		files := u.files
		profile.SetScriptResolver(func(name string) bool { return files.Get().HasScript(name) })
	}
	u.loadScripts()
	u.runner.OnRun(func(r scripts.Run) {
		u.post(func() { u.onScriptRun(r) })
	})
}

// scriptSettings returns the settings holding scripts and hooks (defaults
// when settings.yaml is not in use).
func (u *appUI) scriptSettings() settings.Settings {
	if u.files == nil {
		return settings.Defaults()
	}
	return u.files.Get()
}

// scriptNames are the defined scripts, sorted. Any goroutine.
func (u *appUI) scriptNames() []string { return u.scriptSettings().ScriptNames() }

// loadScripts hands the scripts and hooks to the runner and dispatcher.
func (u *appUI) loadScripts() {
	s := u.scriptSettings()
	u.runner.SetScripts(s.Scripts)
	u.hooks.SetHooks(s.Hooks)
}

// scriptsChanged reloads scripts after settings.yaml changed. UI goroutine.
func (u *appUI) scriptsChanged() {
	u.loadScripts()
	if u.scripts != nil {
		u.scripts.refresh()
	}
	if u.profiles != nil {
		u.profiles.scriptsChanged()
	}
}

// updateScripts edits scripts/hooks in settings.yaml. UI goroutine.
func (u *appUI) updateScripts(fn func(*settings.Settings)) error {
	if u.files == nil {
		return fmt.Errorf("scripts need settings.yaml, which could not be opened")
	}
	if err := u.files.Update(fn); err != nil {
		return err
	}
	u.scriptsChanged()
	u.tray.rebuild()
	return nil
}

// fireHook runs the hooks for p, adding the active profile and mode. Any
// goroutine; never blocks.
func (u *appUI) fireHook(p scripts.Payload) {
	if p.Profile == "" {
		p.Profile = u.prefs.ActiveProfile()
	}
	if p.Mode == "" {
		p.Mode = string(u.prefs.Mode())
	}
	u.hooks.Fire(p)
}

// runPadScript runs a script asked for by the pad (live controller
// goroutine); never blocks.
func (u *appUI) runPadScript(name string, t scriptTrigger) {
	u.runner.Run(context.Background(), name, scripts.Payload{
		Event: "pad", Control: t.Control, Gesture: t.Gesture,
		Profile: u.prefs.ActiveProfile(), Mode: string(u.prefs.Mode()),
		Action: profile.RunAction(name), Trigger: padTrigger(t),
	})
}

// runScriptNow runs a script from the Scripts screen or the tray and
// toasts the result. UI goroutine.
func (u *appUI) runScriptNow(name string) {
	id := u.runner.Run(context.Background(), name, scripts.Payload{
		Event: "manual", Profile: u.prefs.ActiveProfile(), Mode: string(u.prefs.Mode()), Trigger: "Run now",
	})
	u.runNow[id] = true
	u.toasts.show(toastInfo, "Running "+name+"…")
}

// onScriptRun reflects a run update. UI goroutine.
func (u *appUI) onScriptRun(r scripts.Run) {
	if u.scripts != nil {
		u.scripts.runUpdated(r)
	}
	if r.Status.Done() && u.runNow[r.ID] {
		delete(u.runNow, r.ID)
		kind, msg := runToast(r)
		u.toasts.show(kind, msg)
	}
}

// padPresenceHook fires pad_connected / pad_disconnected when the pad's
// presence changes. UI goroutine.
func (u *appUI) padPresenceHook(d pad.Device, present bool) {
	if present == u.padKnown {
		return
	}
	u.padKnown = present
	ev := scripts.Payload{Event: scripts.EventPadDisconnected}
	if present {
		ev = scripts.Payload{Event: scripts.EventPadConnected, Pad: fmt.Sprintf("%04x:%04x", d.VendorID, d.ProductID)}
	}
	u.fireHook(ev)
}

// forgetPrunedSessions drops hook state of sessions the tracker expired.
func (u *appUI) forgetPrunedSessions() {
	live := map[string]bool{}
	for _, s := range u.tracker.Sessions() {
		live[s.Key()] = true
	}
	for _, k := range u.hooks.Sessions() {
		if !live[k] {
			agent, id, _ := strings.Cut(k, "/")
			u.hooks.ForgetSession(agent, id)
		}
	}
}

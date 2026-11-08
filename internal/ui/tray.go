package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// tray manages the system tray icon and menu when the platform has one.
type tray struct {
	u     *appUI
	desk  desktop.App
	menu  *fyne.Menu
	last  events.State
	wait  int
	model []trayItem
}

func newTray(u *appUI) *tray {
	t := &tray{u: u, last: events.StateIdle}
	u.app.SetIcon(iconApp)
	u.win.SetIcon(iconApp)
	desk, ok := u.app.(desktop.App)
	if !ok {
		return t
	}
	t.desk = desk
	t.menu = fyne.NewMenu("agentpad")
	t.rebuild()
	desk.SetSystemTrayIcon(iconIdle)
	desk.SetSystemTrayWindow(u.win)
	u.win.SetCloseIntercept(func() {
		u.visible.Store(false)
		u.win.Hide()
	})
	return t
}

// state collects what the menu shows. UI goroutine.
func (t *tray) state() trayState {
	s := trayState{Agents: t.last, Active: t.u.prefs.ActiveProfile(), Orient: t.u.profiles.orient,
		Follow: t.u.ledFollow.Load(), Mode: t.u.mode, Waiting: t.wait}
	for _, p := range t.u.profiles.profiles {
		s.Profiles = append(s.Profiles, p.Name)
	}
	return s
}

// rebuild regenerates the menu from the current state. UI goroutine.
func (t *tray) rebuild() {
	if t == nil || t.desk == nil {
		return
	}
	t.model = trayModel(t.state())
	t.menu.Items = t.items(t.model)
	t.desk.SetSystemTrayMenu(t.menu)
	t.menu.Refresh()
}

func (t *tray) items(model []trayItem) []*fyne.MenuItem {
	out := make([]*fyne.MenuItem, 0, len(model))
	for _, it := range model {
		if it.Separator {
			out = append(out, fyne.NewMenuItemSeparator())
			continue
		}
		mi := fyne.NewMenuItem(it.Label, nil)
		if it.Action != trayNone {
			mi.Action = func() { t.do(it) }
		}
		mi.Checked, mi.Disabled, mi.IsQuit = it.Checked, it.Disabled, it.Quit
		if len(it.Children) > 0 {
			mi.ChildMenu = fyne.NewMenu(it.Label, t.items(it.Children)...)
		}
		out = append(out, mi)
	}
	return out
}

// do runs a tray action. UI goroutine (Fyne calls menu actions there).
func (t *tray) do(it trayItem) {
	u := t.u
	switch it.Action {
	case trayProfile:
		u.activateProfile(it.Arg)
	case trayOrientation:
		o, _ := profile.ParseOrientation(it.Arg)
		u.profiles.rotate(o)
		u.toasts.show(toastInfo, "Orientation: "+o.Label()+". Apply the profile to use it on the pad.")
	case trayFollow:
		on := !u.ledFollow.Load()
		u.setLEDFollow(on)
		u.pad.led.follow.SetChecked(on)
	case trayMode:
		u.setMode(ParseMode(it.Arg))
	case trayJump:
		u.jumpNext()
	case trayShow:
		u.visible.Store(true)
		u.win.Show()
		u.win.RequestFocus()
	case trayQuit:
		u.quit()
		return
	}
	t.rebuild()
}

// show reflects s and the number of waiting sessions in the tray. UI goroutine.
func (t *tray) show(s events.State, waiting int) {
	if t.desk == nil || (s == t.last && waiting == t.wait) {
		return
	}
	iconChanged := s != t.last
	t.last, t.wait = s, waiting
	t.rebuild()
	if iconChanged {
		t.desk.SetSystemTrayIcon(stateIcon(s))
	}
}

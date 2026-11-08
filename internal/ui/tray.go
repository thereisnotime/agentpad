package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/thereisnotime/agentpad/internal/events"
)

// tray manages the system tray icon and menu when the platform has one.
type tray struct {
	desk   desktop.App
	menu   *fyne.Menu
	status *fyne.MenuItem
	jump   *fyne.MenuItem
	last   events.State
}

func newTray(u *appUI) *tray {
	t := &tray{}
	u.app.SetIcon(iconApp)
	u.win.SetIcon(iconApp)
	desk, ok := u.app.(desktop.App)
	if !ok {
		return t
	}
	t.desk = desk
	t.status = fyne.NewMenuItem(trayStatus(events.StateIdle), nil)
	t.status.Disabled = true
	show := fyne.NewMenuItem("Show window", func() {
		u.visible.Store(true)
		u.win.Show()
		u.win.RequestFocus()
	})
	// Placeholder: the jump-to-session backend is built separately.
	t.jump = fyne.NewMenuItem("Jump to waiting agent", nil)
	t.jump.Disabled = true
	quit := fyne.NewMenuItem("Quit", u.quit)
	quit.IsQuit = true
	t.menu = fyne.NewMenu("agentpad", show, fyne.NewMenuItemSeparator(), t.status, t.jump, fyne.NewMenuItemSeparator(), quit)
	desk.SetSystemTrayMenu(t.menu)
	desk.SetSystemTrayIcon(iconIdle)
	desk.SetSystemTrayWindow(u.win)
	u.win.SetCloseIntercept(func() {
		u.visible.Store(false)
		u.win.Hide()
	})
	return t
}

// show reflects s in the tray. UI goroutine.
func (t *tray) show(s events.State) {
	if t.desk == nil || s == t.last {
		return
	}
	t.last = s
	t.status.Label = trayStatus(s)
	t.menu.Refresh()
	t.desk.SetSystemTrayIcon(stateIcon(s))
}

func trayStatus(s events.State) string { return "Agents: " + string(s) }

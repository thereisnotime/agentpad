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
	last   events.State
}

func newTray(u *appUI) *tray {
	t := &tray{}
	u.app.SetIcon(iconIdle)
	desk, ok := u.app.(desktop.App)
	if !ok {
		return t
	}
	t.desk = desk
	t.status = fyne.NewMenuItem(trayStatus(events.StateIdle), nil)
	t.status.Disabled = true
	show := fyne.NewMenuItem("Show window", func() {
		u.win.Show()
		u.win.RequestFocus()
	})
	quit := fyne.NewMenuItem("Quit", u.quit)
	quit.IsQuit = true
	t.menu = fyne.NewMenu("agentpad", show, fyne.NewMenuItemSeparator(), t.status, fyne.NewMenuItemSeparator(), quit)
	desk.SetSystemTrayMenu(t.menu)
	desk.SetSystemTrayIcon(iconIdle)
	desk.SetSystemTrayWindow(u.win)
	u.win.SetCloseIntercept(u.win.Hide)
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

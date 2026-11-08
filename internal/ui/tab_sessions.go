package ui

import (
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/events"
)

var sessionColumns = []string{"Agent", "State", "Directory", "Last tool", "Last event"}

type sessionsTab struct {
	u       *appUI
	content fyne.CanvasObject

	home     string
	sessions []events.Session

	dot   *canvas.Circle
	title *canvas.Text
	count *widget.Label
	table *widget.Table
	empty fyne.CanvasObject
}

func newSessionsTab(u *appUI) *sessionsTab {
	t := &sessionsTab{u: u}
	t.home, _ = os.UserHomeDir()

	t.dot = canvas.NewCircle(colorIdle)
	t.title = canvas.NewText(stateTitle(events.StateIdle), theme.Color(theme.ColorNameForeground))
	t.title.TextSize = 24
	t.title.TextStyle = fyne.TextStyle{Bold: true}
	t.count = widget.NewLabel("")
	t.count.Importance = widget.LowImportance
	header := container.NewHBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(22, 22), t.dot)),
		t.title,
		container.NewCenter(t.count),
	)

	t.table = newTextTable(sessionColumns, []float32{90, 120, 330, 130, 110},
		func() int { return len(t.sessions) }, t.cell)

	empty := widget.NewLabel("No active agent sessions. Add the hooks from the Setup tab, then start an agent.")
	empty.Alignment = fyne.TextAlignCenter
	empty.Wrapping = fyne.TextWrapWord
	t.empty = container.NewCenter(empty)

	t.content = container.NewBorder(container.NewVBox(container.NewPadded(header), widget.NewSeparator()),
		nil, nil, nil, container.NewStack(t.table, t.empty))
	return t
}

func (t *sessionsTab) cell(row, col int, dot *canvas.Circle, label *widget.Label) {
	if row >= len(t.sessions) {
		label.SetText("")
		return
	}
	s := t.sessions[row]
	switch col {
	case 0:
		label.SetText(s.Agent)
	case 1:
		dot.FillColor = stateColor(s.State)
		dot.Show()
		dot.Refresh()
		label.SetText(string(s.State))
	case 2:
		label.SetText(shortenPath(s.Cwd, t.home))
	case 3:
		label.SetText(s.LastTool)
	case 4:
		label.SetText(relTime(s.LastEvent, time.Now()))
	}
}

// refresh reloads sessions from the tracker. UI goroutine.
func (t *sessionsTab) refresh() {
	t.sessions = t.u.tracker.Sessions()
	if len(t.sessions) == 0 {
		t.count.SetText("")
		t.empty.Show()
	} else {
		t.count.SetText(plural(len(t.sessions), "session"))
		t.empty.Hide()
	}
	t.table.Refresh()
}

func (t *sessionsTab) showAggregate(s events.State) {
	t.dot.FillColor = stateColor(s)
	t.dot.Refresh()
	t.title.Text = stateTitle(s)
	t.title.Refresh()
}

package ui

import (
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/events"
)

type logTab struct {
	u       *appUI
	content fyne.CanvasObject

	events *ring
	table  *widget.Table
	count  *widget.Label
}

func newLogTab(u *appUI) *logTab {
	t := &logTab{u: u, events: newRing(logRows)}

	t.table = newTextTable(logColumns, []float32{80, 80, 120, 110, 110, 360},
		t.events.len, func(row, col int, _ *canvas.Circle, label *widget.Label) {
			if row >= t.events.len() {
				label.SetText("")
				return
			}
			label.SetText(logCell(t.events.newest(row), col))
		})

	open := widget.NewButtonWithIcon("Open log file", theme.FolderOpenIcon(), t.openFile)
	clear := widget.NewButtonWithIcon("Clear view", theme.ContentClearIcon(), func() {
		t.events.clear()
		t.changed()
	})
	t.count = widget.NewLabel("")
	t.count.Importance = widget.LowImportance
	path := widget.NewLabel(u.deps.LogPath)
	path.Importance = widget.LowImportance
	path.Truncation = fyne.TextTruncateEllipsis

	bar := container.NewBorder(nil, nil, t.count, container.NewHBox(open, clear), path)
	t.content = container.NewBorder(bar, nil, nil, nil, t.table)
	t.changed()
	return t
}

// load replaces the view with past events (oldest first). UI goroutine.
func (t *logTab) load(past []events.Event) {
	t.events.clear()
	t.events.add(past...)
	t.changed()
}

// add appends one live event. UI goroutine.
func (t *logTab) add(e events.Event) {
	t.events.add(e)
	t.changed()
}

func (t *logTab) changed() {
	t.count.SetText("newest first, " + plural(t.events.len(), "event"))
	t.table.Refresh()
}

func (t *logTab) openFile() {
	cmd := exec.Command("xdg-open", t.u.deps.LogPath)
	if err := cmd.Start(); err != nil {
		dialog.ShowError(err, t.u.win)
		return
	}
	go func() { _ = cmd.Wait() }()
}

package ui

import (
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/events"
)

var logWidths = []float32{104, 100, 136, 110, 124, 460}

type logTab struct {
	u       *appUI
	content fyne.CanvasObject

	events *ring
	table  *widget.Table
	count  *widget.Label
}

func newLogTab(u *appUI) *logTab {
	t := &logTab{u: u, events: newRing(logRows)}

	t.table = widget.NewTableWithHeaders(
		func() (int, int) { return t.events.len(), len(logColumns) },
		newLogCell,
		func(id widget.TableCellID, o fyne.CanvasObject) { t.updateCell(id, o) },
	)
	t.table.ShowHeaderColumn = false
	t.table.CreateHeader = func() fyne.CanvasObject {
		return inset(4, 4, 8, 4, text("", 12, colText2, true))
	}
	t.table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(logColumns) {
			lbl := o.(*fyne.Container).Objects[0].(*canvas.Text)
			lbl.Text = logColumns[id.Col]
			lbl.Refresh()
		}
	}
	for i, w := range logWidths {
		t.table.SetColumnWidth(i, w)
	}

	open := widget.NewButtonWithIcon("Open file", theme.FolderOpenIcon(), t.openFile)
	clear := widget.NewButtonWithIcon("Clear view", theme.ContentClearIcon(), func() {
		t.events.clear()
		t.changed()
	})
	clear.Importance = widget.LowImportance
	t.count = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	path := widget.NewLabelWithStyle(u.deps.LogPath, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	path.Importance = widget.LowImportance
	path.Truncation = fyne.TextTruncateEllipsis

	toolbar := surface(container.NewBorder(nil, nil, nil, vcenter(container.NewHBox(open, clear)), tight(t.count, path)))
	tableBG := roundRect(colSurface, 14)
	tableBG.StrokeColor = colBorder
	tableBG.StrokeWidth = 1
	tableCard := container.NewStack(tableBG, inset(6, 6, 6, 6, t.table))
	// toolbar stays put; only the table scrolls
	t.content = inset(18, 18, 20, 20, container.NewBorder(
		container.NewVBox(screenTitle("Log", ""), toolbar), nil, nil, nil, tableCard))
	t.changed()
	return t
}

// newLogCell: a label, or a coloured badge for the kind column.
func newLogCell() fyne.CanvasObject {
	label := widget.NewLabel("")
	label.Truncation = fyne.TextTruncateEllipsis
	bg := roundRect(colorIdle, 9)
	txt := text("", 11, colText, true)
	badge := container.NewHBox(vcenter(container.NewStack(bg, inset(2, 2, 8, 8, txt))))
	return container.NewStack(label, badge)
}

func (t *logTab) updateCell(id widget.TableCellID, o fyne.CanvasObject) {
	c := o.(*fyne.Container)
	label := c.Objects[0].(*widget.Label)
	badge := c.Objects[1].(*fyne.Container)
	if id.Row >= t.events.len() {
		label.SetText("")
		badge.Hide()
		return
	}
	e := t.events.newest(id.Row)
	if id.Col == 2 {
		label.Hide()
		badge.Show()
		st := badge.Objects[0].(*fyne.Container).Objects[1].(*fyne.Container)
		bg := st.Objects[0].(*canvas.Rectangle)
		txt := st.Objects[1].(*fyne.Container).Objects[0].(*canvas.Text)
		col := kindColor(e.Kind)
		bg.FillColor = withAlpha(col, 0x30)
		bg.StrokeColor = withAlpha(col, 0x70)
		bg.StrokeWidth = 1
		txt.Text = string(e.Kind)
		txt.Color = col
		bg.Refresh()
		txt.Refresh()
		badge.Refresh()
		return
	}
	badge.Hide()
	label.Show()
	label.TextStyle = fyne.TextStyle{Monospace: id.Col == 0 || id.Col == 4}
	label.Importance = widget.MediumImportance
	if id.Col == 0 || id.Col == 4 {
		label.Importance = widget.LowImportance
	}
	label.SetText(logCell(e, id.Col))
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
	t.count.SetText(plural(t.events.len(), "event") + ", newest first")
	t.table.Refresh()
}

func (t *logTab) openFile() {
	cmd := exec.Command("xdg-open", t.u.deps.LogPath)
	if err := cmd.Start(); err != nil {
		t.u.toasts.show(toastError, "Could not open the log: "+err.Error())
		return
	}
	go func() { _ = cmd.Wait() }()
}

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// newTextTable builds a read-only table with a header row. Each cell is an
// optional coloured dot followed by a truncated label; cell fills both.
func newTextTable(headers []string, widths []float32, rows func() int,
	cell func(row, col int, dot *canvas.Circle, label *widget.Label)) *widget.Table {
	t := widget.NewTableWithHeaders(
		func() (int, int) { return rows(), len(headers) },
		newTableCell,
		func(id widget.TableCellID, o fyne.CanvasObject) {
			dot, label := tableCellParts(o)
			dot.Hide()
			cell(id.Row, id.Col, dot, label)
		},
	)
	t.ShowHeaderColumn = false
	t.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	t.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(headers) {
			o.(*widget.Label).SetText(headers[id.Col])
		}
	}
	for i, w := range widths {
		t.SetColumnWidth(i, w)
	}
	return t
}

func newTableCell() fyne.CanvasObject {
	dot := canvas.NewCircle(colorIdle)
	dot.Hide()
	label := widget.NewLabel("")
	label.Truncation = fyne.TextTruncateEllipsis
	dotBox := container.NewCenter(container.NewGridWrap(fyne.NewSize(10, 10), dot))
	return container.NewBorder(nil, nil, dotBox, nil, label)
}

func tableCellParts(o fyne.CanvasObject) (*canvas.Circle, *widget.Label) {
	c := o.(*fyne.Container)
	label := c.Objects[0].(*widget.Label)
	dot := c.Objects[1].(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*canvas.Circle)
	return dot, label
}

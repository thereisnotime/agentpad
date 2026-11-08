package ui

import (
	"testing"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/events"
)

func TestCopyButton(t *testing.T) {
	a := test.NewTempApp(t)
	b := newCopyButton(a, "Copy", func() string { return "hello" })
	test.Tap(b)
	if got := a.Clipboard().Content(); got != "hello" {
		t.Fatalf("clipboard = %q", got)
	}
	if b.Text != "Copied" {
		t.Errorf("button text = %q, want Copied", b.Text)
	}
}

func TestSnippetCardCopies(t *testing.T) {
	a := test.NewTempApp(t)
	card := snippetCard(a, "Codex CLI", "~/.codex/config.toml", `notify = ["agentpad"]`).(*widget.Card)
	if card.Title != "Codex CLI" {
		t.Errorf("title %q", card.Title)
	}
	w := test.NewTempWindow(t, card)
	_ = w
	var btn *widget.Button
	for _, o := range test.LaidOutObjects(card) {
		if b, ok := o.(*widget.Button); ok {
			btn = b
		}
	}
	if btn == nil {
		t.Fatal("no copy button")
	}
	test.Tap(btn)
	if got := a.Clipboard().Content(); got != `notify = ["agentpad"]` {
		t.Errorf("clipboard = %q", got)
	}
}

func TestTextTableCells(t *testing.T) {
	test.NewTempApp(t)
	rows := [][]string{{"claude", "busy"}, {"codex", "attention"}}
	tbl := newTextTable([]string{"Agent", "State"}, []float32{80, 100},
		func() int { return len(rows) },
		func(row, col int, dot *canvas.Circle, label *widget.Label) {
			label.SetText(rows[row][col])
			if col == 1 {
				dot.FillColor = stateColor(events.State(rows[row][col]))
				dot.Show()
			}
		})
	n, c := tbl.Length()
	if n != 2 || c != 2 {
		t.Fatalf("length = %d x %d", n, c)
	}
	cell := newTableCell()
	tbl.UpdateCell(widget.TableCellID{Row: 1, Col: 1}, cell)
	dot, label := tableCellParts(cell)
	if label.Text != "attention" || !dot.Visible() || dot.FillColor != colorAttention {
		t.Errorf("cell = %q visible=%v color=%v", label.Text, dot.Visible(), dot.FillColor)
	}
	tbl.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	if _, label := tableCellParts(cell); label.Text != "claude" {
		t.Errorf("cell = %q", label.Text)
	}
	if dot.Visible() {
		t.Error("dot should be hidden for a plain column")
	}
}

func TestStateIcon(t *testing.T) {
	if stateIcon(events.StateAttention) == stateIcon(events.StateIdle) ||
		stateIcon(events.StateBusy) == stateIcon(events.StateIdle) {
		t.Error("icons must differ per state")
	}
}

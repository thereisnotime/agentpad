package ui

import (
	"testing"

	"fyne.io/fyne/v2"
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
	if b.Text != "Copied ✓" {
		t.Errorf("button text = %q", b.Text)
	}
}

func TestSnippetCardCopies(t *testing.T) {
	a := test.NewTempApp(t)
	card := snippetCard(a, "codex", "Codex CLI", "~/.codex/config.toml", `notify = ["agentpad"]`)
	test.NewTempWindow(t, card)
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

func TestShellSwitchesAtBreakpoint(t *testing.T) {
	test.NewTempApp(t)
	objs := []fyne.CanvasObject{widget.NewLabel("h"), widget.NewLabel("side"), widget.NewLabel("tabs"), widget.NewLabel("c")}
	l := shellLayout{}
	l.Layout(objs, fyne.NewSize(1200, 800))
	if !objs[1].Visible() || objs[2].Visible() || objs[3].Position().X != sidebarWidth {
		t.Error("wide: want sidebar")
	}
	l.Layout(objs, fyne.NewSize(500, 800))
	if objs[1].Visible() || !objs[2].Visible() || objs[3].Position().X != 0 {
		t.Error("narrow: want top tabs")
	}
	if objs[3].Size().Width != 500 {
		t.Errorf("narrow content width = %v", objs[3].Size().Width)
	}
}

func TestTwoPaneStacksWhenNarrow(t *testing.T) {
	test.NewTempApp(t)
	a, b := widget.NewLabel("main"), widget.NewLabel("side")
	l := &twoPaneLayout{side: inspectorWidth, gap: 16}
	l.Layout([]fyne.CanvasObject{a, b}, fyne.NewSize(1000, 600))
	if b.Position().X <= a.Position().X || b.Position().Y != 0 {
		t.Error("wide: side should be right of main")
	}
	l.Layout([]fyne.CanvasObject{a, b}, fyne.NewSize(480, 600))
	if b.Position().X != 0 || b.Position().Y <= 0 {
		t.Error("narrow: side should be below main")
	}
}

func TestStateIcon(t *testing.T) {
	if stateIcon(events.StateAttention) == stateIcon(events.StateIdle) ||
		stateIcon(events.StateBusy) == stateIcon(events.StateIdle) {
		t.Error("icons must differ per state")
	}
}

package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/setup"
)

const setupNote = "agentpad never edits these files. Copy each snippet into the file shown and merge it with what is already there."

func newSetupTab(u *appUI) fyne.CanvasObject {
	bin := u.deps.Bin
	if bin == "" {
		bin = "agentpad"
	}
	cards := newGrid(420,
		snippetCard(u.app, "claude", "Claude Code", "~/.claude/settings.json (merge into \"hooks\")", setup.ClaudeSnippet(bin)),
		snippetCard(u.app, "codex", "Codex CLI", "~/.codex/config.toml", setup.CodexSnippet(bin)),
		snippetCard(u.app, "opencode", "OpenCode", "~/.config/opencode/plugins/agentpad.js", setup.OpenCodePlugin(u.deps.Addr)),
		snippetCard(u.app, "", "udev rule", "/etc/udev/rules.d/50-agentpad.rules (or run `just udev-install`), then replug the pad", setup.UdevRule()),
	)
	return screen(container.NewVBox(screenTitle("Setup", setupNote), cards))
}

// snippetCard shows text read-only in a monospace box with a Copy button.
func snippetCard(a fyne.App, agent, title, target, snippet string) fyne.CanvasObject {
	letter, col := agentBadge(agent)
	var badge fyne.CanvasObject
	if agent == "" {
		badge = fixed(34, 34, canvas.NewCircle(colSurface3), inset(7, 7, 7, 7, widget.NewIcon(theme.ComputerIcon())))
	} else {
		l := text(letter, 15, colText, true)
		badge = fixed(34, 34, canvas.NewCircle(col), container.NewCenter(l))
	}
	head := container.NewBorder(nil, nil, vcenter(badge), vcenter(newCopyButton(a, "Copy", func() string { return snippet })),
		tight(boldLabel(title), monoSecondary(target)))

	grid := widget.NewTextGridFromString(snippet)
	grid.Scroll = fyne.ScrollBoth // long snippets scroll inside their box
	box := roundRect(rgb(0x111217), 10)
	box.StrokeColor = colBorder
	box.StrokeWidth = 1
	code := fixed(0, 200, box, inset(8, 8, 10, 10, grid))
	return surface(container.NewBorder(head, nil, nil, nil, inset(8, 0, 0, 0, code)))
}

func monoSecondary(s string) *widget.Label {
	l := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	l.Importance = widget.LowImportance
	l.Wrapping = fyne.TextWrapBreak
	return l
}

// newCopyButton copies text() to the clipboard and briefly confirms it.
func newCopyButton(a fyne.App, label string, text func() string) *widget.Button {
	var b *widget.Button
	b = widget.NewButtonWithIcon(label, theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(text())
		b.SetText("Copied ✓")
		b.SetIcon(theme.NewSuccessThemedResource(theme.ConfirmIcon()))
		time.AfterFunc(1500*time.Millisecond, func() {
			fyne.Do(func() {
				b.SetText(label)
				b.SetIcon(theme.ContentCopyIcon())
			})
		})
	})
	return b
}

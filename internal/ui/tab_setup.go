package ui

import (
	"time"

	"fyne.io/fyne/v2"
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
	note := wrapLabel(setupNote)
	note.Importance = widget.MediumImportance
	about := widget.NewLabel("agentpad " + u.deps.Version + "  ·  " + bin)
	about.Importance = widget.LowImportance
	about.Truncation = fyne.TextTruncateEllipsis

	sections := container.NewVBox(
		note,
		snippetCard(u.app, "Claude Code", "~/.claude/settings.json (merge into \"hooks\")", setup.ClaudeSnippet(bin)),
		snippetCard(u.app, "Codex CLI", "~/.codex/config.toml", setup.CodexSnippet(bin)),
		snippetCard(u.app, "OpenCode", "~/.config/opencode/plugins/agentpad.js", setup.OpenCodePlugin(u.deps.Addr)),
		snippetCard(u.app, "udev rule", "/etc/udev/rules.d/50-agentpad.rules (or run `just udev-install`), then replug the pad", setup.UdevRule()),
		about,
	)
	return container.NewVScroll(container.NewPadded(sections))
}

// snippetCard shows text read-only in a monospace grid with a Copy button.
func snippetCard(a fyne.App, title, target, text string) fyne.CanvasObject {
	grid := widget.NewTextGridFromString(text)
	grid.Scroll = fyne.ScrollHorizontalOnly
	copyBtn := newCopyButton(a, "Copy", func() string { return text })
	body := container.NewBorder(nil, container.NewHBox(copyBtn), nil, nil, grid)
	return widget.NewCard(title, target, body)
}

// newCopyButton copies text() to the clipboard and briefly confirms it.
func newCopyButton(a fyne.App, label string, text func() string) *widget.Button {
	var b *widget.Button
	b = widget.NewButtonWithIcon(label, theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(text())
		b.SetText("Copied")
		b.SetIcon(theme.ConfirmIcon())
		go func() {
			time.Sleep(1500 * time.Millisecond)
			fyne.Do(func() {
				b.SetText(label)
				b.SetIcon(theme.ContentCopyIcon())
			})
		}()
	})
	return b
}

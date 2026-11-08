package ui

import (
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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
		u.snippetCard("claude", "Claude Code", "~/.claude/settings.json (merge into \"hooks\")",
			editTarget{Path: "~/.claude/settings.json"}, setup.ClaudeSnippet(bin)),
		u.snippetCard("codex", "Codex CLI", "~/.codex/config.toml",
			editTarget{Path: "~/.codex/config.toml"}, setup.CodexSnippet(bin)),
		u.snippetCard("opencode", "OpenCode", "~/.config/opencode/plugins/agentpad.js",
			editTarget{Path: "~/.config/opencode/plugins/agentpad.js"}, setup.OpenCodePlugin(u.deps.Addr)),
		u.snippetCard("", "udev rule", "/etc/udev/rules.d/50-agentpad.rules (or run `just udev-install`), then replug the pad",
			editTarget{Path: udevRulePath, ReadOnly: true}, setup.UdevRule()),
	)
	return screen(container.NewVBox(screenTitle("Setup", setupNote), cards))
}

const udevRulePath = "/etc/udev/rules.d/50-agentpad.rules"

// editFile opens a snippet's target file in the default editor, offering to
// create it empty first. It never writes the snippet into it. UI goroutine.
func (u *appUI) editFile(t editTarget) {
	home, _ := os.UserHomeDir()
	p, step := planEdit(t, home, fileExists)
	open := func() {
		if err := u.openFile(p); err != nil {
			u.toasts.show(toastError, "Could not open "+p+": "+err.Error())
		}
	}
	switch step {
	case editOpen:
		open()
	case editCreate:
		dialog.ShowConfirm("Create file", "Create "+p+"? It will be empty; paste the snippet into it.", func(ok bool) {
			if !ok {
				return
			}
			if err := createEmpty(p); err != nil {
				u.toasts.show(toastError, "Could not create "+p+": "+err.Error())
				return
			}
			open()
		}, u.win)
	case editUdevHint:
		dialog.ShowInformation("No udev rule yet", p+" does not exist. Run `just udev-install` "+
			"(or save the rule there with sudo), then replug the pad.", u.win)
	default:
		u.toasts.show(toastError, "No file to open for this snippet.")
	}
}

// snippetCard shows text read-only in a monospace box with Copy and Edit file buttons.
func (u *appUI) snippetCard(agent, title, target string, edit editTarget, snippet string) fyne.CanvasObject {
	a := u.app
	letter, col := agentBadge(agent)
	var badge fyne.CanvasObject
	if agent == "" {
		badge = fixed(34, 34, canvas.NewCircle(colSurface3), inset(7, 7, 7, 7, widget.NewIcon(theme.ComputerIcon())))
	} else {
		l := text(letter, 15, colText, true)
		badge = fixed(34, 34, canvas.NewCircle(col), container.NewCenter(l))
	}
	editBtn := widget.NewButtonWithIcon("Edit file", theme.DocumentCreateIcon(), func() { u.editFile(edit) })
	buttons := container.NewHBox(newCopyButton(a, "Copy", func() string { return snippet }), editBtn)
	head := container.NewBorder(nil, nil, vcenter(badge), vcenter(buttons),
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

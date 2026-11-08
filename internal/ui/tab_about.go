package ui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const aboutText = "agentpad turns a cheap CH57x 3-key + knob macro pad into a physical controller " +
	"for AI coding agents: approve, reject and navigate from the pad, and see at a glance when an agent needs you."

func newAboutTab(u *appUI) fyne.CanvasObject {
	name := text("agentpad", 26, colText, true)
	ver := widget.NewLabelWithStyle("Version "+u.deps.Version, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	ver.Importance = widget.LowImportance
	head := container.NewBorder(nil, nil, vcenter(fixed(72, 72, widget.NewIcon(iconApp))), nil, inset(0, 0, 10, 0, container.NewVBox(name, ver)))

	bug := widget.NewButtonWithIcon("Report a bug", theme.ErrorIcon(), func() { u.open(bugReportURL(u.diagnostics())) })
	bug.Importance = widget.HighImportance
	feature := widget.NewButtonWithIcon("Request a feature", theme.ContentAddIcon(), func() { u.open(featureReqURL) })
	diag := newCopyButton(u.app, "Copy diagnostics", func() string { return u.diagnostics().text() })
	diag.Importance = widget.LowImportance

	links := container.NewVBox(
		link("Repository", repoURL),
		link("Documentation (README)", docsURL),
		link("Changelog and releases", releasesURL),
	)
	license := secondary("Released under the MIT License. Not affiliated with the pad's manufacturer or any agent vendor.")

	return screen(container.NewVBox(
		screenTitle("About", ""),
		surface(container.NewVBox(head, wrapLabel(aboutText), chipRow(bug, feature, diag))),
		newGrid(300,
			newCard("Links", "", links),
			newCard("License", "", license),
			newSettingsCard(u),
		),
	))
}

func link(label, raw string) fyne.CanvasObject {
	u, err := url.Parse(raw)
	if err != nil {
		return widget.NewLabel(label)
	}
	return widget.NewHyperlink(label, u)
}

// open launches a URL in the browser, reporting failures in a toast.
func (u *appUI) open(raw string) {
	parsed, err := url.Parse(raw)
	if err == nil {
		err = u.app.OpenURL(parsed)
	}
	if err != nil {
		u.toasts.show(toastError, "Could not open the browser: "+err.Error())
	}
}

// diagnostics describes this install for bug reports. UI goroutine.
func (u *appUI) diagnostics() diagnostics {
	return diagnostics{
		Version: u.deps.Version,
		OS:      localOS(),
		Pad:     padString(u.device, u.deviceErr == nil),
	}
}

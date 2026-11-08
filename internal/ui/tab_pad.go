package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/setup"
)

const usbCHint = "Nothing detected? Many of these pads lack the USB-C CC resistors, so a USB-C host never powers them. " +
	"Use a USB-A port or a USB-C to USB-A adapter, and a cable that carries data."

type padTab struct {
	u       *appUI
	content fyne.CanvasObject

	connIcon  *widget.Icon
	connLabel *widget.Label
	connHint  *widget.Label

	toolIcon  *widget.Icon
	toolLabel *widget.Label
	toolHint  *widget.Label

	ledSelect *widget.Select
	testBtn   *widget.Button
	status    *widget.Label
}

func newPadTab(u *appUI) *padTab {
	t := &padTab{u: u}

	t.connIcon = widget.NewIcon(theme.QuestionIcon())
	t.connLabel = widget.NewLabelWithStyle("Looking for a pad…", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.connHint = wrapLabel(usbCHint)
	t.connHint.Importance = widget.LowImportance
	conn := widget.NewCard("Connection", "CH57x 3 keys + 1 knob (1189:8890 / 8840 / 8842)",
		container.NewVBox(container.NewBorder(nil, nil, t.connIcon, nil, t.connLabel), t.connHint))

	t.toolIcon = widget.NewIcon(theme.QuestionIcon())
	t.toolLabel = widget.NewLabelWithStyle("Checking…", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.toolHint = wrapLabel("Install it with `just tool-install` (puts it in ~/.local/bin), then press Recheck.")
	t.toolHint.Importance = widget.LowImportance
	t.toolHint.Hide()
	recheck := widget.NewButtonWithIcon("Recheck", theme.ViewRefreshIcon(), t.checkTool)
	backend := widget.NewCard("Backend", "ch57x-keyboard-tool writes mappings and LED modes",
		container.NewVBox(container.NewBorder(nil, nil, t.toolIcon, recheck, t.toolLabel), t.toolHint))

	t.ledSelect = widget.NewSelect(ledModes, nil)
	t.ledSelect.SetSelectedIndex(1)
	t.testBtn = widget.NewButtonWithIcon("Test", theme.MediaPlayIcon(), t.testLED)

	// Set initial values before wiring callbacks so loading doesn't write prefs.
	follow := widget.NewCheck("LED follows agents", nil)
	follow.SetChecked(u.ledFollow.Load())
	follow.OnChanged = u.setLEDFollow
	attention := widget.NewSelect(ledModes[1:], nil)
	if n := int(u.attentionMode.Load()); n > 0 && n < len(ledModes) {
		attention.SetSelectedIndex(n - 1)
	}
	attention.OnChanged = func(s string) {
		if n := ledModeIndex(s); n > 0 {
			u.setAttentionMode(n)
		}
	}
	led := widget.NewCard("LED", "Board 8890 only selects LED modes by index; 0 is off",
		widget.NewForm(
			widget.NewFormItem("Mode", container.NewHBox(t.ledSelect, t.testBtn)),
			widget.NewFormItem("", follow),
			widget.NewFormItem("Attention mode", attention),
		))

	t.status = widget.NewLabel("")
	t.status.Wrapping = fyne.TextWrapWord

	t.content = container.NewBorder(nil, t.status, nil, nil,
		container.NewVScroll(container.NewVBox(conn, backend, led)))
	return t
}

// start does the first device and backend check. UI goroutine.
func (t *padTab) start() {
	t.checkTool()
	go func() {
		dev, err := pad.Detect()
		t.u.padPresent.Store(err == nil)
		if err == nil {
			t.u.writer.setProduct(dev.ProductID)
		}
		fyne.Do(func() { t.showDevice(dev, err) })
	}()
}

func (t *padTab) showDevice(d pad.Device, err error) {
	if err != nil {
		t.connIcon.SetResource(theme.WarningIcon())
		if errors.Is(err, pad.ErrNotConnected) {
			t.connLabel.SetText("Not connected")
		} else {
			t.connLabel.SetText("Detection failed: " + err.Error())
		}
		t.connHint.Show()
		return
	}
	t.connIcon.SetResource(theme.ConfirmIcon())
	t.connLabel.SetText(deviceLabel(d))
	t.connHint.Hide()
}

func (t *padTab) checkTool() {
	go func() {
		err := t.u.writer.Available()
		fyne.Do(func() {
			if err != nil {
				t.toolIcon.SetResource(theme.ErrorIcon())
				t.toolLabel.SetText("ch57x-keyboard-tool not available")
				t.toolHint.Show()
				return
			}
			t.toolIcon.SetResource(theme.ConfirmIcon())
			t.toolLabel.SetText("ch57x-keyboard-tool ready")
			t.toolHint.Hide()
		})
	}()
}

func (t *padTab) testLED() {
	n := ledModeIndex(t.ledSelect.Selected)
	if n < 0 {
		return
	}
	t.testBtn.Disable()
	t.setStatus("Setting LED mode…")
	go func() {
		err := t.u.writer.SetLED(n)
		fyne.Do(func() {
			t.testBtn.Enable()
			if err != nil {
				t.u.showWriteError("LED test failed", err)
				t.setStatus("LED test failed: " + writeErrMessage(err))
				return
			}
			t.setStatus("LED mode " + ledModes[n] + " set")
		})
	}()
}

func (t *padTab) setStatus(s string) { t.status.SetText(s) }

// showWriteError explains a failed pad write. UI goroutine.
func (u *appUI) showWriteError(title string, err error) {
	if errors.Is(err, pad.ErrPermission) {
		rule := setup.UdevRule()
		grid := widget.NewTextGridFromString(rule)
		body := container.NewVBox(
			wrapLabel("Your user cannot open the pad. Add this udev rule as /etc/udev/rules.d/50-agentpad.rules "+
				"(or run `just udev-install`), then replug the pad."),
			grid,
			container.NewHBox(newCopyButton(u.app, "Copy rule", func() string { return rule })),
		)
		d := dialog.NewCustom(title, "Close", body, u.win)
		d.Resize(fyne.NewSize(620, 0))
		d.Show()
		return
	}
	dialog.ShowInformation(title, writeErrMessage(err), u.win)
}

func wrapLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

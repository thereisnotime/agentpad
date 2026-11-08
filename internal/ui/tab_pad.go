package ui

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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
	connBG    interface{ Refresh() }
	connTitle *widget.Label
	connSub   *widget.Label
	connHint  *widget.Label

	toolIcon  *widget.Icon
	toolTitle *widget.Label
	toolHint  *widget.Label

	ledHolder *fyne.Container
	ledFamily string
	led       *ledPanel
	status    *widget.Label
}

func newPadTab(u *appUI) *padTab {
	t := &padTab{u: u}

	connBadge, connBG, connIcon := statusIcon(theme.QuestionIcon(), colorIdle)
	t.connIcon, t.connBG = connIcon, connBG
	t.connTitle = boldLabel("Looking for a pad…")
	t.connSub = secondary("CH57x 3 keys + knob (1189:8890 / 8840 / 8842)")
	t.connHint = secondary(usbCHint)
	conn := surface(container.NewVBox(
		container.NewBorder(nil, nil, vcenter(connBadge), nil, tight(t.connTitle, t.connSub)),
		t.connHint,
	))

	toolBadge, _, toolIcon := statusIcon(theme.QuestionIcon(), colorIdle)
	t.toolIcon = toolIcon
	t.toolTitle = boldLabel("Checking ch57x-keyboard-tool…")
	t.toolHint = secondary("Install it with `just tool-install` (puts it in ~/.local/bin), then press Recheck.")
	t.toolHint.Hide()
	recheck := widget.NewButtonWithIcon("Recheck", theme.ViewRefreshIcon(), t.checkTool)
	backend := surface(container.NewVBox(
		container.NewBorder(nil, nil, vcenter(toolBadge), vcenter(recheck),
			tight(t.toolTitle, secondary("Writes key mappings and LED settings"))),
		t.toolHint,
	))

	t.status = secondary("")
	t.status.Hide()
	t.ledHolder = container.NewStack()
	t.rebuildLED(u.ledProduct(0))

	body := container.NewVBox(
		screenTitle("Pad", "Connection, backend and lighting"),
		newGrid(320, conn, backend),
		t.ledHolder,
		t.status,
	)
	t.content = screen(body)
	return t
}

// rebuildLED swaps in the LED panel for pid's model. UI goroutine.
func (t *padTab) rebuildLED(pid uint16) {
	fam := ledFamily(pid)
	if fam == t.ledFamily {
		return
	}
	t.ledFamily = fam
	t.led = newLEDPanel(t.u, pid)
	t.ledHolder.Objects = []fyne.CanvasObject{t.led.content}
	t.ledHolder.Refresh()
}

// reloadLED rebuilds the LED panel (after settings changed elsewhere). UI goroutine.
func (t *padTab) reloadLED() {
	pid := t.u.ledProduct(t.u.device.ProductID)
	t.ledFamily = ""
	t.rebuildLED(pid)
}

// start does the first device and backend check. UI goroutine.
func (t *padTab) start() {
	t.checkTool()
	go t.u.detect()
}

// showDevice is called by appUI.showDevice. UI goroutine.
func (t *padTab) showDevice(d pad.Device, err error) {
	if err != nil {
		t.connIcon.SetResource(theme.WarningIcon())
		if errors.Is(err, pad.ErrNotConnected) {
			t.connTitle.SetText("No pad connected")
		} else {
			t.connTitle.SetText("Detection failed: " + err.Error())
		}
		t.connSub.SetText("CH57x 3 keys + knob (1189:8890 / 8840 / 8842)")
		t.connHint.Show()
		return
	}
	t.connIcon.SetResource(theme.NewSuccessThemedResource(theme.ConfirmIcon()))
	name := d.Product
	if name == "" {
		name = "CH57x pad"
	}
	t.connTitle.SetText(name + " connected")
	t.connSub.SetText(fmt.Sprintf("%04x:%04x · USB bus %s", d.VendorID, d.ProductID, d.BusPath))
	t.connHint.Hide()
	t.rebuildLED(d.ProductID)
}

func (t *padTab) checkTool() {
	go func() {
		err := t.u.writer.Available()
		fyne.Do(func() {
			if err != nil {
				t.toolIcon.SetResource(theme.NewErrorThemedResource(theme.ErrorIcon()))
				t.toolTitle.SetText("ch57x-keyboard-tool not found")
				t.toolHint.Show()
				return
			}
			t.toolIcon.SetResource(theme.NewSuccessThemedResource(theme.ConfirmIcon()))
			t.toolTitle.SetText("ch57x-keyboard-tool ready")
			t.toolHint.Hide()
		})
	}()
}

func (t *padTab) setStatus(s string) {
	t.status.SetText(s)
	t.status.Show()
}

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
	u.toasts.show(toastError, title+": "+writeErrMessage(err))
}

// actionRow right-aligns buttons, wrapping on narrow widths.
func actionRow(objs ...fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, container.NewHBox(objs...), layout.NewSpacer())
}

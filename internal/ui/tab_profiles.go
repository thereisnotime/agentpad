package ui

import (
	"errors"
	"fmt"
	"image/color"
	"reflect"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/evdev"
	"github.com/thereisnotime/agentpad/internal/profile"
)

const layersNote = "Layer 1 is this profile, layers 2 and 3 are the next two profiles. The side button on the pad switches layers."

type profilesTab struct {
	u       *appUI
	content fyne.CanvasObject

	profiles []profile.Profile
	selected int             // index into profiles, -1 for none
	stored   profile.Profile // as saved
	draft    profile.Profile // as edited
	control  controlID
	orient   profile.Orientation

	chips    *fyne.Container
	title    *widget.Label
	desc     *widget.Label
	unsaved  *widget.Label
	view     *padView
	orientLb *widget.Label

	inspTitle  *widget.Label
	inspSub    *widget.Label
	slotsBox   *fyne.Container
	entries    map[string]*widget.SelectEntry
	activeSlot string
	message    *widget.Label
	saveBtn    *widget.Button
	resetBtn   *widget.Button
	applyBtn   *widget.Button
	loading    bool

	caption    *canvas.Text
	captionGen int
	fade       *fyne.Animation

	gestureMode bool
	liveHint    *widget.Label
	warning     *widget.Label
	monitor     *fyne.Container
	monitorBox  fyne.CanvasObject
	monLog      gestureLog
}

func newProfilesTab(u *appUI) *profilesTab {
	t := &profilesTab{u: u, selected: -1, entries: map[string]*widget.SelectEntry{}}
	t.orient = u.prefs.Orientation()

	t.chips = chipRow()
	t.title = boldLabel("")
	t.desc = secondary("")
	t.unsaved = widget.NewLabel("")
	t.unsaved.Importance = widget.WarningImportance
	t.unsaved.Hide()

	t.view = newPadView(t.orient, t.selectControl)
	t.orientLb = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	rotL := widget.NewButtonWithIcon("", theme.MediaReplayIcon(), func() { t.rotate(t.orient.RotateCCW()) })
	rotR := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { t.rotate(t.orient.RotateCW()) })
	rotL.Importance, rotR.Importance = widget.LowImportance, widget.LowImportance
	rotate := container.NewCenter(container.NewHBox(rotL, vcenter(t.orientLb), rotR))

	stageHead := container.NewBorder(nil, nil, nil, vcenter(t.unsaved), tight(t.title, t.desc))
	t.caption = text("", 13, colAccent, true)
	t.caption.Alignment = fyne.TextAlignCenter
	t.liveHint = secondary(liveOffHint)
	t.liveHint.Alignment = fyne.TextAlignCenter
	t.liveHint.Wrapping = fyne.TextWrapWord
	if u.deps.Input != nil {
		t.liveHint.Hide()
	}
	t.warning = wrapLabel("")
	t.warning.Importance = widget.WarningImportance
	t.warning.Hide()
	t.monitor = container.NewVBox()
	t.monitorBox = container.NewVBox(text("Recognised", 12, colText2, true), t.monitor)
	t.monitorBox.Hide()
	live := container.NewVBox(container.NewCenter(t.caption), t.liveHint)
	stage := surface(container.NewBorder(stageHead, container.NewVBox(live, t.monitorBox, rotate, t.warning, secondary(layersNote)), nil, nil,
		inset(8, 8, 0, 0, t.view.content)))

	t.inspTitle = boldLabel("")
	t.inspSub = secondary("")
	t.slotsBox = container.NewVBox()
	picks := chipRow()
	for _, a := range quickPicks {
		a := a
		picks.Add(newChip(a, true, func() { t.quickPick(a) }))
	}
	t.message = wrapLabel("")
	t.message.Hide()

	t.saveBtn = widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), t.save)
	t.saveBtn.Importance = widget.HighImportance
	t.resetBtn = widget.NewButtonWithIcon("Reset", theme.ContentUndoIcon(), t.confirmReset)
	t.applyBtn = widget.NewButtonWithIcon("Apply to pad", theme.UploadIcon(), t.apply)
	t.applyBtn.Importance = widget.SuccessImportance
	buttons := container.NewVBox(container.NewGridWithColumns(2, t.saveBtn, t.resetBtn), t.applyBtn)

	inspector := surface(container.NewVBox(
		tight(t.inspTitle, t.inspSub),
		t.slotsBox,
		text("Quick picks", 12, colText2, true),
		picks,
		t.message,
		widget.NewSeparator(),
		buttons,
	))

	body := container.NewVBox(
		screenTitle("Profiles", "Pick a profile, click a key or the knob, choose what it sends."),
		t.chips,
		newTwoPane(stage, container.NewVBox(inspector)),
	)
	t.content = screen(body)

	t.showOrientation()
	t.reload("")
	t.selectControl(ctlKey1)
	return t
}

// reload re-reads profiles from the store, keeping the selection by name.
func (t *profilesTab) reload(keep string) {
	list, err := t.u.deps.Store.List()
	if err != nil {
		t.showError("Could not load profiles: " + err.Error())
		return
	}
	t.profiles = list
	t.chips.Objects = nil
	idx := 0
	for i, p := range list {
		i := i
		c := newChip(p.Name, false, func() { t.selectProfile(i) })
		c.setBadge(t.u.deps.Store.IsOverridden(p.Name))
		t.chips.Add(c)
		if p.Name == keep {
			idx = i
		}
	}
	t.chips.Refresh()
	if t.u.tray != nil {
		t.u.tray.rebuild()
	}
	if len(list) == 0 {
		t.selected = -1
		return
	}
	t.selectProfile(idx)
}

func (t *profilesTab) selectProfile(i int) {
	if i < 0 || i >= len(t.profiles) {
		return
	}
	t.selected = i
	for j, o := range t.chips.Objects {
		o.(*chip).setSelected(j == i)
	}
	p, err := t.u.deps.Store.Get(t.profiles[i].Name)
	if err != nil {
		p = t.profiles[i]
	}
	t.stored, t.draft = p, p
	t.title.SetText(p.Name)
	t.desc.SetText(p.Description)
	t.syncView()
	t.loadInspector()
	t.hideMessage()
}

// selectControl shows control c in the inspector.
func (t *profilesTab) selectControl(c controlID) {
	t.control = c
	t.view.setSelected(c)
	t.loadInspector()
}

// loadInspector rebuilds the slot editors for the selected control.
func (t *profilesTab) loadInspector() {
	t.loading = true
	defer func() { t.loading = false }()
	t.inspTitle.SetText(t.control.title())
	modified := ""
	if t.u.deps.Store.IsOverridden(t.draft.Name) {
		modified = " · edited"
	}
	t.inspSub.SetText("Layer 1 · " + t.draft.Name + modified)
	t.slotsBox.Objects = nil
	clear(t.entries)
	slots := slotsFor(t.control, t.draft, t.gestureMode)
	for _, s := range slots {
		s := s
		if s.Section != "" {
			t.slotsBox.Add(inset(6, 0, 0, 0, text(s.Section, 13, colText, true)))
		}
		opts := profile.SupportedKeys()
		if s.ID != slotAction {
			opts = gestureOptions()
		}
		e := widget.NewSelectEntry(opts)
		e.SetPlaceHolder(s.Placeholder)
		e.SetText(getAction(t.draft, t.control, s.ID))
		e.OnChanged = func(v string) {
			t.activeSlot = s.ID
			if !t.loading {
				t.edit(s.ID, v)
			}
		}
		t.entries[s.ID] = e
		t.slotsBox.Add(container.NewVBox(text(s.Label, 12, colText2, true), e))
	}
	t.activeSlot = slots[0].ID
	t.slotsBox.Refresh()
}

// entry returns the editor of the active slot.
func (t *profilesTab) entry() *widget.SelectEntry { return t.entries[t.activeSlot] }

func (t *profilesTab) quickPick(a string) {
	if e := t.entry(); e != nil {
		e.SetText(a)
	}
}

// edit applies one slot change to the draft and validates it.
func (t *profilesTab) edit(slot, v string) {
	t.draft = setAction(t.draft, t.control, slot, v)
	t.syncView()
	if err := profile.Validate(t.draft); err != nil {
		t.showError(err.Error())
	} else {
		t.hideMessage()
	}
}

// syncView updates keycap labels and the unsaved marker from the draft.
func (t *profilesTab) syncView() {
	for _, id := range allControls {
		t.view.controls[id].setAction(getAction(t.draft, id, slotAction))
	}
	if !reflect.DeepEqual(t.draft, t.stored) {
		t.unsaved.SetText("Unsaved")
		t.unsaved.Show()
	} else {
		t.unsaved.Hide()
	}
	t.showWarning()
}

// showWarning shows which controls plain-mode live input cannot tell apart.
func (t *profilesTab) showWarning() {
	w := ""
	if !t.gestureMode {
		w = collisionWarning(t.draft, t.orient)
	}
	if w == "" {
		t.warning.Hide()
		return
	}
	t.warning.SetText(w + ".")
	t.warning.Show()
}

const liveOffHint = "Live input is off. Enable live input (needs input permission) to see presses here."

// setMode switches the editor between plain and gesture slots. UI goroutine.
func (t *profilesTab) setMode(m Mode) {
	on := m == ModeGesture
	if on == t.gestureMode {
		return
	}
	t.gestureMode = on
	if on {
		t.monitorBox.Show()
	} else {
		t.monitorBox.Hide()
	}
	t.loadInspector()
	t.showWarning()
}

// showLive updates the live input hint. UI goroutine.
func (t *profilesTab) showLive(st liveStatus) {
	switch {
	case t.u.deps.Input != nil || st.Reading:
		t.liveHint.Hide()
		return
	case errors.Is(st.Err, evdev.ErrPermission):
		t.liveHint.SetText(liveOffHint + " Fix: run `just udev-install`, then replug the pad.")
	case st.Err != nil:
		t.liveHint.SetText("Live input is off. " + liveErrMessage(st.Err))
	default:
		t.liveHint.SetText(liveOffHint)
	}
	t.liveHint.Show()
}

// monitorAdd shows a recognised gesture in the live monitor. UI goroutine.
func (t *profilesTab) monitorAdd(it monitorItem) {
	t.monLog.add(it)
	t.refreshMonitor(time.Now())
}

// refreshMonitor redraws the monitor lines, fading older ones. UI goroutine.
func (t *profilesTab) refreshMonitor(now time.Time) {
	items := t.monLog.items
	for len(t.monitor.Objects) < len(items) {
		t.monitor.Add(monoText("", 12, colText))
	}
	for i, o := range t.monitor.Objects {
		txt := o.(*canvas.Text)
		if i >= len(items) {
			txt.Text = ""
			txt.Refresh()
			continue
		}
		c := colText
		if items[i].Err {
			c = colorError
		}
		txt.Text = items[i].Text
		txt.Color = withAlpha(c, monitorAlpha(i, now.Sub(items[i].At)))
		txt.Refresh()
	}
}

func (t *profilesTab) rotate(o profile.Orientation) {
	if o == t.orient {
		return
	}
	t.orient = o
	t.u.prefs.SetOrientation(o)
	t.view.setOrientation(o)
	t.showOrientation()
	t.showWarning()
	if t.u.tray != nil {
		t.u.tray.rebuild()
		t.u.setLiveProfile(t.u.activeProfile())
	}
}

func (t *profilesTab) showOrientation() { t.orientLb.SetText(t.orient.Label()) }

// edited returns the profile as currently shown in the editor.
func (t *profilesTab) edited() profile.Profile { return t.draft }

func (t *profilesTab) save() {
	if t.selected < 0 {
		return
	}
	p := t.edited()
	if err := t.u.deps.Store.Save(p); err != nil {
		t.showError("Not saved: " + err.Error())
		return
	}
	t.reload(p.Name)
	if p.Name == t.u.activeProfile().Name {
		t.u.setLiveProfile(p)
	}
	t.u.toasts.show(toastSuccess, "Saved "+p.Name+".")
}

func (t *profilesTab) confirmReset() {
	if t.selected < 0 {
		return
	}
	name := t.stored.Name
	dialog.ShowConfirm("Reset profile",
		fmt.Sprintf("Discard your changes to %q and restore the built-in keys?", name),
		func(ok bool) {
			if !ok {
				return
			}
			if err := t.u.deps.Store.Reset(name); err != nil {
				t.showError("Reset failed: " + err.Error())
				return
			}
			t.reload(name)
			t.u.toasts.show(toastInfo, "Restored the built-in "+name+" profile.")
		}, t.u.win)
}

// apply uploads the edited profile (saved or not) plus the next two layers.
func (t *profilesTab) apply() {
	if t.selected < 0 {
		return
	}
	t.applyBtn.Disable()
	t.applyBtn.SetText("Writing…")
	t.u.applyProfile(t.edited(), t.profiles, func(names []string, err error) {
		t.applyBtn.Enable()
		t.applyBtn.SetText("Apply to pad")
		if err != nil {
			t.showError("Apply failed: " + writeErrMessage(err))
			if names != nil {
				t.u.showWriteError("Apply failed", err)
			}
			return
		}
		t.hideMessage()
		if names == nil {
			t.u.toasts.show(toastSuccess, "Gesture mode now uses "+t.draft.Name+".")
			return
		}
		t.u.toasts.show(toastSuccess, "Written to the pad. Layers: "+strings.Join(names, ", ")+".")
	})
}

// selectByName shows profile name in the editor unless there are unsaved edits.
func (t *profilesTab) selectByName(name string) {
	if !reflect.DeepEqual(t.draft, t.stored) {
		return
	}
	for i, p := range t.profiles {
		if p.Name == name {
			t.selectProfile(i)
			return
		}
	}
}

func (t *profilesTab) showError(s string) {
	t.message.Importance = widget.DangerImportance
	t.message.SetText(s)
	t.message.Show()
}

func (t *profilesTab) hideMessage() {
	t.message.SetText("")
	t.message.Hide()
}

// feedback animates the drawn pad for one live input event. UI goroutine.
func (t *profilesTab) feedback(ev PadInputEvent) {
	c, ok := inputControl(ev.Control)
	if !ok {
		return
	}
	ctl := t.view.controls[c]
	knob := t.view.controls[ctlKnobPress]
	switch c {
	case ctlKnobCCW, ctlKnobCW:
		ctl.animatePress(ev.Down)
		if ev.Down {
			knob.animateTick(detentAngle(c))
		}
	case ctlKnobPress:
		ctl.animatePress(ev.Down)
		if ev.Down {
			ctl.animatePulse()
		}
	default:
		ctl.animatePress(ev.Down)
	}
	if ev.Down {
		t.showCaption(inputCaption(c))
	}
}

// showCaption shows the "last input" caption and fades it out after ~1.5 s.
func (t *profilesTab) showCaption(s string) {
	stopAnim(t.fade)
	t.caption.Text = s
	t.caption.Color = colAccent
	t.caption.Refresh()
	t.captionGen++
	gen := t.captionGen
	time.AfterFunc(1500*time.Millisecond, func() {
		fyne.Do(func() {
			if gen != t.captionGen {
				return
			}
			t.fade = canvas.NewColorRGBAAnimation(colAccent, withAlpha(colAccent, 0), 400*time.Millisecond, func(c color.Color) {
				t.caption.Color = c
				t.caption.Refresh()
			})
			t.fade.Start()
		})
	})
}

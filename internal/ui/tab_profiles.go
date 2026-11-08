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
	mismatch    string // last "pad holds a different mapping" notice, cleared on apply
	monitor     *fyne.Container
	monitorBox  fyne.CanvasObject
	monLog      gestureLog

	layerLb *widget.Label // "Pad on layer 2 · codex" (plain mode)
	chords  *collapsible  // gesture mode: the three chord slots
	picks   *collapsible  // quick picks
}

// monitorLines is how many lines the Recognised card shows before scrolling.
const monitorLines = 8

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
	monScroll := container.NewVScroll(t.monitor)
	monScroll.SetMinSize(fyne.NewSize(0, monitorLines*18))
	t.monitorBox = surface(container.NewVBox(boldLabel("Recognised"), monScroll))
	t.monitorBox.Hide()
	t.layerLb = secondary("")
	t.layerLb.Alignment = fyne.TextAlignCenter
	t.layerLb.Hide()
	live := container.NewVBox(container.NewCenter(t.caption), t.layerLb, t.liveHint)
	// natural heights, top to bottom: drawing, rotate row, notes
	stage := surface(container.NewVBox(stageHead, inset(8, 0, 0, 0, t.view.content), live, rotate, t.warning, secondary(layersNote)))
	left := container.NewVBox(stage, t.monitorBox)

	t.inspTitle = boldLabel("")
	t.inspSub = secondary("")
	t.slotsBox = container.NewVBox()
	picks := chipRow()
	for _, a := range quickPicks {
		a := a
		picks.Add(newChip(a, true, func() { t.quickPick(a) }))
	}
	t.picks = newCollapsible("Quick picks", true, picks)
	t.chords = newCollapsible(sectionChords, false)
	t.message = wrapLabel("")
	t.message.Hide()

	t.saveBtn = widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), t.save)
	t.saveBtn.Importance = widget.HighImportance
	t.resetBtn = widget.NewButtonWithIcon("Reset", theme.ContentUndoIcon(), t.confirmReset)
	t.applyBtn = widget.NewButtonWithIcon("Apply to pad", theme.UploadIcon(), t.apply)
	t.applyBtn.Importance = widget.SuccessImportance
	var share *widget.Button
	share = widget.NewButtonWithIcon("Share", theme.MailForwardIcon(), func() { t.showShareMenu(share) })
	share.Importance = widget.LowImportance
	buttons := container.NewVBox(container.NewGridWithColumns(2, t.saveBtn, t.resetBtn), t.applyBtn, share)

	// the slots scroll inside the inspector when taller than the window;
	// title and buttons stay put
	slots := cappedScroll(container.NewVBox(t.slotsBox, t.picks.content, t.message), t.inspectorLimit)
	inspector := surface(container.NewBorder(tight(t.inspTitle, t.inspSub),
		container.NewVBox(widget.NewSeparator(), buttons), nil, nil, slots))

	body := container.NewVBox(
		screenTitle("Profiles", "Pick a profile, click a key or the knob, choose what it sends."),
		t.chips,
		newTwoPane(left, inspector),
	)
	t.content = screen(body)

	t.showOrientation()
	t.reload("")
	t.selectControl(ctlKey1)
	return t
}

// reload re-reads profiles from the store, keeping the selection by name.
func (t *profilesTab) reload(keep string) {
	idx, ok := t.relist(keep)
	if !ok {
		return
	}
	if len(t.profiles) == 0 {
		t.selected = -1
		return
	}
	t.selectProfile(idx)
}

// externalReload picks up profile files changed outside the app. Unsaved
// edits in the editor are kept; only the chips are refreshed then. UI goroutine.
func (t *profilesTab) externalReload() {
	if t.selected < 0 || reflect.DeepEqual(t.draft, t.stored) {
		keep := ""
		if t.selected >= 0 {
			keep = t.stored.Name
		}
		t.reload(keep)
		return
	}
	if idx, ok := t.relist(t.stored.Name); ok && idx < len(t.profiles) {
		t.selected = idx
		for j, o := range t.chips.Objects {
			o.(*chip).setSelected(j == idx)
		}
	}
	t.u.toasts.show(toastInfo, "Profiles changed on disk. Save or reset "+t.draft.Name+" to see its file version.")
}

// relist reloads the profile list and chips and returns the index of keep
// (0 when missing). ok is false when the store could not be read.
func (t *profilesTab) relist(keep string) (int, bool) {
	t.u.profileSig = dirSignature(t.u.deps.ProfilesDir)
	list, err := t.u.deps.Store.List()
	if err != nil {
		t.showError("Could not load profiles: " + err.Error())
		return 0, false
	}
	t.profiles = list
	t.chips.Objects = nil
	idx := 0
	for i, p := range list {
		i := i
		c := newChip(p.Name, false, func() { t.selectProfile(i) })
		if editedBuiltin(p.Name, t.u.deps.Store.IsOverridden(p.Name)) {
			c.setTag("edited")
		}
		t.chips.Add(c)
		if p.Name == keep {
			idx = i
		}
	}
	t.chips.Refresh()
	if t.u.tray != nil {
		t.u.tray.rebuild()
	}
	return idx, true
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
	sub := "Layer 1 · " + t.draft.Name
	if editedBuiltin(t.draft.Name, t.u.deps.Store.IsOverridden(t.draft.Name)) {
		sub += "\n" + editedNote
	}
	t.inspSub.SetText(sub)
	t.slotsBox.Objects = nil
	clear(t.entries)
	slots := slotsFor(t.control, t.draft, t.gestureMode)
	var grid *fyne.Container
	var chords []fyne.CanvasObject
	inChords := false
	for _, s := range slots {
		s := s
		if s.Section == sectionChords {
			inChords = true
		} else if s.Section != "" {
			t.slotsBox.Add(inset(6, 0, 0, 0, text(s.Section, 13, colText, true)))
			grid = container.New(&columnsLayout{cols: t.slotColumns, gap: 8})
			t.slotsBox.Add(grid)
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
		cell := container.NewVBox(text(s.Label, 12, colText2, true), e)
		switch {
		case inChords:
			chords = append(chords, cell)
		case grid != nil:
			grid.Add(cell)
		default:
			t.slotsBox.Add(cell)
		}
	}
	if len(chords) > 0 {
		t.chords.setBody(chords...)
		t.slotsBox.Add(t.chords.content)
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

// showWarning shows which controls plain-mode live input cannot tell apart,
// and whether the pad seems to hold a different mapping.
func (t *profilesTab) showWarning() {
	var parts []string
	if !t.gestureMode {
		if t.mismatch != "" {
			parts = append(parts, t.mismatch)
		}
		if w := collisionWarning(t.draft, t.orient); w != "" {
			parts = append(parts, w+".")
		}
	}
	if len(parts) == 0 {
		t.warning.Hide()
		return
	}
	t.warning.SetText(strings.Join(parts, "\n"))
	t.warning.Show()
}

// showMismatch notes that the pad sent a key the active profile never sends.
// UI goroutine.
func (t *profilesTab) showMismatch(key string) {
	t.mismatch = mismatchText(key, t.u.prefs.ActiveProfile())
	t.showWarning()
}

// clearMismatch drops the notice after the pad was programmed. UI goroutine.
func (t *profilesTab) clearMismatch() {
	if t.mismatch != "" {
		t.mismatch = ""
		t.showWarning()
	}
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
	t.picks.setOpen(!on)      // the many gesture slots need the room
	t.showLayer(t.u.padLayer) // layers don't matter in gesture mode
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
	t.monitor.Refresh() // text sizes changed
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

// slotColumns is 2 for the gesture slot grid when the panes sit side by side.
func (t *profilesTab) slotColumns() int {
	if t.u.win != nil && sideBySide(t.u.win.Canvas().Size().Width) {
		return 2
	}
	return 1
}

// inspectorLimit caps the inspector's slot area to what fits in the window
// next to the pad (0 = no cap when stacked).
func (t *profilesTab) inspectorLimit() float32 {
	if t.u.win == nil {
		return 0
	}
	sz := t.u.win.Canvas().Size()
	if !sideBySide(sz.Width) {
		return 0
	}
	return max(sz.Height-inspectorChrome, 240)
}

// inspectorChrome is the height above and below the inspector's slot area:
// header, screen title, chips, inspector title and buttons.
const inspectorChrome = 470

// showLayer shows the guessed pad layer under the drawing in plain mode. UI goroutine.
func (t *profilesTab) showLayer(li layerInfo) {
	if li.Text == "" || t.gestureMode {
		t.layerLb.Hide()
		return
	}
	t.layerLb.SetText(li.Text + " · the side button changes it")
	t.layerLb.Show()
}

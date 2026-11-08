package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/profile"
)

const layersNote = "Apply writes this profile to layer 1 and the next two profiles to layers 2 and 3. " +
	"Switch layers with the pad's side button. Some boards only keep layer 1."

type profilesTab struct {
	u       *appUI
	content fyne.CanvasObject

	profiles []profile.Profile
	selected int // index into profiles, -1 for none
	current  profile.Profile

	list     *widget.List
	title    *widget.Label
	desc     *widget.Label
	keys     [3]*widget.SelectEntry
	ccw      *widget.SelectEntry
	press    *widget.SelectEntry
	cw       *widget.SelectEntry
	message  *widget.Label
	saveBtn  *widget.Button
	resetBtn *widget.Button
	applyBtn *widget.Button
}

func newProfilesTab(u *appUI) *profilesTab {
	t := &profilesTab{u: u, selected: -1}

	t.list = widget.NewList(
		func() int { return len(t.profiles) },
		func() fyne.CanvasObject {
			badge := widget.NewLabel("modified")
			badge.Importance = widget.WarningImportance
			badge.TextStyle = fyne.TextStyle{Italic: true}
			return container.NewBorder(nil, nil, nil, badge, widget.NewLabel("profile"))
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*fyne.Container)
			name := t.profiles[id].Name
			row.Objects[0].(*widget.Label).SetText(name)
			badge := row.Objects[1].(*widget.Label)
			if t.u.deps.Store.IsOverridden(name) {
				badge.Show()
			} else {
				badge.Hide()
			}
		},
	)
	t.list.OnSelected = t.selectProfile

	options := profile.SupportedKeys()
	newField := func() *widget.SelectEntry {
		e := widget.NewSelectEntry(options)
		e.OnChanged = func(string) { t.message.SetText("") }
		return e
	}
	for i := range t.keys {
		t.keys[i] = newField()
	}
	t.ccw, t.press, t.cw = newField(), newField(), newField()

	t.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.desc = wrapLabel("")
	t.message = wrapLabel("")

	t.saveBtn = widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), t.save)
	t.saveBtn.Importance = widget.HighImportance
	t.resetBtn = widget.NewButtonWithIcon("Reset", theme.ContentUndoIcon(), t.confirmReset)
	t.applyBtn = widget.NewButtonWithIcon("Apply to pad", theme.UploadIcon(), t.apply)

	note := wrapLabel(layersNote)
	note.Importance = widget.LowImportance

	editor := container.NewVBox(
		t.title,
		t.desc,
		container.NewCenter(t.padLayout()),
		note,
		t.message,
		container.NewHBox(t.saveBtn, t.resetBtn, layout.NewSpacer(), t.applyBtn),
	)
	split := container.NewHSplit(t.list, container.NewPadded(container.NewVScroll(editor)))
	split.Offset = 0.22
	t.content = split

	t.reload("")
	return t
}

// padLayout draws three square key tiles and a rounded knob tile.
func (t *profilesTab) padLayout() fyne.CanvasObject {
	keys := make([]fyne.CanvasObject, 0, 4)
	for i, e := range t.keys {
		keys = append(keys, tile(10, container.NewVBox(
			layout.NewSpacer(),
			widget.NewLabelWithStyle(fmt.Sprintf("Key %d", i+1), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			e,
			layout.NewSpacer(),
		)))
	}
	keyRow := container.NewGridWrap(fyne.NewSize(150, 150), keys...)

	knob := tile(60, container.NewVBox(
		widget.NewLabelWithStyle("Knob", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("⟲", t.ccw),
			widget.NewFormItem("press", t.press),
			widget.NewFormItem("⟳", t.cw),
		),
	))
	knobBox := container.NewGridWrap(fyne.NewSize(240, 230), knob)
	return container.NewHBox(keyRow, knobBox)
}

// tile puts content on a rounded, outlined background.
func tile(radius float32, content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = radius
	return container.NewStack(bg, container.NewPadded(container.NewPadded(content)))
}

// reload re-reads profiles from the store, keeping the selection by name.
func (t *profilesTab) reload(keep string) {
	list, err := t.u.deps.Store.List()
	if err != nil {
		t.showError("Could not load profiles: " + err.Error())
		return
	}
	t.profiles = list
	t.list.Refresh()
	idx := 0
	for i, p := range list {
		if p.Name == keep {
			idx = i
		}
	}
	if len(list) == 0 {
		t.selected = -1
		return
	}
	if idx == t.selected {
		t.selectProfile(idx) // OnSelected won't fire for the same row
	}
	t.list.Select(idx)
}

func (t *profilesTab) selectProfile(id widget.ListItemID) {
	if id < 0 || id >= len(t.profiles) {
		return
	}
	t.selected = id
	p, err := t.u.deps.Store.Get(t.profiles[id].Name)
	if err != nil {
		p = t.profiles[id]
	}
	t.current = p
	t.title.SetText(p.Name)
	t.desc.SetText(p.Description)
	for i, e := range t.keys {
		e.SetText(p.Keys[i])
	}
	t.ccw.SetText(p.Knob.CCW)
	t.press.SetText(p.Knob.Press)
	t.cw.SetText(p.Knob.CW)
	t.message.SetText("")
}

// edited returns the profile as currently shown in the editor.
func (t *profilesTab) edited() profile.Profile {
	return profileFromControls(t.current,
		[3]string{t.keys[0].Text, t.keys[1].Text, t.keys[2].Text},
		t.ccw.Text, t.press.Text, t.cw.Text)
}

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
	t.showInfo("Saved " + p.Name + ".")
}

func (t *profilesTab) confirmReset() {
	if t.selected < 0 {
		return
	}
	name := t.current.Name
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
			t.showInfo("Restored the built-in " + name + " profile.")
		}, t.u.win)
}

// apply uploads the edited profile (saved or not) plus the next two layers.
func (t *profilesTab) apply() {
	if t.selected < 0 {
		return
	}
	p := t.edited()
	if err := profile.Validate(p); err != nil {
		t.showError("Fix this first: " + err.Error())
		return
	}
	layers := profile.Layers(replaceProfile(t.profiles, p), p.Name)
	doc, err := profile.MappingYAML(layers)
	if err != nil {
		t.showError("Could not build the mapping: " + err.Error())
		return
	}
	names := make([]string, len(layers))
	for i, l := range layers {
		names[i] = fmt.Sprintf("%d = %s", i+1, l.Name)
	}
	t.applyBtn.Disable()
	t.showInfo("Writing to the pad…")
	go func() {
		err := t.u.writer.Upload(doc)
		fyne.Do(func() {
			t.applyBtn.Enable()
			if err != nil {
				t.showError("Apply failed: " + writeErrMessage(err))
				t.u.showWriteError("Apply failed", err)
				return
			}
			t.showInfo("Written to the pad. Layers: " + strings.Join(names, ", ") + ".")
		})
	}()
}

func (t *profilesTab) showError(s string) {
	t.message.Importance = widget.DangerImportance
	t.message.SetText(s)
}

func (t *profilesTab) showInfo(s string) {
	t.message.Importance = widget.SuccessImportance
	t.message.SetText(s)
}

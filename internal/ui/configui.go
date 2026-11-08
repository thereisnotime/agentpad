package ui

// Settings export / import / backups (About screen) and the settings.yaml
// error pill.

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/settings"
)

// showConfigError shows (err != nil) or clears the settings.yaml pill. UI goroutine.
func (u *appUI) showConfigError(err error) {
	u.configErr = err
	if err == nil {
		if u.pillConfig.Visible() {
			u.pillConfig.Hide()
			u.toasts.show(toastSuccess, "settings.yaml is valid again.")
		}
		return
	}
	u.pillConfig.set(colorError, configPillText(err))
	u.pillConfig.Show()
	u.toasts.show(toastError, "settings.yaml: "+err.Error()+" Keeping the last good settings.")
}

// explainConfigError is the pill's tap action. UI goroutine.
func (u *appUI) explainConfigError() {
	if u.configErr == nil {
		return
	}
	msg := errorText(u.configErr) + "\n\nagentpad keeps running on the last good settings until the file is fixed."
	body := container.NewVBox(wrapLabel(msg))
	d := dialog.NewCustom("settings.yaml has a problem", "Close", fixedWidth(480, body), u.win)
	if u.files != nil {
		path := u.files.Path()
		edit := widget.NewButtonWithIcon("Open settings.yaml", theme.DocumentIcon(), func() {
			if err := u.openFile(path); err != nil {
				u.toasts.show(toastError, "Could not open the editor: "+err.Error())
			}
		})
		body.Add(edit)
	}
	d.Show()
}

// fixedWidth gives o a minimum width (dialogs size to their content).
func fixedWidth(w float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewStack(fixed(w, 0), o)
}

// settingsUI is the Settings card on the About screen.
type settingsUI struct {
	u          *appUI
	status     *widget.Label
	restoreBtn *widget.Button
	lastBackup string
}

func newSettingsCard(u *appUI) fyne.CanvasObject {
	s := &settingsUI{u: u, status: wrapLabel("")}
	export := widget.NewButtonWithIcon("Export…", theme.DocumentSaveIcon(), s.export)
	imp := widget.NewButtonWithIcon("Import…", theme.FolderOpenIcon(), s.importBundle)
	backups := widget.NewButtonWithIcon("Backups ▸", theme.HistoryIcon(), s.showBackups)
	folder := widget.NewButtonWithIcon("Open settings folder", theme.FolderIcon(), s.openFolder)
	s.restoreBtn = widget.NewButtonWithIcon("Restore backup", theme.ContentUndoIcon(), func() {
		s.confirmRestore(s.lastBackup)
	})
	s.restoreBtn.Importance = widget.LowImportance
	s.status.Hide()
	s.restoreBtn.Hide()
	sub := "Everything lives in settings.yaml and profiles/. Export it to another machine, or import a bundle with a preview first."
	if u.files == nil {
		if u.deps.SettingsWarning != "" {
			sub = "settings.yaml could not be opened, so export and import are off. Preferences are kept by the desktop toolkit for now."
		}
		for _, b := range []*widget.Button{export, imp, backups, folder} {
			b.Disable()
		}
	}
	return newCard("Settings", sub, container.NewVBox(
		chipRow(export, imp, backups, folder),
		container.NewBorder(nil, nil, nil, s.restoreBtn, s.status),
	))
}

func (s *settingsUI) exportedBy() string { return "agentpad " + s.u.deps.Version }

// export asks what to include, then where to save the bundle.
func (s *settingsUI) export() {
	all := widget.NewCheck("Include built-in profiles", nil)
	body := container.NewVBox(
		wrapLabel("The bundle holds settings.yaml, the profiles you changed or added and rules.yaml if you have one."),
		all,
	)
	dialog.ShowCustomConfirm("Export settings", "Choose file…", "Cancel", fixedWidth(420, body), func(ok bool) {
		if ok {
			s.exportTo(all.Checked)
		}
	}, s.u.win)
}

func (s *settingsUI) exportTo(allProfiles bool) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, s.u.win)
			return
		}
		if w == nil {
			return // cancelled
		}
		var buf bytes.Buffer
		err = s.u.files.Export(&buf, settings.ExportOptions{ExportedBy: s.exportedBy(), AllProfiles: allProfiles})
		if err == nil {
			_, err = w.Write(buf.Bytes())
		}
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			dialog.ShowError(fmt.Errorf("export failed: %w", err), s.u.win)
			return
		}
		s.u.toasts.show(toastSuccess, "Exported to "+w.URI().Name()+".")
	}, s.u.win)
	d.SetFileName(exportFileName(time.Now()))
	d.SetFilter(storage.NewExtensionFileFilter([]string{".yaml", ".yml"}))
	d.Show()
}

// importBundle picks a bundle, validates it and shows the merge preview.
func (s *settingsUI) importBundle() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, s.u.win)
			return
		}
		if r == nil {
			return
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			dialog.ShowError(err, s.u.win)
			return
		}
		s.previewBundle(data)
	}, s.u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".yaml", ".yml"}))
	d.Show()
}

// previewBundle parses and plans data; nothing is applied on any error.
func (s *settingsUI) previewBundle(data []byte) {
	b, err := settings.ParseBundle(bytes.NewReader(data))
	if err != nil {
		s.showImportError(err)
		return
	}
	s.u.files.SetModel(s.u.device.ProductID)
	plan, err := s.u.files.PlanImport(b, settings.Merge)
	if err != nil {
		s.showImportError(err)
		return
	}
	s.showPlan(b, plan)
}

func (s *settingsUI) showImportError(err error) {
	body := wrapLabel(errorText(err) + "\n\nNothing was imported.")
	dialog.NewCustom("Import refused", "Close", fixedWidth(480, body), s.u.win).Show()
}

// showPlan shows the preview of plan with Merge / Replace / Cancel (merge
// plan) or Replace / Back (replace plan).
func (s *settingsUI) showPlan(b *settings.Bundle, plan *settings.Plan) {
	head := "Merge: values in the bundle are added or updated, everything else stays."
	if plan.Mode == settings.Replace {
		head = "Replace: settings start from defaults, profile overrides not in the bundle are removed."
	}
	from := ""
	if b.ExportedBy != "" {
		from = "From " + b.ExportedBy
		if !b.ExportedAt.IsZero() {
			from += ", " + b.ExportedAt.Local().Format("2006-01-02 15:04")
		}
		from += ". "
	}
	body := container.NewVBox(wrapLabel(from+head), boldLabel(planSummary(plan)))
	for _, g := range planPreview(plan) {
		body.Add(text(g.Title, 13, colText2, true))
		for _, r := range g.Rows {
			body.Add(previewLine(r))
		}
	}
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(520, 320))

	d := dialog.NewCustomWithoutButtons("Import preview", scroll, s.u.win)
	cancel := widget.NewButton("Cancel", d.Hide)
	var buttons []fyne.CanvasObject
	if plan.Mode == settings.Merge {
		merge := widget.NewButtonWithIcon("Merge", theme.ConfirmIcon(), func() { d.Hide(); s.apply(plan) })
		merge.Importance = widget.HighImportance
		if plan.Empty() {
			merge.Disable()
		}
		replace := widget.NewButton("Replace…", func() {
			rp, err := s.u.files.PlanImport(b, settings.Replace)
			d.Hide()
			if err != nil {
				s.showImportError(err)
				return
			}
			s.showPlan(b, rp)
		})
		buttons = []fyne.CanvasObject{cancel, replace, merge}
	} else {
		back := widget.NewButton("Back", func() {
			d.Hide()
			if mp, err := s.u.files.PlanImport(b, settings.Merge); err == nil {
				s.showPlan(b, mp)
			}
		})
		replace := widget.NewButtonWithIcon("Replace", theme.WarningIcon(), func() { d.Hide(); s.apply(plan) })
		replace.Importance = widget.DangerImportance
		if plan.Empty() {
			replace.Disable()
		}
		buttons = []fyne.CanvasObject{cancel, back, replace}
	}
	d.SetButtons(buttons)
	d.Show()
}

// previewLine draws one preview row: a coloured marker, the path and detail.
func previewLine(r previewRow) fyne.CanvasObject {
	marks := map[rowKind]struct {
		sym string
		c   color.Color
	}{
		rowAdd: {"+", colorSuccess}, rowChange: {"~", colorBusy},
		rowRemove: {"−", colorError}, rowWarning: {"!", colorAttention},
	}
	m := marks[r.Kind]
	mark := monoText(m.sym, 14, m.c)
	t := monoText(r.Text, 13, m.c)
	lines := container.NewVBox(t)
	if r.Detail != "" {
		d := secondary(r.Detail)
		d.Wrapping = fyne.TextWrapWord
		lines = container.NewVBox(t, d)
	}
	return container.NewBorder(nil, nil, inset(2, 0, 4, 8, mark), nil, lines)
}

// apply backs up and applies plan, then reloads everything.
func (s *settingsUI) apply(plan *settings.Plan) {
	name, err := s.u.files.Apply(plan)
	if err != nil {
		msg := "Import failed: " + errorText(err)
		if name != "" {
			msg += "\n\nThe state before the import is in backup " + name + "."
		}
		dialog.ShowError(fmt.Errorf("%s", msg), s.u.win)
		s.u.configReloaded()
		return
	}
	s.u.configReloaded()
	s.lastBackup = name
	s.status.SetText("Imported · backup " + name)
	s.status.Show()
	s.restoreBtn.Show()
	s.u.toasts.show(toastSuccess, "Imported · backup "+name+".")
}

// showBackups lists backups, newest first, each with Restore.
func (s *settingsUI) showBackups() {
	list, err := s.u.files.Backups()
	if err != nil {
		dialog.ShowError(err, s.u.win)
		return
	}
	body := container.NewVBox()
	if len(list) == 0 {
		body.Add(wrapLabel("No backups yet. agentpad takes one before every import and restore."))
	}
	var d *dialog.CustomDialog
	for i, b := range list {
		if i == 20 {
			body.Add(secondary(fmt.Sprintf("…and %d older, in the backups folder.", len(list)-20)))
			break
		}
		name := b.Name
		restore := widget.NewButtonWithIcon("Restore", theme.ContentUndoIcon(), func() {
			d.Hide()
			s.confirmRestore(name)
		})
		restore.Importance = widget.LowImportance
		body.Add(container.NewBorder(nil, nil, nil, restore, tight(monoText(name, 13, colText), secondary(backupLabel(b)))))
	}
	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(460, min(float32(60*max(len(list), 1)), 360)))
	d = dialog.NewCustom("Backups", "Close", scroll, s.u.win)
	d.Show()
}

func (s *settingsUI) confirmRestore(name string) {
	if name == "" {
		return
	}
	dialog.ShowConfirm("Restore backup",
		fmt.Sprintf("Put back settings.yaml, profiles and rules.yaml from %s?\n\nThe current state is backed up first, so this can be undone.", name),
		func(ok bool) {
			if !ok {
				return
			}
			safety, err := s.u.files.Restore(name)
			s.u.configReloaded()
			if err != nil {
				dialog.ShowError(fmt.Errorf("restore failed: %s", errorText(err)), s.u.win)
				return
			}
			s.lastBackup = safety
			s.status.SetText("Restored " + name + " · previous state in backup " + safety)
			s.status.Show()
			s.restoreBtn.Show()
			s.u.toasts.show(toastSuccess, "Restored "+name+".")
		}, s.u.win)
}

func (s *settingsUI) openFolder() {
	if err := s.u.openFile(s.u.files.Dir()); err != nil {
		s.u.toasts.show(toastError, "Could not open the folder: "+err.Error())
	}
}

// configReloaded re-reads profiles and settings after an import or
// restore. UI goroutine.
func (u *appUI) configReloaded() {
	keep := ""
	if u.profiles.selected >= 0 {
		keep = u.profiles.stored.Name
	}
	u.profiles.reload(keep)
	u.settingsChanged()
}

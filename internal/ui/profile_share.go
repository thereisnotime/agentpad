package ui

// Profiles screen "Share" menu: copy / save a single profile, import one
// from the clipboard or a file.

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/settings"
)

func (t *profilesTab) showShareMenu(anchor fyne.CanvasObject) {
	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Copy as text", t.shareCopy),
		fyne.NewMenuItem("Save to file…", t.shareSave),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Import from clipboard", t.importFromClipboard),
		fyne.NewMenuItem("Import from file…", t.importFromFile),
	)
	widget.ShowPopUpMenuAtRelativePosition(menu, t.u.win.Canvas(), fyne.NewPos(0, anchor.Size().Height), anchor)
}

// shareProfile is the profile as shown in the editor, if it is valid.
func (t *profilesTab) shareProfile() ([]byte, string, bool) {
	if t.selected < 0 {
		return nil, "", false
	}
	p := t.edited()
	if err := profile.Validate(p); err != nil {
		t.showError("Fix this before sharing: " + err.Error())
		return nil, "", false
	}
	data, err := settings.ExportProfile(p)
	if err != nil {
		t.showError("Could not export: " + err.Error())
		return nil, "", false
	}
	return data, p.Name, true
}

func (t *profilesTab) shareCopy() {
	data, name, ok := t.shareProfile()
	if !ok {
		return
	}
	t.u.app.Clipboard().SetContent(string(data))
	t.u.toasts.show(toastSuccess, "Copied "+name+" as text.")
}

func (t *profilesTab) shareSave() {
	data, name, ok := t.shareProfile()
	if !ok {
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, t.u.win)
			return
		}
		if w == nil {
			return
		}
		_, err = w.Write(data)
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			dialog.ShowError(fmt.Errorf("save failed: %w", err), t.u.win)
			return
		}
		t.u.toasts.show(toastSuccess, "Saved "+name+" to "+w.URI().Name()+".")
	}, t.u.win)
	d.SetFileName(profileFileName(name))
	d.SetFilter(storage.NewExtensionFileFilter([]string{".yaml", ".yml"}))
	d.Show()
}

func (t *profilesTab) importFromClipboard() {
	s := t.u.app.Clipboard().Content()
	if strings.TrimSpace(s) == "" {
		t.u.toasts.show(toastInfo, "The clipboard is empty. Copy a profile first (Share > Copy as text).")
		return
	}
	t.previewImport([]byte(s))
}

func (t *profilesTab) importFromFile() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, t.u.win)
			return
		}
		if r == nil {
			return
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			dialog.ShowError(err, t.u.win)
			return
		}
		t.previewImport(data)
	}, t.u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".yaml", ".yml"}))
	d.Show()
}

// previewImport validates a shared profile and asks before saving it.
func (t *profilesTab) previewImport(data []byte) {
	p, err := settings.ParseProfile(data)
	if err != nil {
		body := wrapLabel(errorText(err) + "\n\nNothing was imported.")
		dialog.NewCustom("Profile not imported", "Close", fixedWidth(460, body), t.u.win).Show()
		return
	}
	lines := []string{"Name: " + p.Name}
	if p.Description != "" {
		lines = append(lines, "Description: "+p.Description)
	}
	lines = append(lines, "Keys: "+strings.Join(p.Keys[:], ", "),
		fmt.Sprintf("Knob: %s / %s / %s", p.Knob.CCW, p.Knob.Press, p.Knob.CW))
	if t.u.deps.Store.IsOverridden(p.Name) {
		lines = append(lines, "", "You already have a changed "+p.Name+"; a different one is saved as a new name.")
	}
	if t.selected >= 0 && !reflect.DeepEqual(t.draft, t.stored) {
		lines = append(lines, "", "Unsaved edits to "+t.draft.Name+" will be discarded.")
	}
	body := container.NewVBox(wrapLabel(strings.Join(lines, "\n")))
	dialog.ShowCustomConfirm("Import profile", "Import", "Cancel", fixedWidth(460, body), func(ok bool) {
		if ok {
			t.importProfile(data)
		}
	}, t.u.win)
}

// importProfile saves a shared profile (renaming on conflict) and selects it.
func (t *profilesTab) importProfile(data []byte) {
	msg, saved, err := importShared(data, t.u.deps.Store)
	if err != nil {
		t.showError("Import failed: " + errorText(err))
		return
	}
	t.stored = t.draft // the import replaces whatever was being edited
	t.reload(saved)
	t.u.toasts.show(toastSuccess, msg)
}

// importShared imports data into store and describes the result.
func importShared(data []byte, store profile.Store) (msg, saved string, err error) {
	p, err := settings.ParseProfile(data)
	if err != nil {
		return "", "", err
	}
	existed := store.IsOverridden(p.Name)
	got, err := settings.ImportProfile(data, store)
	if err != nil {
		return "", "", err
	}
	return shareImportMessage(p.Name, got.Name, existed && got.Name == p.Name), got.Name, nil
}

package ui

// "Edit file" on the Setup screen, without Fyne. agentpad only ever opens
// these files in the user's editor or creates an empty one on request; it
// never writes snippet content into them.

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// editStep is what "Edit file" does for a target path.
type editStep int

const (
	editOpen       editStep = iota // the file exists: open it
	editCreate                     // missing: confirm, create it empty, then open it
	editUdevHint                   // read-only system file is missing: explain `just udev-install`
	editImpossible                 // no usable path
)

// editTarget is a snippet's target file.
type editTarget struct {
	Path     string // may start with ~/
	ReadOnly bool   // system file agentpad never creates (the udev rule)
}

// expandHome replaces a leading ~/ with home.
func expandHome(p, home string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok && home != "" {
		return filepath.Join(home, rest)
	}
	return p
}

// planEdit decides what "Edit file" does: the expanded path and the step.
func planEdit(t editTarget, home string, exists func(string) bool) (string, editStep) {
	p := expandHome(t.Path, home)
	if p == "" || !filepath.IsAbs(p) {
		return p, editImpossible
	}
	switch {
	case exists(p):
		return p, editOpen
	case t.ReadOnly:
		return p, editUdevHint
	}
	return p, editCreate
}

// fileExists reports whether p exists (any error but not-exist counts as
// existing, so agentpad never tries to create over something it can't see).
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return !errors.Is(err, fs.ErrNotExist)
}

// createEmpty creates the parent directories and an empty file (0644). It
// never overwrites: an existing file is an error.
func createEmpty(p string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// xdgOpen opens p in the user's default application, detached.
func xdgOpen(p string) error {
	cmd := exec.Command("xdg-open", p)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap it
	return nil
}

// Package desktopentry installs the freedesktop .desktop file and icons so
// desktop shells (KDE, GNOME, ...) show agentpad's icon in the taskbar and
// window decorations. On Wayland this is the only way: the compositor maps the
// window's app_id (config.AppID) to <app_id>.desktop and takes its Icon.
package desktopentry

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thereisnotime/agentpad/internal/config"
)

//go:embed icon.svg
var iconSVG []byte

//go:embed icon-256.png
var iconPNG []byte

// IconSVG returns the scalable app icon.
func IconSVG() []byte { return iconSVG }

// Paths are the files Ensure writes, relative to a data home.
type Paths struct {
	Desktop string
	SVG     string
	PNG     string
}

// PathsIn returns the install locations under dataHome (e.g. ~/.local/share).
func PathsIn(dataHome string) Paths {
	return Paths{
		Desktop: filepath.Join(dataHome, "applications", config.AppID+".desktop"),
		SVG:     filepath.Join(dataHome, "icons", "hicolor", "scalable", "apps", config.AppID+".svg"),
		PNG:     filepath.Join(dataHome, "icons", "hicolor", "256x256", "apps", config.AppID+".png"),
	}
}

// DataHome returns $XDG_DATA_HOME or ~/.local/share.
func DataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

// Entry renders the .desktop file for the binary at exec.
func Entry(exec string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=agentpad",
		"GenericName=Macro pad controller",
		"Comment=Physical approve/reject controller for AI coding agents",
		"Exec=" + quoteExec(exec),
		"Icon=" + config.AppID,
		"Terminal=false",
		"Categories=Utility;Development;",
		"Keywords=macropad;claude;codex;opencode;ch57x;",
		"StartupWMClass=" + config.AppID,
		"StartupNotify=true",
		"",
	}, "\n")
}

// quoteExec quotes a path for the Exec key per the Desktop Entry spec.
func quoteExec(p string) string {
	if !strings.ContainsAny(p, " \t\"`$\\") {
		return p
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(p) + `"`
}

// Ensure writes the desktop entry and icons under dataHome when missing or
// outdated (different Exec or icon bytes). It reports whether anything changed.
// Errors are returned but callers may ignore them: the app works without it.
func Ensure(dataHome, exec string) (changed bool, err error) {
	p := PathsIn(dataHome)
	files := []struct {
		path string
		data []byte
	}{
		{p.Desktop, []byte(Entry(exec))},
		{p.SVG, iconSVG},
		{p.PNG, iconPNG},
	}
	for _, f := range files {
		cur, rerr := os.ReadFile(f.path)
		if rerr == nil && bytes.Equal(cur, f.data) {
			continue
		}
		if err := writeAtomic(f.path, f.data); err != nil {
			return changed, fmt.Errorf("install %s: %w", f.path, err)
		}
		changed = true
	}
	return changed, nil
}

// Remove deletes the installed entry and icons (missing files are fine).
func Remove(dataHome string) error {
	p := PathsIn(dataHome)
	for _, f := range []string{p.Desktop, p.SVG, p.PNG} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agentpad-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

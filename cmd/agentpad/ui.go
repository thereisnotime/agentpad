package main

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/app"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/ui"
)

// runUI starts the desktop app and blocks until it quits.
func runUI() {
	ui.Run(app.NewWithID(config.AppID), ui.Deps{
		Writer:  pad.NewToolWriter(),
		Store:   profile.NewFileStore(filepath.Join(config.ConfigDir(), "profiles")),
		Bin:     selfPath(),
		Version: version,
		LogPath: filepath.Join(config.StateDir(), "events.jsonl"),
		Addr:    config.ListenAddr,
	})
}

// selfPath returns the resolved absolute path of this binary, or "agentpad".
func selfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "agentpad"
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		return real
	}
	return exe
}

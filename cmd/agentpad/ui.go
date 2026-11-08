package main

import (
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2/app"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/ui"
)

// runUI starts the desktop app and blocks until it quits.
func runUI() {
	var input ui.PadInput
	if os.Getenv("AGENTPAD_FAKE_INPUT") == "1" {
		// dev only: replay a demo sequence for the live feedback animations
		input = ui.NewFakeInput(700 * time.Millisecond)
	}
	ui.Run(app.NewWithID(config.AppID), ui.Deps{
		Input:   input,
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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/settings"
	"github.com/thereisnotime/agentpad/internal/ui"
)

// runUI starts the desktop app and blocks until it quits.
func runUI() {
	var input ui.PadInput
	if os.Getenv("AGENTPAD_FAKE_INPUT") == "1" {
		// dev only: replay a demo sequence for the live feedback animations
		input = ui.NewFakeInput(700 * time.Millisecond)
	}
	a := app.NewWithID(config.AppID)

	// SIGINT/SIGTERM quit the app the normal way, so ui.Run's shutdown leaves
	// gesture mode and puts the plain mapping back on the pad. A second
	// signal (or a shutdown that hangs) still ends the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	finished, watched := make(chan struct{}), make(chan struct{})
	defer func() {
		close(finished)
		<-watched
		stop()
	}()
	go func() {
		defer close(watched)
		select {
		case <-ctx.Done():
			stop()
			time.AfterFunc(15*time.Second, func() { os.Exit(130) })
			fyne.Do(a.Quit)
		case <-finished:
		}
	}()

	deps := ui.Deps{
		Input: input,
		// dev only: AGENTPAD_LIVE_INPUT=0 never opens the pad's input devices
		Live:    ui.LiveDevices{Disabled: os.Getenv("AGENTPAD_LIVE_INPUT") == "0"},
		Writer:  pad.NewToolWriter(),
		Bin:     selfPath(),
		Version: version,
		LogPath: filepath.Join(config.StateDir(), "events.jsonl"),
		Addr:    config.ListenAddr,
	}
	openSettings(a, &deps)
	ui.Run(a, deps)
}

// openSettings wires settings.yaml into deps, migrating the old Fyne
// preferences the first time the file is created. When the file can't be
// opened at all, the Fyne preferences stay in use and the UI shows why.
func openSettings(a fyne.App, deps *ui.Deps) {
	dir := config.ConfigDir()
	store, err := settings.Open(dir)
	var ve *settings.ValidationError
	if store == nil || err != nil && !errors.As(err, &ve) {
		fmt.Fprintln(os.Stderr, "agentpad: settings.yaml unavailable, using toolkit preferences:", err)
		deps.Store = profile.NewFileStore(filepath.Join(dir, "profiles"))
		deps.ProfilesDir = filepath.Join(dir, "profiles")
		deps.SettingsWarning = fmt.Sprintf("settings.yaml could not be opened (%v). Using the old preferences; changes won't reach settings.yaml.", err)
		return
	}
	if err == nil {
		if applied, merr := store.Migrate(ui.LegacyPrefs(a.Preferences())); merr != nil {
			fmt.Fprintln(os.Stderr, "agentpad: migrating preferences:", merr)
		} else if applied {
			fmt.Fprintln(os.Stderr, "agentpad: copied the old preferences into", store.Path())
		}
	}
	deps.Settings = ui.NewFileSettings(store, err)
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

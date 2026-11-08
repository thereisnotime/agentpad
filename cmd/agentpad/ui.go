package main

import (
	"context"
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

	ui.Run(a, ui.Deps{
		Input: input,
		// dev only: AGENTPAD_LIVE_INPUT=0 never opens the pad's input devices
		Live:    ui.LiveDevices{Disabled: os.Getenv("AGENTPAD_LIVE_INPUT") == "0"},
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

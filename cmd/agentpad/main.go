// Command agentpad is a desktop controller for CH57x macro pads and AI coding agents.
//
//	agentpad                 start the desktop app
//	agentpad hook <agent>    forward an agent hook payload (claude|codex|opencode)
//	agentpad version         print version
//	agentpad desktop-install install the .desktop file and icons (taskbar icon)
//	agentpad desktop-remove  remove them again
package main

import (
	"fmt"
	"os"

	"github.com/thereisnotime/agentpad/internal/desktopentry"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hook":
			os.Exit(runHook(os.Args[2:]))
		case "version", "--version", "-v":
			fmt.Println("agentpad", version)
			return
		case "desktop-install":
			if _, err := desktopentry.Ensure(desktopentry.DataHome(), selfPath()); err != nil {
				fmt.Fprintln(os.Stderr, "agentpad:", err)
				os.Exit(1)
			}
			fmt.Println("installed", desktopentry.PathsIn(desktopentry.DataHome()).Desktop)
			return
		case "desktop-remove":
			if err := desktopentry.Remove(desktopentry.DataHome()); err != nil {
				fmt.Fprintln(os.Stderr, "agentpad:", err)
				os.Exit(1)
			}
			return
		case "help", "--help", "-h":
			fmt.Println("usage: agentpad [hook <claude|codex|opencode> | version | desktop-install | desktop-remove]")
			return
		}
	}
	// Best effort: without a matching <app_id>.desktop file Wayland shells show
	// a generic icon for the window. Never fatal.
	_, _ = desktopentry.Ensure(desktopentry.DataHome(), selfPath())
	runUI()
}

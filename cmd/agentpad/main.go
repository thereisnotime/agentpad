// Command agentpad is a desktop controller for CH57x macro pads and AI coding agents.
//
//	agentpad                 start the desktop app
//	agentpad hook <agent>    forward an agent hook payload (claude|codex|opencode)
//	agentpad version         print version
package main

import (
	"fmt"
	"os"
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
		case "help", "--help", "-h":
			fmt.Println("usage: agentpad [hook <claude|codex|opencode> | version]")
			return
		}
	}
	runUI()
}

package main

import (
	"fmt"
	"os"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/hook"
)

// runHook implements `agentpad hook <agent> [args...]`.
func runHook(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentpad hook <claude|codex|opencode> [payload]")
		return 2
	}
	return hook.Run(args[0], args[1:], os.Stdin, config.ListenAddr)
}

// Package hook implements `agentpad hook <agent>`.
package hook

import "io"

// Run forwards one agent payload to the receiver at addr. claude/opencode read
// stdin; codex uses the last argument. POST timeout 300ms. Always returns 0
// for known agents; returns 2 with usage on stderr for an unknown agent.
func Run(agent string, args []string, stdin io.Reader, addr string) int { panic("TODO") }

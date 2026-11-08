// Package hook implements `agentpad hook <agent>`.
package hook

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/thereisnotime/agentpad/internal/config"
)

const (
	maxPayload   = 1 << 20
	stdinTimeout = 500 * time.Millisecond
	postTimeout  = 300 * time.Millisecond
)

// stderr is where usage goes; swapped in tests.
var stderr io.Writer = os.Stderr

// Run forwards one agent payload to the receiver at addr. claude/opencode read
// stdin; codex uses the last argument. POST timeout 300ms. Always returns 0
// for known agents; returns 2 with usage on stderr for an unknown agent.
func Run(agent string, args []string, stdin io.Reader, addr string) int {
	switch agent {
	case "claude", "opencode", "codex":
	default:
		_, _ = fmt.Fprintf(stderr, "usage: agentpad hook <claude|codex|opencode> [payload]\nunknown agent %q\n", agent)
		return 2
	}
	if addr == "" {
		addr = config.ListenAddr
	}

	var payload []byte
	if agent == "codex" && len(args) > 0 {
		payload = []byte(args[len(args)-1])
	} else {
		payload = readStdin(stdin)
	}

	client := &http.Client{Timeout: postTimeout}
	resp, err := client.Post("http://"+addr+config.EventsPath+agent, "application/json", bytes.NewReader(payload))
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}
	return 0
}

// readStdin reads up to maxPayload bytes but gives up after stdinTimeout so a
// stdin that never closes can't hang the agent.
func readStdin(r io.Reader) []byte {
	if r == nil {
		return nil
	}
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(io.LimitReader(r, maxPayload))
		done <- b
	}()
	select {
	case b := <-done:
		return b
	case <-time.After(stdinTimeout):
		return nil
	}
}

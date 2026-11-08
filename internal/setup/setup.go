// Package setup renders copy-paste integration snippets. It never writes files.
package setup

import (
	"encoding/json"
	"strings"
)

// claudeEvents are the Claude Code hook events agentpad listens to. None of
// them require a matcher; omitting it matches everything.
var claudeEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"Notification",
	"PermissionRequest",
	"PostToolUse",
	"Stop",
	"SessionEnd",
}

type claudeHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type claudeGroup struct {
	Hooks []claudeHandler `json:"hooks"`
}

// ClaudeSnippet returns a ~/.claude/settings.json "hooks" JSON block.
func ClaudeSnippet(bin string) string {
	cmd := shellQuote(bin) + " hook claude"
	hooks := make(map[string][]claudeGroup, len(claudeEvents))
	for _, ev := range claudeEvents {
		hooks[ev] = []claudeGroup{{Hooks: []claudeHandler{{Type: "command", Command: cmd, Timeout: 5}}}}
	}
	b, _ := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	return string(b)
}

// CodexSnippet returns the ~/.codex/config.toml notify line.
func CodexSnippet(bin string) string {
	return "notify = [" + tomlString(bin) + `, "hook", "codex"]`
}

// OpenCodePlugin returns a JS plugin for ~/.config/opencode/plugins/agentpad.js.
func OpenCodePlugin(addr string) string {
	url, _ := json.Marshal("http://" + addr + "/v2/events/opencode")
	return `// agentpad OpenCode plugin.
// Save as ~/.config/opencode/plugins/agentpad.js (global) or
// .opencode/plugins/agentpad.js (per project). OpenCode loads it on start.
// Forwards every bus event to the local agentpad receiver; never throws.
// The origin (terminal tab, multiplexer pane, pid) lets agentpad jump back here.
const AGENTPAD_URL = ` + string(url) + `

const env = process.env
const ORIGIN = Object.fromEntries(Object.entries({
  agent_pid: process.pid,
  term_program: env.TERM_PROGRAM,
  window_id: env.WINDOWID,
  konsole_service: env.KONSOLE_DBUS_SERVICE,
  konsole_window: env.KONSOLE_DBUS_WINDOW,
  konsole_session: env.KONSOLE_DBUS_SESSION,
  zellij_session: env.ZELLIJ_SESSION_NAME,
  zellij_pane: env.ZELLIJ_PANE_ID,
  tmux: env.TMUX,
  tmux_pane: env.TMUX_PANE,
}).filter(([, v]) => v))

export const AgentpadPlugin = async () => {
  return {
    event: async ({ event }) => {
      try {
        await fetch(AGENTPAD_URL, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ payload: event, origin: ORIGIN }),
          signal: AbortSignal.timeout(300),
        })
      } catch {
        // agentpad not running; ignore
      }
    },
  }
}
`
}

// UdevRule returns the udev rule granting the active user access to the pad.
func UdevRule() string {
	return `SUBSYSTEM=="usb", ATTR{idVendor}=="1189", ATTR{idProduct}=="8890|8840|8842", TAG+="uaccess"`
}

// shellQuote single-quotes s for sh when it contains anything unusual.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+:@%=") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tomlString renders s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(`\u00`)
				b.WriteByte("0123456789ABCDEF"[r>>4])
				b.WriteByte("0123456789ABCDEF"[r&0xf])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

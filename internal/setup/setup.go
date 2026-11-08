// Package setup renders copy-paste integration snippets. It never writes files.
package setup

// Stubs: implemented by the events work. bin is the absolute agentpad path.

// ClaudeSnippet returns a ~/.claude/settings.json "hooks" JSON block.
func ClaudeSnippet(bin string) string { panic("TODO") }

// CodexSnippet returns the ~/.codex/config.toml notify line.
func CodexSnippet(bin string) string { panic("TODO") }

// OpenCodePlugin returns a JS plugin for ~/.config/opencode/plugin/agentpad.js.
func OpenCodePlugin(addr string) string { panic("TODO") }

// UdevRule returns the udev rule granting the active user access to the pad.
func UdevRule() string {
	return `SUBSYSTEM=="usb", ATTR{idVendor}=="1189", ATTR{idProduct}=="8890|8840|8842", TAG+="uaccess"`
}

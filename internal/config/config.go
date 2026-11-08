// Package config resolves agentpad's on-disk locations and shared constants.
package config

import (
	"os"
	"path/filepath"
)

const (
	// AppID is the reverse-DNS application identifier used by the desktop UI.
	AppID = "dev.thereisnotime.agentpad"
	// ListenAddr is the loopback address of the local event receiver.
	ListenAddr = "127.0.0.1:47321"
	// EventsPath is the HTTP path prefix agents post events to: /v1/events/<agent>.
	EventsPath = "/v1/events/"
)

// ConfigDir returns $XDG_CONFIG_HOME/agentpad (default ~/.config/agentpad).
func ConfigDir() string { return xdg("XDG_CONFIG_HOME", ".config") }

// StateDir returns $XDG_STATE_HOME/agentpad (default ~/.local/state/agentpad).
func StateDir() string { return xdg("XDG_STATE_HOME", filepath.Join(".local", "state")) }

func xdg(env, fallback string) string {
	base := os.Getenv(env)
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, fallback)
	}
	return filepath.Join(base, "agentpad")
}

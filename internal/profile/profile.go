// Package profile defines per-agent key profiles for a 3-key + 1-knob pad.
package profile

// Knob holds the actions for one rotary encoder.
type Knob struct {
	CCW   string `yaml:"ccw"`
	Press string `yaml:"press"`
	CW    string `yaml:"cw"`
}

// Profile maps the pad's three keys and knob to actions understood by
// ch57x-keyboard-tool (e.g. "a", "ctrl-c", "escape", "volumeup").
type Profile struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Keys        [3]string `yaml:"keys"`
	Knob        Knob      `yaml:"knob"`
}

// Store persists user overrides of profiles. Implemented in store.go.
type Store interface {
	// List returns built-ins merged with user overrides, built-in order first.
	List() ([]Profile, error)
	// Get returns the effective profile by name.
	Get(name string) (Profile, error)
	// Save validates and persists p as an override.
	Save(p Profile) error
	// Reset removes the override for name, restoring the built-in.
	Reset(name string) error
	// IsOverridden reports whether name has a persisted override.
	IsOverridden(name string) bool
}

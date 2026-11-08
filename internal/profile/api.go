package profile

// Stubs: implemented by the device/profile work (replace bodies, keep signatures).

// Builtins returns the built-in profiles in display order:
// claude-code, codex, opencode, media.
func Builtins() []Profile { panic("TODO") }

// SupportedKeys returns action names accepted in profiles (for UI suggestions).
func SupportedKeys() []string { panic("TODO") }

// Validate checks every action; the error names the control, e.g. `key 2: unknown key "foo"`.
func Validate(p Profile) error { panic("TODO") }

// NewFileStore returns a Store persisting overrides as <dir>/<name>.yaml.
func NewFileStore(dir string) Store { panic("TODO") }

// Layers orders profiles for upload: active first, then the next two in list
// order (wrapping, skipping active). Returns at most 3.
func Layers(all []Profile, active string) []Profile { panic("TODO") }

// MappingYAML renders 1-3 profiles as a ch57x-keyboard-tool mapping document
// (orientation normal, rows 1, columns 3, knobs 1).
func MappingYAML(layers []Profile) ([]byte, error) { panic("TODO") }

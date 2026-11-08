package profile

// ValidName reports whether name is a valid profile name (non-empty, a-z, 0-9 and -).
func ValidName(name string) bool { return nameRe.MatchString(name) }

// IsBuiltin reports whether name is one of the built-in profiles.
func IsBuiltin(name string) bool {
	for _, b := range Builtins() {
		if b.Name == name {
			return true
		}
	}
	return false
}

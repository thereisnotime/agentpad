package profile

import "reflect"

// Equal reports whether two profiles map every control to the same actions.
// Nil and empty gesture maps are treated as equal.
func Equal(a, b Profile) bool {
	return reflect.DeepEqual(normalized(a), normalized(b))
}

func normalized(p Profile) Profile {
	if len(p.Gestures) == 0 {
		p.Gestures = nil
		return p
	}
	g := make(map[string]map[string]string, len(p.Gestures))
	for control, kinds := range p.Gestures {
		var k map[string]string
		for kind, action := range kinds {
			if action == "" {
				continue
			}
			if k == nil {
				k = map[string]string{}
			}
			k[kind] = action
		}
		if k != nil {
			g[control] = k
		}
	}
	if len(g) == 0 {
		g = nil
	}
	p.Gestures = g
	return p
}

// builtin returns the built-in profile with that name.
func builtin(name string) (Profile, bool) {
	for _, b := range Builtins() {
		if b.Name == name {
			return b, true
		}
	}
	return Profile{}, false
}

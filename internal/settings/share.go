package settings

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"github.com/thereisnotime/agentpad/internal/profile"
	"gopkg.in/yaml.v3"
)

// profileDoc is a shared profile: header plus every Profile field inline,
// so new Profile fields travel without changes here.
type profileDoc struct {
	Kind            string `yaml:"kind"`
	Schema          int    `yaml:"schema"`
	profile.Profile `yaml:",inline"`
}

// ExportProfile renders p as a shareable document (kind agentpad-profile,
// schema 1) suitable for a file or the clipboard.
func ExportProfile(p profile.Profile) ([]byte, error) {
	return yaml.Marshal(profileDoc{Kind: ProfileKind, Schema: Schema, Profile: p})
}

// ParseProfile parses and validates a shared profile. Plain profile YAML
// without kind/schema (e.g. a profiles/<name>.yaml file) is accepted too.
func ParseProfile(data []byte) (profile.Profile, error) {
	var d profileDoc
	if err := yaml.Unmarshal(data, &d); err != nil {
		return profile.Profile{}, &ValidationError{File: "profile", Errors: yamlErrors(err)}
	}
	switch {
	case d.Kind == BundleKind:
		return profile.Profile{}, errors.New("profile: this is a bundle, import it as a bundle")
	case d.Kind != "" && d.Kind != ProfileKind:
		return profile.Profile{}, fmt.Errorf("profile: kind %q, want %q", d.Kind, ProfileKind)
	case d.Schema > Schema:
		return profile.Profile{}, fmt.Errorf("profile: schema %d is newer than this agentpad understands (%d)", d.Schema, Schema)
	}
	if err := profile.Validate(d.Profile); err != nil {
		return profile.Profile{}, fmt.Errorf("profile %s: %w", d.Name, err)
	}
	return d.Profile, nil
}

var suffixRe = regexp.MustCompile(`^(.+)-(\d+)$`)

// ImportProfile parses, validates and saves a shared profile into store.
// When the name already has an override with different content the profile
// is saved under the next free name (codex -> codex-2, codex-3, ...). An
// identical existing override is returned without writing. The returned
// profile carries the name it was saved under.
func ImportProfile(data []byte, store profile.Store) (profile.Profile, error) {
	p, err := ParseProfile(data)
	if err != nil {
		return profile.Profile{}, err
	}
	if store.IsOverridden(p.Name) {
		if old, err := store.Get(p.Name); err == nil && reflect.DeepEqual(old, p) {
			return old, nil
		}
		p.Name = freeName(p.Name, store)
	}
	if err := store.Save(p); err != nil {
		return profile.Profile{}, err
	}
	return p, nil
}

func freeName(name string, store profile.Store) string {
	base := name
	if m := suffixRe.FindStringSubmatch(name); m != nil {
		base = m[1]
	}
	for i := 2; ; i++ {
		cand := base + "-" + strconv.Itoa(i)
		if !store.IsOverridden(cand) && !profile.IsBuiltin(cand) {
			return cand
		}
	}
}

package profile

import "gopkg.in/yaml.v3"

// Layers orders profiles for upload: active first, then the next two in list
// order (wrapping, skipping active). Returns at most 3.
func Layers(all []Profile, active string) []Profile {
	idx := -1
	for i, p := range all {
		if p.Name == active {
			idx = i
			break
		}
	}
	if idx < 0 {
		return append([]Profile(nil), all[:min(3, len(all))]...)
	}
	out := []Profile{all[idx]}
	for i := 1; i < len(all) && len(out) < 3; i++ {
		out = append(out, all[(idx+i)%len(all)])
	}
	return out
}

type mappingDoc struct {
	Orientation string         `yaml:"orientation"`
	Rows        int            `yaml:"rows"`
	Columns     int            `yaml:"columns"`
	Knobs       int            `yaml:"knobs"`
	Layers      []mappingLayer `yaml:"layers"`
}

type mappingLayer struct {
	Buttons [][]quoted    `yaml:"buttons"`
	Knobs   []mappingKnob `yaml:"knobs"`
}

type mappingKnob struct {
	CCW   quoted `yaml:"ccw"`
	Press quoted `yaml:"press"`
	CW    quoted `yaml:"cw"`
}

// quoted always marshals as a double-quoted YAML string so values like "1"
// or "<110>" never turn into numbers or other types.
type quoted string

func (q quoted) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Tag: "!!str", Value: string(q)}, nil
}

// MappingYAML renders 1-3 profiles as a ch57x-keyboard-tool mapping document
// (orientation normal, rows 1, columns 3, knobs 1).
func MappingYAML(layers []Profile) ([]byte, error) {
	return MappingYAMLOriented(layers, OrientNormal)
}

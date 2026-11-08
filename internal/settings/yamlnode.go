package settings

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML decodes an LED map from scratch so a partial map (e.g. only
// effect/color) never inherits a default mode.
func (l *LED) UnmarshalYAML(n *yaml.Node) error {
	type raw struct {
		Layer  int    `yaml:"layer"`
		Mode   int    `yaml:"mode"`
		Effect string `yaml:"effect"`
		Color  string `yaml:"color"`
	}
	var r raw
	if err := n.Decode(&r); err != nil {
		return err
	}
	*l = LED(r)
	return nil
}

var lineRe = regexp.MustCompile(`line (\d+): (.*)`)

// yamlErrors turns yaml.v3 syntax/type errors into FieldErrors with lines.
func yamlErrors(err error) []FieldError {
	var te *yaml.TypeError
	msgs := []string{err.Error()}
	if errors.As(err, &te) {
		msgs = te.Errors
	}
	out := make([]FieldError, 0, len(msgs))
	for _, m := range msgs {
		fe := FieldError{Message: strings.TrimPrefix(m, "yaml: ")}
		if sm := lineRe.FindStringSubmatch(m); sm != nil {
			fe.Line, _ = strconv.Atoi(sm[1])
			fe.Message = sm[2]
		}
		out = append(out, fe)
	}
	return out
}

// parsed is the result of reading one settings document.
type parsed struct {
	settings Settings
	doc      *yaml.Node // DocumentNode wrapping a MappingNode
	warnings []string
}

// parseSettings decodes data on top of Defaults(), reports unknown keys as
// warnings and validates. On error the returned *ValidationError has lines.
// A partially valid document still returns its node (for comment-preserving
// writes) alongside the error.
func parseSettings(data []byte, file string) (*parsed, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, &ValidationError{File: file, Errors: yamlErrors(err)}
	}
	if doc.Kind == 0 { // empty file
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, &ValidationError{File: file, Errors: []FieldError{{
			Line: root.Line, Column: root.Column, Message: "settings must be a YAML mapping",
		}}}
	}
	p := &parsed{settings: Defaults(), doc: &doc}
	if err := root.Decode(&p.settings); err != nil {
		return p, &ValidationError{File: file, Errors: yamlErrors(err)}
	}
	p.warnings = unknownKeys(root, reflect.TypeFor[Settings](), "")
	if errs := Validate(p.settings); len(errs) > 0 {
		return p, &ValidationError{File: file, Errors: locateAll(root, "", errs)}
	}
	return p, nil
}

// locateAll fills Line/Column of errs from the node tree under root.
// prefix is prepended to each path when looking it up (e.g. "settings").
func locateAll(root *yaml.Node, prefix string, errs []FieldError) []FieldError {
	for i := range errs {
		path := strings.TrimPrefix(strings.TrimPrefix(errs[i].Path, prefix), ".")
		if n := locate(root, path); n != nil {
			errs[i].Line, errs[i].Column = n.Line, n.Column
		}
	}
	return errs
}

// locate returns the value node at a dotted path, or the deepest node found.
func locate(root *yaml.Node, path string) *yaml.Node {
	n := root
	if path == "" {
		return n
	}
	for part := range strings.SplitSeq(path, ".") {
		if n.Kind != yaml.MappingNode {
			return n
		}
		i := findKey(n, part)
		if i < 0 {
			return n
		}
		n = n.Content[i+1]
	}
	return n
}

func findKey(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// field returns the type of the struct field / map value named key in t.
func field(t reflect.Type, key string) (reflect.Type, bool) {
	switch t.Kind() {
	case reflect.Map:
		return t.Elem(), true
	case reflect.Struct:
		for i := range t.NumField() {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if name == "" {
				name = strings.ToLower(f.Name)
			}
			if name == key {
				return f.Type, true
			}
		}
	}
	return nil, false
}

// unknownKeys lists keys under n that t has no field for.
func unknownKeys(n *yaml.Node, t reflect.Type, prefix string) []string {
	if n.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		path := strings.TrimPrefix(prefix+"."+k.Value, ".")
		ft, ok := field(t, k.Value)
		if !ok {
			out = append(out, fmt.Sprintf("line %d: unknown key %q (kept, but agentpad ignores it)", k.Line, path))
			continue
		}
		out = append(out, unknownKeys(n.Content[i+1], ft, path)...)
	}
	return out
}

func sameScalar(a, b *yaml.Node) bool {
	return a.Kind == yaml.ScalarNode && b.Kind == yaml.ScalarNode &&
		a.Value == b.Value && a.ShortTag() == b.ShortTag()
}

// merge writes the values of src (freshly encoded known settings) into dst
// (the user's document) in place, keeping comments, key order, styles and
// unknown keys. Keys of a Go map (t is a map) missing from src are removed;
// for structs only known fields missing from src are removed.
func merge(dst, src *yaml.Node, t reflect.Type) {
	for i := 0; i+1 < len(src.Content); i += 2 {
		k, v := src.Content[i], src.Content[i+1]
		j := findKey(dst, k.Value)
		if j < 0 {
			dst.Content = append(dst.Content, k, v)
			continue
		}
		dv := dst.Content[j+1]
		ft, _ := field(t, k.Value)
		if ft != nil && dv.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode &&
			(ft.Kind() == reflect.Struct || ft.Kind() == reflect.Map) {
			merge(dv, v, ft)
			continue
		}
		if sameScalar(dv, v) {
			continue
		}
		v.HeadComment, v.LineComment, v.FootComment = dv.HeadComment, dv.LineComment, dv.FootComment
		if dv.Kind == v.Kind && v.Kind == yaml.ScalarNode && dv.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0 {
			v.Style = dv.Style // keep the user's quoting
		}
		dst.Content[j+1] = v
	}
	kept := dst.Content[:0]
	for i := 0; i+1 < len(dst.Content); i += 2 {
		k := dst.Content[i]
		_, known := field(t, k.Value)
		if findKey(src, k.Value) < 0 && (t.Kind() == reflect.Map || known) {
			continue
		}
		kept = append(kept, k, dst.Content[i+1])
	}
	dst.Content = kept
}

// encodeNode encodes s into a mapping node.
func encodeNode(s Settings) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(s); err != nil {
		return nil, err
	}
	return &n, nil
}

// render writes doc with 2-space indentation.
func render(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const fileHeader = `agentpad settings (schema 1)

Edit freely: agentpad reloads this file a moment after you save it.
Invalid values are reported (with the line) and ignored; the last good
settings stay active. Unknown keys are kept and listed as warnings.
Reference: docs/configuration.md in the agentpad repository.`

// keyComments are head comments for the commented first-run file, by path.
var keyComments = map[string]string{
	"schema": "Settings format version. Leave as is.",
	"pad":    "The macro pad.",
	"pad.orientation": "How the pad sits on the desk:\n" +
		"  normal (knob right) | upsidedown (knob left) | clockwise (knob bottom) | counterclockwise (knob top)",
	"pad.active_profile": "Profile last written to layer 1 (agentpad updates this when you apply):\n" +
		"  claude-code | codex | opencode | media | the name of any custom profile | \"\" (none yet)",
	"pad.mode": "How key presses are handled:\n" +
		"  plain (the pad types the keys itself) | gesture (agentpad reads the pad: tap, hold, double-tap, chords)",
	"led":               "LEDs.",
	"led.follow_agents": "Switch the LEDs when an agent needs attention: true | false",
	"led.8890": "0x8890 boards: built-in effects by index, no colours.\n" +
		"  attention = while an agent waits for you, normal = the rest of the time.\n" +
		"  Format: {mode: 0-5} (0 = off, 1-5 = built-in effects)",
	"led.884x": "0x8840/0x8842 boards: named effects with colours, set per layer.\n" +
		"  Format: {layer: 0-2, effect: off | backlight | shock | shock2 | press, color: COLOR}\n" +
		"  COLOR: red | orange | yellow | green | cyan | blue | purple (white for backlight only);\n" +
		"  leave color out for effect off.",
	"led.labels": "Your names for the 0x8890 effects shown in the UI, keyed by mode index 0-5,\n" +
		"  e.g. labels: {\"1\": Steady, \"2\": Breathing}",
	"gestures":                 "Gesture mode timing, in milliseconds.",
	"gestures.hold_ms":         "Press at least this long to count as a hold: 100-5000",
	"gestures.long_hold_ms":    "Press at least this long to count as a long hold: more than hold_ms, up to 10000",
	"gestures.double_tap_ms":   "Max gap between two taps of a double-tap: 100-1000",
	"gestures.chord_window_ms": "Max gap between the two key presses of a chord: 10-500",
	"notifications":            "Desktop notifications.",
	"notifications.attention":  "Notify when an agent needs attention: true | false",
	"ui":                       "Desktop UI.",
	"ui.theme":                 "Colour theme: system | dark | light",
}

// commentedDoc renders s as a document with explanatory comments.
func commentedDoc(s Settings) (*yaml.Node, error) {
	root, err := encodeNode(s)
	if err != nil {
		return nil, err
	}
	var annotate func(n *yaml.Node, prefix string)
	annotate = func(n *yaml.Node, prefix string) {
		if n.Kind != yaml.MappingNode || n.Style == yaml.FlowStyle {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			path := strings.TrimPrefix(prefix+"."+n.Content[i].Value, ".")
			if c, ok := keyComments[path]; ok {
				n.Content[i].HeadComment = c
			}
			annotate(n.Content[i+1], path)
		}
	}
	annotate(root, "")
	return &yaml.Node{Kind: yaml.DocumentNode, HeadComment: fileHeader, Content: []*yaml.Node{root}}, nil
}

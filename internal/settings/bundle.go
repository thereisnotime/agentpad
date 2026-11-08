package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/scripts"
	"gopkg.in/yaml.v3"
)

// Bundle and profile document kinds.
const (
	BundleKind  = "agentpad-bundle"
	ProfileKind = "agentpad-profile"
	// BundleExt is the suggested file extension for bundles.
	BundleExt = ".agentpad.yaml"
	// RulesFile is the per-app rules file passed through bundles verbatim.
	RulesFile = "rules.yaml"
	// BackupDir is the directory (inside the config dir) holding backups.
	BackupDir = "backups"
)

const backupTimeLayout = "20060102T150405Z"

// ExportOptions selects what Export writes. The zero value exports settings,
// profile overrides and rules.yaml (when present).
type ExportOptions struct {
	ExportedBy   string // e.g. "agentpad v0.2.0"
	AllProfiles  bool   // include built-ins that are not overridden
	SkipSettings bool
	SkipProfiles bool
	SkipRules    bool
}

type bundleOut struct {
	Kind       string            `yaml:"kind"`
	Schema     int               `yaml:"schema"`
	ExportedBy string            `yaml:"exported_by,omitempty"`
	ExportedAt string            `yaml:"exported_at"`
	Settings   *yaml.Node        `yaml:"settings,omitempty"`
	Profiles   []profile.Profile `yaml:"profiles,omitempty"`
	Rules      *yaml.Node        `yaml:"rules,omitempty"`
}

// Export writes a bundle to w.
func (s *Store) Export(w io.Writer, opts ExportOptions) error {
	out := bundleOut{
		Kind: BundleKind, Schema: Schema, ExportedBy: opts.ExportedBy,
		ExportedAt: s.now().UTC().Format(time.RFC3339),
	}
	if !opts.SkipSettings {
		n, err := encodeNode(s.Get())
		if err != nil {
			return err
		}
		out.Settings = n
	}
	if !opts.SkipProfiles {
		all, err := s.profiles.List()
		if err != nil {
			return err
		}
		for _, p := range all {
			if opts.AllProfiles || s.profiles.IsOverridden(p.Name) {
				out.Profiles = append(out.Profiles, p)
			}
		}
	}
	if !opts.SkipRules {
		data, err := os.ReadFile(filepath.Join(s.dir, RulesFile))
		switch {
		case err == nil:
			var doc yaml.Node
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("%s: %w", RulesFile, err)
			}
			if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
				out.Rules = doc.Content[0]
			}
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
	}
	var root yaml.Node
	if err := root.Encode(out); err != nil {
		return err
	}
	data, err := render(&yaml.Node{
		Kind:        yaml.DocumentNode,
		HeadComment: "agentpad bundle: import it from the app (Settings > Import) to preview the changes first.",
		Content:     []*yaml.Node{&root},
	})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// Bundle is a parsed bundle. Sections are nil when absent.
type Bundle struct {
	Kind       string
	Schema     int
	ExportedBy string
	ExportedAt time.Time  // zero when missing or unparsable
	Settings   *yaml.Node // raw mapping; may be partial
	Profiles   []profile.Profile
	Rules      *yaml.Node // raw rules.yaml content

	profileNodes []*yaml.Node
}

// ParseBundle reads a bundle and checks its kind and schema. Content is
// validated by PlanImport.
func ParseBundle(r io.Reader) (*Bundle, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, &ValidationError{File: "bundle", Errors: yamlErrors(err)}
	}
	if doc.Kind == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("bundle: not a YAML mapping")
	}
	var raw struct {
		Kind       string    `yaml:"kind"`
		Schema     int       `yaml:"schema"`
		ExportedBy string    `yaml:"exported_by"`
		ExportedAt string    `yaml:"exported_at"`
		Settings   yaml.Node `yaml:"settings"`
		Profiles   yaml.Node `yaml:"profiles"`
		Rules      yaml.Node `yaml:"rules"`
	}
	if err := doc.Content[0].Decode(&raw); err != nil {
		return nil, &ValidationError{File: "bundle", Errors: yamlErrors(err)}
	}
	switch {
	case raw.Kind == ProfileKind:
		return nil, errors.New("bundle: this is a single profile, import it as a profile")
	case raw.Kind != BundleKind:
		return nil, fmt.Errorf("bundle: kind %q, want %q", raw.Kind, BundleKind)
	case raw.Schema < 1:
		return nil, errors.New("bundle: missing schema")
	case raw.Schema > Schema:
		return nil, fmt.Errorf("bundle: schema %d is newer than this agentpad understands (%d)", raw.Schema, Schema)
	}
	b := &Bundle{Kind: raw.Kind, Schema: raw.Schema, ExportedBy: raw.ExportedBy}
	b.ExportedAt, _ = time.Parse(time.RFC3339, raw.ExportedAt)
	if raw.Settings.Kind != 0 && raw.Settings.Tag != "!!null" {
		if raw.Settings.Kind != yaml.MappingNode {
			return nil, &ValidationError{File: "bundle", Errors: []FieldError{{
				Path: "settings", Line: raw.Settings.Line, Column: raw.Settings.Column, Message: "must be a mapping",
			}}}
		}
		b.Settings = &raw.Settings
	}
	if raw.Profiles.Kind != 0 && raw.Profiles.Tag != "!!null" {
		if raw.Profiles.Kind != yaml.SequenceNode {
			return nil, &ValidationError{File: "bundle", Errors: []FieldError{{
				Path: "profiles", Line: raw.Profiles.Line, Column: raw.Profiles.Column, Message: "must be a list",
			}}}
		}
		var errs []FieldError
		b.Profiles = []profile.Profile{} // present, possibly empty (replace resets all)
		for i, n := range raw.Profiles.Content {
			var p profile.Profile
			if err := n.Decode(&p); err != nil {
				for _, fe := range yamlErrors(err) {
					fe.Path = fmt.Sprintf("profiles[%d]", i)
					errs = append(errs, fe)
				}
				continue
			}
			b.Profiles = append(b.Profiles, p)
			b.profileNodes = append(b.profileNodes, n)
		}
		if len(errs) > 0 {
			return nil, &ValidationError{File: "bundle", Errors: errs}
		}
	}
	if raw.Rules.Kind != 0 && raw.Rules.Tag != "!!null" {
		b.Rules = &raw.Rules
	}
	return b, nil
}

// ImportMode chooses how a bundle is applied.
type ImportMode int

const (
	// Merge adds and updates; nothing that is missing from the bundle is removed.
	Merge ImportMode = iota
	// Replace makes each section present in the bundle match it exactly:
	// settings start from defaults, profile overrides not in the bundle are
	// reset. Sections missing from the bundle are left alone.
	Replace
)

func (m ImportMode) String() string {
	if m == Replace {
		return "replace"
	}
	return "merge"
}

// ChangeKind is the kind of a planned change.
type ChangeKind string

// Change kinds.
const (
	Add    ChangeKind = "add"
	Modify ChangeKind = "change"
	Remove ChangeKind = "remove"
)

// Change areas.
const (
	AreaSettings = "settings"
	AreaProfile  = "profile"
	AreaRules    = "rules"
	AreaScripts  = "scripts"
	AreaHooks    = "hooks"
)

// Change is one line of an import preview.
type Change struct {
	Kind ChangeKind
	Area string // AreaSettings | AreaProfile | AreaRules
	// Path is the dotted settings path, the profile name, or "rules.yaml".
	Path string
	// Old and New are display values: scalars for settings, YAML for
	// profiles, the command line for scripts. Empty for adds (Old) and
	// removals (New).
	Old, New string
	// Dangerous marks changes that make agentpad run commands (scripts and
	// hooks added or changed). They need explicit consent: see Plan.Consent.
	Dangerous bool

	hook scripts.Hook // AreaHooks: the hook added or removed
}

func (c Change) String() string {
	switch c.Kind {
	case Add:
		return fmt.Sprintf("+ %s %s", c.Area, c.Path)
	case Remove:
		return fmt.Sprintf("- %s %s", c.Area, c.Path)
	}
	if c.Area == AreaSettings || c.Area == AreaScripts {
		return fmt.Sprintf("~ %s %s: %s -> %s", c.Area, c.Path, c.Old, c.New)
	}
	return fmt.Sprintf("~ %s %s", c.Area, c.Path)
}

// Plan is a validated import, ready to preview and Apply.
type Plan struct {
	Mode     ImportMode
	Changes  []Change
	Warnings []string

	settings *Settings // nil: settings unchanged
	base     Settings  // settings the plan was computed against
	save     []profile.Profile
	remove   []string
	rules    []byte // nil: rules unchanged
}

// Empty reports whether applying the plan would change nothing.
func (p *Plan) Empty() bool { return len(p.Changes) == 0 }

// Dangerous returns the changes that need consent (scripts and hooks added
// or changed), in preview order.
func (p *Plan) Dangerous() []Change {
	var out []Change
	for _, c := range p.Changes {
		if c.Dangerous {
			out = append(out, c)
		}
	}
	return out
}

// Consent returns a copy of p keeping only the dangerous changes accept
// returns true for; every other change is kept as is. A refused script
// keeps its current definition (or stays absent), a refused hook is not
// added. Apply the returned plan. A hook or profile that needs a refused
// script then fails validation in Apply, before anything is written.
func (p *Plan) Consent(accept func(Change) bool) *Plan {
	out := *p
	out.Changes = nil
	var next *Settings
	if p.settings != nil {
		n := p.settings.Clone()
		next = &n
	}
	for _, c := range p.Changes {
		if !c.Dangerous || accept(c) {
			out.Changes = append(out.Changes, c)
			continue
		}
		if next == nil {
			continue
		}
		switch c.Area {
		case AreaScripts:
			if old, ok := p.base.Scripts[c.Path]; ok {
				next.Scripts[c.Path] = old.Clone()
			} else {
				delete(next.Scripts, c.Path)
			}
		case AreaHooks:
			next.Hooks = slices.DeleteFunc(next.Hooks, func(h scripts.Hook) bool { return h == c.hook })
		}
	}
	if next != nil && len(next.Scripts) == 0 {
		next.Scripts = nil
	}
	if next != nil && len(next.Hooks) == 0 {
		next.Hooks = nil
	}
	settingsLeft := false
	for _, c := range out.Changes {
		if c.Area == AreaSettings || c.Area == AreaScripts || c.Area == AreaHooks {
			settingsLeft = true
		}
	}
	if !settingsLeft {
		next = nil
	}
	out.settings = next
	return &out
}

// Count returns the number of changes of kind k.
func (p *Plan) Count(k ChangeKind) int {
	n := 0
	for _, c := range p.Changes {
		if c.Kind == k {
			n++
		}
	}
	return n
}

// PlanImport validates everything in b (settings via Validate, LED policies
// via pad.ValidateLED for their board family, profiles via profile.Validate)
// and computes the changes against the current configuration. On any
// problem it returns a *ValidationError naming the field or profile and
// control, with bundle line numbers.
func (s *Store) PlanImport(b *Bundle, mode ImportMode) (*Plan, error) {
	cur := s.Get()
	plan := &Plan{Mode: mode, base: cur}
	var errs []FieldError
	resolve := cur.HasScript

	if b.Settings != nil {
		next := Defaults()
		if mode == Merge {
			next = cur.Clone()
		}
		if mode == Merge {
			// decoding replaces the scripts map and hooks list; merging keeps
			// what the bundle doesn't mention
			next.Scripts, next.Hooks = nil, nil
		}
		if err := b.Settings.Decode(&next); err != nil {
			for _, fe := range yamlErrors(err) {
				fe.Path = "settings"
				errs = append(errs, fe)
			}
		} else {
			for _, w := range unknownKeys(b.Settings, reflect.TypeFor[Settings](), "settings") {
				plan.Warnings = append(plan.Warnings, strings.Replace(w, "(kept, but agentpad ignores it)", "(not imported)", 1))
			}
			if mode == Merge {
				next.Scripts = mergeScripts(cur.Scripts, next.Scripts)
				next.Hooks = mergeHooks(cur.Hooks, next.Hooks)
			}
			resolve = next.HasScript
			if se := Validate(next); len(se) > 0 {
				for i := range se {
					se[i].Path = "settings." + se[i].Path
				}
				errs = append(errs, locateAll(b.Settings, "settings", se)...)
			} else if ch := diffSettings(cur, next); len(ch) > 0 {
				plan.Changes = append(plan.Changes, ch...)
				plan.settings = &next
			}
		}
	}

	if b.Profiles != nil {
		errs = append(errs, s.planProfiles(b, plan, resolve)...)
	}

	if b.Rules != nil {
		data, err := render(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{b.Rules}})
		if err != nil {
			errs = append(errs, FieldError{Path: "rules", Message: err.Error()})
		} else {
			old, rerr := os.ReadFile(filepath.Join(s.dir, RulesFile))
			switch {
			case errors.Is(rerr, fs.ErrNotExist):
				plan.Changes = append(plan.Changes, Change{Kind: Add, Area: AreaRules, Path: RulesFile, New: string(data)})
				plan.rules = data
			case rerr != nil:
				return nil, rerr
			case !bytes.Equal(old, data):
				plan.Changes = append(plan.Changes, Change{Kind: Modify, Area: AreaRules, Path: RulesFile, Old: string(old), New: string(data)})
				plan.rules = data
			}
		}
	}
	if len(errs) > 0 {
		return nil, &ValidationError{File: "bundle", Errors: errs}
	}
	return plan, nil
}

// mergeScripts is cur with add's scripts added or replaced.
func mergeScripts(cur, add map[string]scripts.Script) map[string]scripts.Script {
	if len(cur) == 0 && len(add) == 0 {
		return nil
	}
	out := make(map[string]scripts.Script, len(cur)+len(add))
	for k, v := range cur {
		out[k] = v.Clone()
	}
	for k, v := range add {
		out[k] = v.Clone()
	}
	return out
}

// mergeHooks is cur followed by the hooks of add it doesn't have yet.
func mergeHooks(cur, add []scripts.Hook) []scripts.Hook {
	out := slices.Clone(cur)
	for _, h := range add {
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

func (s *Store) planProfiles(b *Bundle, plan *Plan, resolve func(string) bool) []FieldError {
	var errs []FieldError
	seen := map[string]bool{}
	for i, p := range b.Profiles {
		n := b.profileNodes[i]
		path := "profiles." + p.Name
		if p.Name == "" {
			path = fmt.Sprintf("profiles[%d]", i)
		}
		if err := profile.ValidateWith(p, resolve); err != nil {
			errs = append(errs, FieldError{Path: path, Line: n.Line, Column: n.Column, Message: err.Error()})
			continue
		}
		if seen[p.Name] {
			errs = append(errs, FieldError{Path: path, Line: n.Line, Column: n.Column, Message: "duplicate profile name"})
			continue
		}
		seen[p.Name] = true
		old, err := s.profiles.Get(p.Name)
		exists := err == nil
		switch {
		case exists && reflect.DeepEqual(old, p):
			continue // unchanged (an equal built-in needs no override either)
		case exists:
			plan.Changes = append(plan.Changes, Change{Kind: Modify, Area: AreaProfile, Path: p.Name,
				Old: profileText(old), New: profileText(p)})
		default:
			plan.Changes = append(plan.Changes, Change{Kind: Add, Area: AreaProfile, Path: p.Name, New: profileText(p)})
		}
		plan.save = append(plan.save, p)
	}
	if plan.Mode == Replace {
		names, err := s.overrideNames()
		if err != nil {
			return append(errs, FieldError{Path: "profiles", Message: err.Error()})
		}
		for _, name := range names {
			if seen[name] {
				continue
			}
			old, _ := s.profiles.Get(name)
			c := Change{Kind: Remove, Area: AreaProfile, Path: name, Old: profileText(old)}
			if profile.IsBuiltin(name) {
				c.Kind = Modify // resetting a built-in brings back its defaults
				for _, bi := range profile.Builtins() {
					if bi.Name == name {
						c.New = profileText(bi)
					}
				}
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("profile %s: override removed, built-in restored", name))
			}
			plan.Changes = append(plan.Changes, c)
			plan.remove = append(plan.remove, name)
		}
	}
	return errs
}

func profileText(p profile.Profile) string {
	b, err := yaml.Marshal(p)
	if err != nil {
		return p.Name
	}
	return string(b)
}

// overrideNames lists profile names with an override file, sorted.
func (s *Store) overrideNames() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "profiles"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if ok && !e.IsDir() && profile.ValidName(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// leaves flattens settings to dotted path -> display value. LED maps are
// one leaf each ({mode: 1}).
func leaves(s Settings) ([]string, map[string]string) {
	n, err := encodeNode(s)
	if err != nil {
		return nil, nil
	}
	var order []string
	vals := map[string]string{}
	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		if prefix == "scripts" || prefix == "hooks" {
			return // diffed by diffScripts / diffHooks
		}
		if n.Kind == yaml.MappingNode && n.Style != yaml.FlowStyle {
			for i := 0; i+1 < len(n.Content); i += 2 {
				walk(n.Content[i+1], strings.TrimPrefix(prefix+"."+n.Content[i].Value, "."))
			}
			return
		}
		v := n.Value
		if n.Kind != yaml.ScalarNode {
			out, err := yaml.Marshal(n)
			if err == nil {
				v = strings.TrimSpace(string(out))
			}
		}
		order = append(order, prefix)
		vals[prefix] = v
	}
	walk(n, "")
	return order, vals
}

func diffSettings(old, next Settings) []Change {
	oOrder, o := leaves(old)
	nOrder, n := leaves(next)
	var out []Change
	for _, p := range nOrder {
		ov, had := o[p]
		switch {
		case !had:
			out = append(out, Change{Kind: Add, Area: AreaSettings, Path: p, New: n[p]})
		case ov != n[p]:
			out = append(out, Change{Kind: Modify, Area: AreaSettings, Path: p, Old: ov, New: n[p]})
		}
	}
	for _, p := range oOrder {
		if _, ok := n[p]; !ok {
			out = append(out, Change{Kind: Remove, Area: AreaSettings, Path: p, Old: o[p]})
		}
	}
	out = append(out, diffScripts(old.Scripts, next.Scripts)...)
	return append(out, diffHooks(old.Hooks, next.Hooks)...)
}

// scriptText is the display value of a script: its command plus options.
func scriptText(s scripts.Script) string {
	t := s.Preview()
	if s.Cwd != "" {
		t += "  (in " + s.Cwd + ")"
	}
	if len(s.Env) > 0 {
		keys := slices.Sorted(maps.Keys(s.Env))
		t += "  env " + strings.Join(keys, ",")
	}
	if s.Timeout > 0 {
		t += "  timeout " + s.Timeout.String()
	}
	return t
}

func diffScripts(old, next map[string]scripts.Script) []Change {
	var out []Change
	for _, name := range slices.Sorted(maps.Keys(next)) {
		ns := next[name]
		prev, had := old[name]
		switch {
		case !had:
			out = append(out, Change{Kind: Add, Area: AreaScripts, Path: name, New: scriptText(ns), Dangerous: true})
		case !reflect.DeepEqual(prev, ns):
			out = append(out, Change{Kind: Modify, Area: AreaScripts, Path: name, Old: scriptText(prev), New: scriptText(ns), Dangerous: true})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(old)) {
		if _, ok := next[name]; !ok {
			out = append(out, Change{Kind: Remove, Area: AreaScripts, Path: name, Old: scriptText(old[name])})
		}
	}
	return out
}

func diffHooks(old, next []scripts.Hook) []Change {
	var out []Change
	for _, h := range next {
		if !slices.Contains(old, h) {
			out = append(out, Change{Kind: Add, Area: AreaHooks, Path: h.String(), New: h.String(), Dangerous: true, hook: h})
		}
	}
	for _, h := range old {
		if !slices.Contains(next, h) {
			out = append(out, Change{Kind: Remove, Area: AreaHooks, Path: h.String(), Old: h.String(), hook: h})
		}
	}
	return out
}

// Apply backs up the current configuration to backups/<UTC timestamp>/ and
// then writes the plan, each file atomically. It returns the backup name.
// The plan is re-validated first; nothing is written if that fails.
// Watch's onChange (if any) runs afterwards when settings changed.
func (s *Store) Apply(plan *Plan) (string, error) {
	if plan == nil || plan.Empty() {
		return "", nil
	}
	if plan.settings != nil {
		if errs := Validate(*plan.settings); len(errs) > 0 {
			return "", &ValidationError{File: "plan", Errors: errs}
		}
	}
	resolve := s.Get().HasScript
	if plan.settings != nil {
		resolve = plan.settings.HasScript
	}
	for _, p := range plan.save {
		if err := profile.ValidateWith(p, resolve); err != nil {
			return "", fmt.Errorf("profile %s: %w", p.Name, err)
		}
	}
	name, err := s.backup()
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if plan.settings != nil {
		s.mu.Lock()
		_, err := s.refreshLocked()
		if err == nil {
			err = s.setLocked(*plan.settings)
		}
		s.mu.Unlock()
		if err != nil {
			return name, err
		}
	}
	for _, p := range plan.save {
		if err := s.profiles.Save(p); err != nil {
			return name, err
		}
	}
	for _, n := range plan.remove {
		if err := s.profiles.Reset(n); err != nil {
			return name, err
		}
	}
	if plan.rules != nil {
		if err := writeAtomic(filepath.Join(s.dir, RulesFile), plan.rules); err != nil {
			return name, err
		}
	}
	if plan.settings != nil {
		s.fireNotify()
	}
	return name, nil
}

func (s *Store) fireNotify() {
	s.mu.Lock()
	cur, notify := s.cur.Clone(), s.notify
	s.mu.Unlock()
	if notify != nil {
		notify(cur, nil)
	}
}

// Backup is one backup directory.
type Backup struct {
	Name  string    // directory name under backups/, pass to Restore
	Time  time.Time // when it was taken (UTC)
	Files []string  // relative paths, e.g. "settings.yaml", "profiles/codex.yaml"
}

// backup copies settings.yaml, profiles/*.yaml and rules.yaml into a new
// backups/<timestamp> directory and returns its name.
func (s *Store) backup() (string, error) {
	root := filepath.Join(s.dir, BackupDir)
	base := s.now().UTC().Format(backupTimeLayout)
	name := base
	for i := 2; ; i++ {
		err := os.MkdirAll(root, 0o755)
		if err == nil {
			err = os.Mkdir(filepath.Join(root, name), 0o755)
		}
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	dst := filepath.Join(root, name)
	files := []string{FileName, RulesFile}
	overrides, err := s.overrideNames()
	if err != nil {
		return "", err
	}
	for _, n := range overrides {
		files = append(files, filepath.Join("profiles", n+".yaml"))
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(s.dir, f))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := writeAtomic(filepath.Join(dst, f), data); err != nil {
			return "", err
		}
	}
	return name, nil
}

// Backups lists backups, newest first.
func (s *Store) Backups() ([]Backup, error) {
	root := filepath.Join(s.dir, BackupDir)
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Backup
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ts, _, _ := strings.Cut(e.Name(), "-")
		t, err := time.Parse(backupTimeLayout, ts)
		if err != nil {
			continue
		}
		b := Backup{Name: e.Name(), Time: t}
		_ = filepath.WalkDir(filepath.Join(root, e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(filepath.Join(root, e.Name()), p)
				b.Files = append(b.Files, filepath.ToSlash(rel))
			}
			return nil
		})
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b Backup) int {
		if c := b.Time.Compare(a.Time); c != 0 {
			return c
		}
		return strings.Compare(b.Name, a.Name)
	})
	return out, nil
}

// Restore replaces settings.yaml, the profile overrides and rules.yaml with
// the contents of backup name. The backup is validated first and the
// current configuration is itself backed up (the returned name), so a
// restore can be undone. Watch's onChange (if any) runs afterwards.
func (s *Store) Restore(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid backup name %q", name)
	}
	src := filepath.Join(s.dir, BackupDir, name)
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("backup %q not found", name)
	}
	settingsData, err := os.ReadFile(filepath.Join(src, FileName))
	hasSettings := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var p *parsed
	if hasSettings {
		var perr error
		if p, perr = parseSettings(settingsData, "backup "+name+"/"+FileName); perr != nil {
			return "", perr
		}
	}
	bstore := profile.NewFileStore(filepath.Join(src, "profiles"))
	bprofiles, err := bstore.List()
	if err != nil {
		return "", fmt.Errorf("backup %s: %w", name, err)
	}
	var restore []profile.Profile
	for _, bp := range bprofiles {
		if !bstore.IsOverridden(bp.Name) {
			continue
		}
		if err := profile.Validate(bp); err != nil {
			return "", fmt.Errorf("backup %s: profile %s: %w", name, bp.Name, err)
		}
		restore = append(restore, bp)
	}
	rules, err := os.ReadFile(filepath.Join(src, RulesFile))
	hasRules := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	safety, err := s.backup()
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if hasSettings {
		s.mu.Lock()
		err := writeAtomic(s.path, settingsData)
		if err == nil {
			s.lastHash = sha256Sum(settingsData)
			s.cur, s.doc, s.warnings = p.settings, p.doc, p.warnings
		}
		s.mu.Unlock()
		if err != nil {
			return safety, err
		}
	}
	keep := map[string]bool{}
	for _, bp := range restore {
		keep[bp.Name] = true
		data, err := os.ReadFile(filepath.Join(src, "profiles", bp.Name+".yaml"))
		if err != nil {
			return safety, err
		}
		if err := writeAtomic(filepath.Join(s.dir, "profiles", bp.Name+".yaml"), data); err != nil {
			return safety, err
		}
	}
	current, err := s.overrideNames()
	if err != nil {
		return safety, err
	}
	for _, n := range current {
		if !keep[n] {
			if err := s.profiles.Reset(n); err != nil {
				return safety, err
			}
		}
	}
	rulesPath := filepath.Join(s.dir, RulesFile)
	if hasRules {
		err = writeAtomic(rulesPath, rules)
	} else if err = os.Remove(rulesPath); errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return safety, err
	}
	s.fireNotify()
	return safety, nil
}

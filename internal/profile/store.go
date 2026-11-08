package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type fileStore struct {
	dir string
}

// NewFileStore returns a Store persisting overrides as <dir>/<name>.yaml.
func NewFileStore(dir string) Store { return &fileStore{dir: dir} }

func (s *fileStore) path(name string) string { return filepath.Join(s.dir, name+".yaml") }

func (s *fileStore) load(name string) (Profile, error) {
	b, err := os.ReadFile(s.path(name))
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := yaml.Unmarshal(b, &p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", name, err)
	}
	p.Name = name // the file name is authoritative
	return p, nil
}

func (s *fileStore) List() ([]Profile, error) {
	builtins := Builtins()
	seen := make(map[string]bool, len(builtins))
	out := make([]Profile, 0, len(builtins))
	for _, b := range builtins {
		seen[b.Name] = true
		p, err := s.load(b.Name)
		switch {
		case err == nil:
			out = append(out, p)
		case errors.Is(err, fs.ErrNotExist):
			out = append(out, b)
		default:
			return nil, err
		}
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var extra []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok || e.IsDir() || seen[name] || !nameRe.MatchString(name) {
			continue
		}
		extra = append(extra, name)
	}
	sort.Strings(extra)
	for _, name := range extra {
		p, err := s.load(name)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *fileStore) Get(name string) (Profile, error) {
	if !nameRe.MatchString(name) {
		return Profile{}, fmt.Errorf("invalid profile name %q", name)
	}
	p, err := s.load(name)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return Profile{}, err
	}
	for _, b := range Builtins() {
		if b.Name == name {
			return b, nil
		}
	}
	return Profile{}, fmt.Errorf("unknown profile %q", name)
}

func (s *fileStore) Save(p Profile) error {
	if err := Validate(p); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "."+p.Name+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path(p.Name))
}

func (s *fileStore) Reset(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q", name)
	}
	err := os.Remove(s.path(name))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *fileStore) IsOverridden(name string) bool {
	if !nameRe.MatchString(name) {
		return false
	}
	_, err := os.Stat(s.path(name))
	return err == nil
}

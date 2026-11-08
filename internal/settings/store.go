package settings

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
	"gopkg.in/yaml.v3"
)

// Store owns settings.yaml in one config directory. It is safe for
// concurrent use.
type Store struct {
	dir      string
	path     string
	profiles profile.Store

	mu       sync.Mutex
	cur      Settings
	doc      *yaml.Node // last parsed document (comments, unknown keys)
	warnings []string
	lastHash [sha256.Size]byte // content last read or written by us
	created  bool              // settings.yaml did not exist at Open
	migrated bool
	model    uint16
	notify   func(Settings, error) // set by Watch

	// Watch tunables; tests shorten them.
	pollInterval time.Duration
	debounce     time.Duration
	forcePoll    bool
	now          func() time.Time
}

// Open loads <dir>/settings.yaml, creating dir and a commented default file
// when missing. If the file exists but is invalid, Open returns a usable
// Store holding defaults together with a *ValidationError; the file is left
// untouched until it is fixed or Update is called.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		dir:          dir,
		path:         filepath.Join(dir, FileName),
		profiles:     profile.NewFileStore(filepath.Join(dir, "profiles")),
		cur:          Defaults(),
		pollInterval: time.Second,
		debounce:     250 * time.Millisecond,
		now:          time.Now,
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.created = true
		doc, err := commentedDoc(s.cur)
		if err != nil {
			return nil, err
		}
		s.doc = doc
		if err := s.writeDocLocked(); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	s.lastHash = sha256.Sum256(data)
	p, perr := parseSettings(data, s.path)
	if p != nil {
		s.doc, s.warnings = p.doc, p.warnings
	}
	if perr != nil {
		return s, perr
	}
	s.cur = p.settings
	return s, nil
}

// Dir returns the config directory.
func (s *Store) Dir() string { return s.dir }

// Path returns the settings file path.
func (s *Store) Path() string { return s.path }

// Profiles returns the profile override store under <dir>/profiles.
func (s *Store) Profiles() profile.Store { return s.profiles }

// SetModel sets the connected pad's USB product ID; imports then validate LED
// settings against its capabilities. 0 (the default) means unknown.
func (s *Store) SetModel(productID uint16) {
	s.mu.Lock()
	s.model = productID
	s.mu.Unlock()
}

// Get returns a copy of the current (last valid) settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Clone()
}

// Warnings lists non-fatal problems: unknown keys (with lines) and skipped
// migration values.
func (s *Store) Warnings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.warnings)
}

// Update applies fn to a copy of the settings, validates the result and
// writes it atomically, editing the existing YAML tree so comments, key
// order and unknown keys survive. Nothing is written when validation fails.
// An external edit not yet seen by Watch is picked up first; Update refuses
// to overwrite a file that is not valid YAML.
func (s *Store) Update(fn func(*Settings)) error {
	s.mu.Lock()
	adopted, err := s.refreshLocked()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	next := s.cur.Clone()
	fn(&next)
	err = s.setLocked(next)
	cur, notify := s.cur.Clone(), s.notify
	s.mu.Unlock()
	if adopted && notify != nil {
		notify(cur, nil)
	}
	return err
}

// setLocked validates next and writes it into the document.
func (s *Store) setLocked(next Settings) error {
	if errs := Validate(next); len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	fresh, err := encodeNode(next)
	if err != nil {
		return err
	}
	if s.doc == nil {
		if s.doc, err = commentedDoc(next); err != nil {
			return err
		}
	} else {
		merge(s.doc.Content[0], fresh, reflect.TypeFor[Settings]())
	}
	if err := s.writeDocLocked(); err != nil {
		s.lastHash = [sha256.Size]byte{} // force a re-read next time
		return err
	}
	s.cur = next.Clone()
	return nil
}

func (s *Store) writeDocLocked() error {
	data, err := render(s.doc)
	if err != nil {
		return err
	}
	s.lastHash = sha256.Sum256(data)
	return writeAtomic(s.path, data)
}

// refreshLocked re-reads the file if it changed behind our back. It reports
// whether a valid external edit was adopted. A file that is valid YAML but
// has invalid values contributes its tree (so comments survive the next
// write) while the settings stay at the last good values.
func (s *Store) refreshLocked() (bool, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.doc = nil
		return false, nil
	}
	if err != nil {
		return false, err
	}
	h := sha256.Sum256(data)
	if h == s.lastHash {
		return false, nil
	}
	p, perr := parseSettings(data, s.path)
	if p == nil {
		return false, perr // not YAML: never overwrite the user's half-edit
	}
	s.lastHash, s.doc, s.warnings = h, p.doc, p.warnings
	if perr != nil {
		return false, nil
	}
	s.cur = p.settings
	return true, nil
}

// writeAtomic writes data to path via a temp file and rename. A symlinked
// path (dotfiles) is resolved so the link itself is kept.
func writeAtomic(path string, data []byte) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func sha256Sum(b []byte) [sha256.Size]byte { return sha256.Sum256(b) }

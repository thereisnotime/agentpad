package settings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Watch reloads settings.yaml when it changes on disk until ctx is done.
// It watches the config directory (and the link target's directory when
// settings.yaml is a symlink) with inotify and falls back to polling once
// per second when inotify is unavailable; a slow poll also runs alongside
// inotify as a safety net. Bursts of events are debounced (250 ms) and the
// store's own writes are ignored by content hash.
//
// onChange runs on the watcher goroutine with the new settings and a nil
// error, or with the unchanged last good settings and a *ValidationError
// when the edit is invalid. Watch sets up the watch before returning.
func (s *Store) Watch(ctx context.Context, onChange func(Settings, error)) {
	s.mu.Lock()
	s.notify = onChange
	s.mu.Unlock()
	events := make(chan struct{}, 1)
	if !s.forcePoll {
		if fd, err := s.startInotify(); err == nil {
			go readInotify(ctx, fd, s.watchedNames(), events)
		}
	}
	go s.watchLoop(ctx, events, onChange)
}

func (s *Store) watchedNames() map[string]bool {
	names := map[string]bool{FileName: true}
	if target, err := filepath.EvalSymlinks(s.path); err == nil {
		names[filepath.Base(target)] = true
	}
	return names
}

func (s *Store) startInotify() (int, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return -1, err
	}
	dirs := map[string]bool{s.dir: true}
	if target, err := filepath.EvalSymlinks(s.path); err == nil {
		dirs[filepath.Dir(target)] = true
	}
	const mask = unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO | unix.IN_CREATE | unix.IN_MODIFY | unix.IN_DELETE
	for d := range dirs {
		if _, err := unix.InotifyAddWatch(fd, d, mask); err != nil {
			_ = unix.Close(fd)
			return -1, err
		}
	}
	return fd, nil
}

// readInotify signals events for the watched names until ctx is done, then
// closes fd.
func readInotify(ctx context.Context, fd int, names map[string]bool, events chan<- struct{}) {
	defer func() { _ = unix.Close(fd) }()
	buf := make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))
	pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for ctx.Err() == nil {
		n, err := unix.Poll(pfd, 200)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return // the poll loop keeps working without inotify
		}
		if n <= 0 {
			continue
		}
		r, err := unix.Read(fd, buf)
		if err != nil || r <= 0 {
			continue
		}
		hit := false
		for off := 0; off+unix.SizeofInotifyEvent <= r; {
			ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
			start := off + unix.SizeofInotifyEvent
			end := min(start+int(ev.Len), r)
			if names[string(bytes.TrimRight(buf[start:end], "\x00"))] || ev.Mask&unix.IN_Q_OVERFLOW != 0 {
				hit = true
			}
			off = end
		}
		if hit {
			select {
			case events <- struct{}{}:
			default:
			}
		}
	}
}

func (s *Store) watchLoop(ctx context.Context, events <-chan struct{}, onChange func(Settings, error)) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	pending := false
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			s.mu.Lock()
			s.notify = nil
			s.mu.Unlock()
			return
		case <-events:
			timer.Reset(s.debounce) // every event restarts the quiet period
			pending = true
		case <-ticker.C:
			if !pending && s.changedOnDisk() {
				timer.Reset(s.debounce)
				pending = true
			}
		case <-timer.C:
			pending = false
			s.reload(onChange)
		}
	}
}

func (s *Store) changedOnDisk() bool {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return false
	}
	h := sha256.Sum256(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	return h != s.lastHash
}

// reload re-reads the file and reports a change through onChange.
func (s *Store) reload(onChange func(Settings, error)) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) || err == nil && len(bytes.TrimSpace(data)) == 0 {
		return // deleted, or truncated mid-save; keep the current settings
	}
	s.mu.Lock()
	if err != nil {
		cur := s.cur.Clone()
		s.mu.Unlock()
		onChange(cur, err)
		return
	}
	h := sha256.Sum256(data)
	if h == s.lastHash {
		s.mu.Unlock()
		return // our own write, or unchanged
	}
	s.lastHash = h
	p, perr := parseSettings(data, s.path)
	if p != nil {
		s.doc, s.warnings = p.doc, p.warnings
	}
	if perr == nil {
		s.cur = p.settings
	}
	cur := s.cur.Clone()
	s.mu.Unlock()
	onChange(cur, perr)
}

package ui

import (
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

func newTestUI(t *testing.T) *appUI {
	t.Helper()
	a := test.NewTempApp(t)
	dir := t.TempDir()
	u := newAppUI(a, Deps{
		Writer:  &pad.ToolWriter{Path: filepath.Join(dir, "no-such-tool")},
		Store:   profile.NewFileStore(filepath.Join(dir, "profiles")),
		Bin:     "/usr/bin/agentpad",
		Version: "test",
		LogPath: filepath.Join(dir, "events.jsonl"),
		Addr:    "127.0.0.1:0",
	})
	u.ledFollow.Store(false) // no device writes in tests
	t.Cleanup(u.shutdown)
	return u
}

func TestEventUpdatesSessionsAndLog(t *testing.T) {
	u := newTestUI(t)
	u.onEvent(events.Event{
		Time: time.Now(), Agent: "claude", SessionID: "s1", Kind: events.KindToolPending,
		Tool: "Bash", Cwd: "/tmp/x", Message: "needs permission",
	})
	if u.tracker.Aggregate() != events.StateAttention {
		t.Fatalf("aggregate = %s", u.tracker.Aggregate())
	}
	if len(u.sessions.sessions) != 1 || u.sessions.empty.Visible() {
		t.Errorf("sessions not shown: %d", len(u.sessions.sessions))
	}
	if u.sessions.title.Text != stateTitle(events.StateAttention) {
		t.Errorf("header = %q", u.sessions.title.Text)
	}
	if u.logView.events.len() != 1 || u.logView.events.newest(0).Tool != "Bash" {
		t.Errorf("log view = %d", u.logView.events.len())
	}
}

func TestProfilesEditorSaveAndReset(t *testing.T) {
	u := newTestUI(t)
	p := u.profiles
	if len(p.profiles) == 0 || p.selected != 0 {
		t.Fatalf("profiles not loaded: %d selected=%d", len(p.profiles), p.selected)
	}
	name := p.current.Name
	builtin := p.keys[0].Text

	p.keys[0].SetText("not-a-real-key")
	p.save()
	if p.message.Text == "" || u.deps.Store.IsOverridden(name) {
		t.Fatalf("invalid key should not save, message=%q", p.message.Text)
	}

	p.keys[0].SetText("f13")
	p.save()
	if !u.deps.Store.IsOverridden(name) {
		t.Fatalf("save failed: %q", p.message.Text)
	}
	if got, _ := u.deps.Store.Get(name); got.Keys[0] != "f13" {
		t.Errorf("stored key = %q", got.Keys[0])
	}

	if err := u.deps.Store.Reset(name); err != nil {
		t.Fatal(err)
	}
	p.reload(name)
	if p.keys[0].Text != builtin {
		t.Errorf("after reset key 1 = %q, want %q", p.keys[0].Text, builtin)
	}
}

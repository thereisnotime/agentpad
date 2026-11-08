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
	if len(u.sessions.sessions) != 1 || u.sessions.empty.Visible() || !u.sessions.cards[0].content.Visible() {
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
	name := p.stored.Name
	p.selectControl(ctlKey1)
	builtin := p.entry().Text

	p.entry().SetText("not-a-real-key")
	if !p.message.Visible() {
		t.Error("invalid key should show an inline message")
	}
	p.save()
	if u.deps.Store.IsOverridden(name) {
		t.Fatal("invalid key should not save")
	}

	p.entry().SetText("f13")
	if p.view.controls[ctlKey1].action != "f13" {
		t.Errorf("drawing not updated: %q", p.view.controls[ctlKey1].action)
	}
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
	p.selectControl(ctlKey1)
	if p.entry().Text != builtin {
		t.Errorf("after reset key 1 = %q, want %q", p.entry().Text, builtin)
	}
}

func TestKnobSelectionAndQuickPick(t *testing.T) {
	u := newTestUI(t)
	p := u.profiles
	p.selectControl(ctlKnobCW)
	if !p.view.controls[ctlKnobCW].selected || p.view.controls[ctlKey1].selected {
		t.Error("selection not reflected in the drawing")
	}
	p.quickPick("volumeup")
	if p.draft.Knob.CW != "volumeup" {
		t.Errorf("knob cw = %q", p.draft.Knob.CW)
	}
	if !p.unsaved.Visible() {
		t.Error("unsaved marker should show")
	}
}

func TestRotatePersistsOrientation(t *testing.T) {
	u := newTestUI(t)
	p := u.profiles
	p.rotate(p.orient.RotateCW())
	if got := u.app.Preferences().String(prefOrientation); got != string(profile.OrientClockwise) {
		t.Errorf("orientation pref = %q", got)
	}
	if p.orientLb.Text != profile.OrientClockwise.Label() {
		t.Errorf("label = %q", p.orientLb.Text)
	}
}

func TestReceiverPillAndPolicyPrefs(t *testing.T) {
	u := newTestUI(t)
	if u.pillAgents.text.Text != "Agents idle" {
		t.Errorf("agents pill = %q", u.pillAgents.text.Text)
	}
	pol := LEDPolicy{Attention: pad.LEDSetting{Mode: 3}, Otherwise: pad.LEDSetting{Mode: 0}}
	u.setLEDPolicy(pol)
	u.loadLEDPolicy(0x8890)
	if got := *u.policy.Load(); got != pol {
		t.Errorf("policy = %+v", got)
	}
}

func TestNavigate(t *testing.T) {
	u := newTestUI(t)
	u.navigate(screenAbout)
	if !u.screens.Objects[screenAbout].Visible() || u.screens.Objects[screenProfiles].Visible() {
		t.Error("navigate did not switch screens")
	}
	if !u.nav[screenAbout][0].selected || u.nav[screenProfiles][1].selected {
		t.Error("nav items not updated")
	}
}

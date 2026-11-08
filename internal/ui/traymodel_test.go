package ui

import (
	"testing"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestTrayModelOrderAndChecks(t *testing.T) {
	m := trayModel(trayState{
		Agents: events.StateAttention, Profiles: []string{"claude-code", "codex", "media"},
		Active: "codex", Orient: profile.OrientClockwise, Follow: true,
	})
	var labels []string
	for _, it := range m {
		if it.Separator {
			labels = append(labels, "---")
		} else {
			labels = append(labels, it.Label)
		}
	}
	want := []string{trayStatus(events.StateAttention), "Jump to waiting agent", "---", "Profile", "Orientation", "LED follows agents", "---", "Show window", "Quit"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %q", labels)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, labels[i], want[i])
		}
	}
	if !m[0].Disabled || !m[1].Disabled || !m[5].Checked || !m[8].Quit {
		t.Error("status/jump disabled, follow checked, quit marked")
	}
	profiles := m[3].Children
	if len(profiles) != 3 || profiles[1].Arg != "codex" || !profiles[1].Checked || profiles[0].Checked {
		t.Errorf("profiles = %+v", profiles)
	}
	orients := m[4].Children
	if len(orients) != 4 {
		t.Fatalf("orientations = %d", len(orients))
	}
	for _, o := range orients {
		if o.Checked != (o.Arg == string(profile.OrientClockwise)) || o.Action != trayOrientation {
			t.Errorf("orientation item %+v", o)
		}
	}
}

func TestTrayModelEmptyProfiles(t *testing.T) {
	m := trayModel(trayState{Orient: profile.OrientNormal})
	if p := m[3].Children; len(p) != 1 || !p[0].Disabled {
		t.Errorf("empty profile list should show a disabled placeholder: %+v", p)
	}
	if m[5].Checked {
		t.Error("follow off")
	}
}

package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestStateColor(t *testing.T) {
	if stateColor(events.StateAttention) != colorAttention ||
		stateColor(events.StateBusy) != colorBusy ||
		stateColor(events.StateIdle) != colorIdle ||
		stateColor("weird") != colorIdle {
		t.Error("unexpected state colours")
	}
	if stateColor(events.StateAttention) == stateColor(events.StateBusy) {
		t.Error("attention and busy must differ")
	}
}

func TestShortenPath(t *testing.T) {
	home := "/home/me"
	cases := map[string]string{
		"/home/me":           "~",
		"/home/me/":          "~",
		"/home/me/src/x":     "~/src/x",
		"/home/meow/x":       "/home/meow/x",
		"/etc":               "/etc",
		"":                   "",
		"/home/me/a/../b/./": "~/b",
	}
	for in, want := range cases {
		if got := shortenPath(in, home); got != want {
			t.Errorf("shortenPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := shortenPath("/x", ""); got != "/x" {
		t.Errorf("empty home: got %q", got)
	}
}

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{0, "0 s ago"},
		{42 * time.Second, "42 s ago"},
		{3 * time.Minute, "3 min ago"},
		{5 * time.Hour, "5 h ago"},
		{50 * time.Hour, "2 d ago"},
		{-time.Second, "just now"},
	}
	for _, c := range cases {
		if got := relTime(now.Add(-c.ago), now); got != c.want {
			t.Errorf("relTime(-%v) = %q, want %q", c.ago, got, c.want)
		}
	}
	if relTime(time.Time{}, now) != "never" {
		t.Error("zero time should be never")
	}
}

func TestWriteErrMessage(t *testing.T) {
	if writeErrMessage(nil) != "" {
		t.Error("nil should be empty")
	}
	wrapped := fmt.Errorf("upload: %w", pad.ErrPermission)
	if !strings.Contains(writeErrMessage(wrapped), "udev") {
		t.Errorf("permission: %q", writeErrMessage(wrapped))
	}
	if !strings.Contains(writeErrMessage(pad.ErrToolMissing), "just tool-install") {
		t.Error("tool missing should mention just tool-install")
	}
	if !strings.Contains(writeErrMessage(pad.ErrNotConnected), "USB-A") {
		t.Error("not connected should mention USB-A")
	}
	if writeErrMessage(errors.New("boom")) != "boom" {
		t.Error("other errors pass through")
	}
}

func TestRing(t *testing.T) {
	r := newRing(3)
	for i := range 5 {
		r.add(events.Event{Message: fmt.Sprint(i)})
	}
	if r.len() != 3 {
		t.Fatalf("len = %d", r.len())
	}
	if r.newest(0).Message != "4" || r.newest(2).Message != "2" {
		t.Errorf("order wrong: %q %q", r.newest(0).Message, r.newest(2).Message)
	}
	r.clear()
	if r.len() != 0 {
		t.Error("clear")
	}
}

func TestLogCell(t *testing.T) {
	e := events.Event{
		Time: time.Now(), Agent: "claude", Kind: events.KindToolPending, Tool: "Bash",
		SessionID: "0123456789abcdef", Message: "needs\n  permission",
	}
	want := []string{e.Time.Local().Format("15:04:05"), "claude", "tool_pending", "Bash", "01234567…", "needs permission"}
	for i, w := range want {
		if got := logCell(e, i); got != w {
			t.Errorf("col %d (%s) = %q, want %q", i, logColumns[i], got, w)
		}
	}
	if logCell(e, 99) != "" {
		t.Error("out of range column")
	}
}

func TestReplaceProfile(t *testing.T) {
	list := []profile.Profile{{Name: "a"}, {Name: "b"}}
	out := replaceProfile(list, profile.Profile{Name: "b", Description: "new"})
	if out[1].Description != "new" || list[1].Description != "" {
		t.Error("replace must copy, not mutate")
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "event") != "1 event" || plural(3, "event") != "3 events" || plural(0, "event") != "0 events" {
		t.Error("plural")
	}
}

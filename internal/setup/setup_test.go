package setup

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeSnippet(t *testing.T) {
	for _, bin := range []string{"/home/u/.local/bin/agentpad", "/home/u/My Apps/agent'pad"} {
		s := ClaudeSnippet(bin)
		var doc struct {
			Hooks map[string][]struct {
				Matcher *string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal([]byte(s), &doc); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, s)
		}
		for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Notification", "PermissionRequest", "PostToolUse", "Stop", "SessionEnd"} {
			groups := doc.Hooks[ev]
			if len(groups) != 1 || len(groups[0].Hooks) != 1 {
				t.Fatalf("%s: bad groups %+v", ev, groups)
			}
			h := groups[0].Hooks[0]
			if h.Type != "command" || !strings.HasSuffix(h.Command, " hook claude") {
				t.Fatalf("%s: bad handler %+v", ev, h)
			}
		}
		if len(doc.Hooks) != 7 {
			t.Fatalf("unexpected events: %d", len(doc.Hooks))
		}
	}
	if got := ClaudeSnippet("/usr/bin/agentpad"); !strings.Contains(got, `"/usr/bin/agentpad hook claude"`) {
		t.Fatalf("plain path should be unquoted:\n%s", got)
	}
	if got := ClaudeSnippet("/a b/agentpad"); !strings.Contains(got, `"'/a b/agentpad' hook claude"`) {
		t.Fatalf("spaced path should be single-quoted:\n%s", got)
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"/usr/bin/agentpad": "/usr/bin/agentpad",
		"/a b/c":            "'/a b/c'",
		"/it's":             `'/it'\''s'`,
		"":                  "''",
		"/x;rm -rf":         "'/x;rm -rf'",
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodexSnippet(t *testing.T) {
	tests := map[string]string{
		"/usr/bin/agentpad":  `notify = ["/usr/bin/agentpad", "hook", "codex"]`,
		`C:\bin\agent"pad`:   `notify = ["C:\\bin\\agent\"pad", "hook", "codex"]`,
		"/a b/agentpad":      `notify = ["/a b/agentpad", "hook", "codex"]`,
		"/odd\x01name\nline": `notify = ["/odd\u0001name\nline", "hook", "codex"]`,
	}
	for in, want := range tests {
		if got := CodexSnippet(in); got != want {
			t.Errorf("CodexSnippet(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestOpenCodePlugin(t *testing.T) {
	js := OpenCodePlugin("127.0.0.1:47321")
	for _, want := range []string{
		`"http://127.0.0.1:47321/v1/events/opencode"`,
		"export const AgentpadPlugin = async",
		"event: async ({ event })",
		"JSON.stringify(event)",
		"AbortSignal.timeout(300)",
		"catch",
		"~/.config/opencode/plugins/agentpad.js",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("plugin missing %q", want)
		}
	}
}

func TestUdevRule(t *testing.T) {
	if !strings.Contains(UdevRule(), `ATTR{idVendor}=="1189"`) {
		t.Fatal("udev rule changed")
	}
}

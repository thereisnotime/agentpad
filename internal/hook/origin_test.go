package hook

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/thereisnotime/agentpad/internal/events"
)

type fakeProc struct {
	pid, ppid int
	comm      string
	tty       int
	argv      []string
}

func writeProcTree(t *testing.T, procs ...fakeProc) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range procs {
		dir := filepath.Join(root, strconv.Itoa(p.pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		stat := strconv.Itoa(p.pid) + " (" + p.comm + ") S " + strconv.Itoa(p.ppid) + " 1 1 " + strconv.Itoa(p.tty) + " -1 4194304 0 0"
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := strings.Join(p.argv, "\x00")
		if cmd != "" {
			cmd += "\x00"
		}
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmd), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const pts7 = 136<<8 | 7 // 34823

func TestCaptureOriginClaudeInZellijInKonsole(t *testing.T) {
	root := writeProcTree(t,
		fakeProc{900, 1, "konsole", 0, []string{"/usr/bin/konsole"}},
		fakeProc{910, 900, "zellij", pts7, []string{"zellij", "attach", "main"}},
		fakeProc{100, 1, "zellij", 0, []string{"zellij", "--server", "/run/user/1000/zellij/main"}},
		fakeProc{200, 100, "bash", pts7, []string{"/bin/bash"}},
		fakeProc{300, 200, "node", pts7, []string{"node", "/home/u/.local/bin/caveman", "claude"}},
		fakeProc{400, 300, "claude", pts7, []string{"/home/u/.local/bin/claude", "--resume"}},
		fakeProc{500, 400, "sh", pts7, []string{"/bin/sh", "-c", "agentpad hook claude"}},
		fakeProc{600, 500, "agentpad", pts7, []string{"agentpad", "hook", "claude"}},
	)
	env := []string{
		"HOME=/home/u", "KONSOLE_DBUS_SERVICE=:1.80", "KONSOLE_DBUS_WINDOW=/Windows/1",
		"KONSOLE_DBUS_SESSION=/Sessions/26", "WINDOWID=94187130383728", "ZELLIJ_SESSION_NAME=main",
		"ZELLIJ_PANE_ID=3", "ZELLIJ=0", "TERM_PROGRAM=zellij", "garbage-without-equals", "TMUX_PANE=",
	}
	got := CaptureOrigin("claude", env, root, 600)
	want := events.Origin{
		AgentPID: 400, TTY: "/dev/pts/7", TermProgram: "zellij", WindowID: "94187130383728",
		KonsoleService: ":1.80", KonsoleWindow: "/Windows/1", KonsoleSession: "/Sessions/26",
		ZellijSession: "main", ZellijPane: "3",
	}
	if got != want {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestCaptureOriginAgentMatching(t *testing.T) {
	tests := []struct {
		name  string
		agent string
		procs []fakeProc
		want  int
	}{
		{"node cli", "codex", []fakeProc{
			{10, 1, "bash", pts7, []string{"bash"}},
			{20, 10, "node", pts7, []string{"node", "/usr/lib/node_modules/@openai/codex/bin/codex.js"}},
			{30, 20, "agentpad", pts7, []string{"agentpad", "hook", "codex", "{}"}},
		}, 20},
		{"dot-prefixed comm", "opencode", []fakeProc{
			{10, 1, "bash", pts7, []string{"bash"}},
			{20, 10, ".opencode", pts7, []string{"/home/u/.opencode/bin/.opencode"}},
			{30, 20, "agentpad", pts7, nil},
		}, 20},
		{"no match falls back to parent", "claude", []fakeProc{
			{10, 1, "bash", pts7, []string{"bash"}},
			{20, 10, "sh", pts7, []string{"sh", "-c", "x"}},
			{30, 20, "agentpad", pts7, nil},
		}, 20},
		{"other agent ignored", "claude", []fakeProc{
			{10, 1, "codex", pts7, []string{"codex"}},
			{20, 10, "sh", pts7, []string{"sh"}},
			{30, 20, "agentpad", pts7, nil},
		}, 20},
		{"comm with spaces and parens", "claude", []fakeProc{
			{10, 1, "claude", pts7, nil},
			{20, 10, "weird (x) y", pts7, nil},
			{30, 20, "agentpad", pts7, nil},
		}, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeProcTree(t, tc.procs...)
			if got := CaptureOrigin(tc.agent, nil, root, 30); got.AgentPID != tc.want || got.TTY != "/dev/pts/7" {
				t.Fatalf("got %+v want pid %d", got, tc.want)
			}
		})
	}
}

func TestCaptureOriginMissingProc(t *testing.T) {
	got := CaptureOrigin("claude", []string{"TMUX=/tmp/tmux-1000/default,123,0", "TMUX_PANE=%4"}, t.TempDir(), 99)
	if got.AgentPID != 0 || got.Tmux != "/tmp/tmux-1000/default,123,0" || got.TmuxPane != "%4" {
		t.Fatalf("got %+v", got)
	}
}

func TestAncestorsBoundedAndCycleSafe(t *testing.T) {
	var procs []fakeProc
	for i := 2; i < 100; i++ {
		procs = append(procs, fakeProc{i, i + 1, "sh", 0, nil})
	}
	procs = append(procs, fakeProc{100, 50, "loop", 0, nil}) // cycle back
	root := writeProcTree(t, procs...)
	if n := len(Ancestors(root, 2)); n != maxAncestors {
		t.Fatalf("ancestors = %d", n)
	}
	root = writeProcTree(t, fakeProc{5, 6, "a", 0, nil}, fakeProc{6, 5, "b", 0, nil})
	if n := len(Ancestors(root, 5)); n != 1 {
		t.Fatalf("cycle ancestors = %d", n)
	}
}

func TestTTYName(t *testing.T) {
	tests := map[int]string{0: "", pts7: "/dev/pts/7", 136<<8 | 255: "/dev/pts/255", 137 << 8: "/dev/pts/256", 4<<8 | 2: "/dev/tty2", 4<<8 | 70: "", 1 << 20: ""}
	for nr, want := range tests {
		if got := ttyName(nr); got != want {
			t.Errorf("ttyName(%d) = %q want %q", nr, got, want)
		}
	}
}

func TestCaptureOriginSelf(t *testing.T) {
	// Real /proc, own process: must not panic and must find a parent.
	if o := CaptureOrigin("claude", os.Environ(), "/proc", os.Getpid()); o.AgentPID == 0 {
		t.Fatal("no agent pid from real /proc")
	}
}

func TestRunSendsCapturedOrigin(t *testing.T) {
	root := writeProcTree(t,
		fakeProc{10, 1, "claude", pts7, []string{"claude"}},
		fakeProc{20, 10, "agentpad", pts7, []string{"agentpad", "hook", "claude"}},
	)
	oldEnv, oldRoot, oldPID := environ, procRoot, selfPID
	environ = func() []string { return []string{"KONSOLE_DBUS_SESSION=/Sessions/4", "TMUX_PANE=%1"} }
	procRoot, selfPID = root, func() int { return 20 }
	defer func() { environ, procRoot, selfPID = oldEnv, oldRoot, oldPID }()

	r, addr := newRecv(t)
	Run("claude", nil, strings.NewReader(`not json`), addr)
	r.mu.Lock()
	defer r.mu.Unlock()
	want := `{"payload":"not json","origin":{"agent_pid":10,"tty":"/dev/pts/7","konsole_session":"/Sessions/4","tmux_pane":"%1"}}`
	if len(r.body) != 1 || r.body[0] != want {
		t.Fatalf("body = %v\nwant   %s", r.body, want)
	}
}

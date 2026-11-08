package scripts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestRunner(t *testing.T, scripts map[string]Script) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	r := NewRunner(Options{LogPath: filepath.Join(dir, "state", "scripts.jsonl"),
		Environ: []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}, KillGrace: 300 * time.Millisecond})
	r.homeDir = func() (string, error) { return dir, nil }
	r.SetScripts(scripts)
	t.Cleanup(r.Close)
	return r, dir
}

func runAndWait(t *testing.T, r *Runner, name string, p Payload) Run {
	t.Helper()
	id := r.Run(context.Background(), name, p)
	r.Wait()
	for _, run := range r.Runs() {
		if run.ID == id {
			return run
		}
	}
	t.Fatalf("run %d not recorded", id)
	return Run{}
}

func TestRunTrueAndFalse(t *testing.T) {
	r, _ := newTestRunner(t, map[string]Script{
		"ok":   {Run: []string{"/bin/true"}},
		"fail": {Run: []string{"/bin/sh", "-c", "echo oops >&2; exit 3"}},
	})
	if run := runAndWait(t, r, "ok", Payload{Event: "manual"}); run.Status != StatusOK || run.ExitCode != 0 {
		t.Fatalf("ok: %+v", run)
	}
	run := runAndWait(t, r, "fail", Payload{Event: "manual"})
	if run.Status != StatusFailed || run.ExitCode != 3 || strings.TrimSpace(run.Stderr) != "oops" {
		t.Fatalf("fail: %+v", run)
	}
	if last, ok := r.Last("ok"); !ok || last.Status != StatusOK {
		t.Errorf("Last(ok) = %+v %v", last, ok)
	}
}

func TestRunEnvAndStdin(t *testing.T) {
	r, dir := newTestRunner(t, map[string]Script{
		"env":   {Run: []string{"printenv", "AGENTPAD_EVENT", "AGENTPAD_AGENT", "AGENTPAD_TOOL", "STAGE"}, Env: map[string]string{"STAGE": "prod"}},
		"stdin": {Run: []string{"cat"}},
		"pwd":   {Shell: "pwd", Cwd: "~/sub"},
		"home":  {Run: []string{"~/bin/hello"}},
	})
	run := runAndWait(t, r, "env", Payload{Event: "attention", Agent: "claude", Tool: "Bash"})
	if run.Status != StatusOK || run.Stdout != "attention\nclaude\nBash\nprod\n" {
		t.Fatalf("env: %+v", run)
	}
	run = runAndWait(t, r, "stdin", Payload{Event: "gesture", Control: "key2", Gesture: "hold", Profile: "codex"})
	var p Payload
	if err := json.Unmarshal([]byte(run.Stdout), &p); err != nil {
		t.Fatalf("stdin is not JSON: %q: %v", run.Stdout, err)
	}
	if p.Event != "gesture" || p.Control != "key2" || p.Gesture != "hold" || p.Profile != "codex" || p.Time.IsZero() {
		t.Errorf("stdin payload = %+v", p)
	}
	if run := runAndWait(t, r, "pwd", Payload{}); run.Status != StatusError || !strings.Contains(run.Err, "not a directory") {
		t.Errorf("missing cwd: %+v", run)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if run := runAndWait(t, r, "pwd", Payload{}); run.Status != StatusOK || strings.TrimSpace(run.Stdout) != filepath.Join(dir, "sub") {
		t.Errorf("cwd ~ expansion: %+v", run)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "hello"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if run := runAndWait(t, r, "home", Payload{}); run.Status != StatusOK || run.Stdout != "hi\n" {
		t.Errorf("run[0] ~ expansion: %+v", run)
	}
}

func TestRunNoShellByDefault(t *testing.T) {
	r, _ := newTestRunner(t, map[string]Script{
		"argv": {Run: []string{"printenv", "$HOME;", "true"}},
	})
	// printenv of a variable named "$HOME;" exits 1 and nothing is expanded
	if run := runAndWait(t, r, "argv", Payload{}); run.Status != StatusFailed || run.Stdout != "" {
		t.Fatalf("argv ran through a shell? %+v", run)
	}
}

func TestRunTimeoutKillsGroup(t *testing.T) {
	r, dir := newTestRunner(t, nil)
	marker := filepath.Join(dir, "child-alive")
	// the child of the shell must die too: it would touch marker after 1.5s
	r.SetScripts(map[string]Script{
		"slow": {Run: []string{"/bin/sh", "-c", "(sleep 1.5; touch " + marker + ") & sleep 5"}, Timeout: 200 * time.Millisecond},
	})
	start := time.Now()
	run := runAndWait(t, r, "slow", Payload{})
	if run.Status != StatusTimeout {
		t.Fatalf("status = %s (%+v)", run.Status, run)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("timeout took %s", el)
	}
	time.Sleep(1800 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a process of the group survived the timeout")
	}
}

func TestRunSingleFlight(t *testing.T) {
	r, _ := newTestRunner(t, map[string]Script{
		"slow": {Run: []string{"/bin/sh", "-c", "sleep 5"}, Timeout: 300 * time.Millisecond},
	})
	first := r.Run(context.Background(), "slow", Payload{})
	second := r.Run(context.Background(), "slow", Payload{})
	r.Wait()
	got := map[RunID]Status{}
	for _, run := range r.Runs() {
		got[run.ID] = run.Status
	}
	if got[first] != StatusTimeout || got[second] != StatusSkipped {
		t.Fatalf("statuses = %v", got)
	}
	if r.Running("slow") {
		t.Error("still marked running")
	}
}

func TestRunUnknownAndConcurrencyLimit(t *testing.T) {
	r, _ := newTestRunner(t, nil)
	r.opts.MaxConcurrent = 2
	r.sem = make(chan struct{}, 2)
	scripts := map[string]Script{}
	for _, n := range []string{"a", "b", "c"} {
		scripts[n] = Script{Run: []string{"/bin/sh", "-c", "sleep 0.3"}}
	}
	r.SetScripts(scripts)
	var mu sync.Mutex
	maxRunning, running := 0, 0
	r.OnRun(func(run Run) {
		mu.Lock()
		defer mu.Unlock()
		switch run.Status {
		case StatusRunning:
			running++
			maxRunning = max(maxRunning, running)
		case StatusOK:
			running--
		}
	})
	for _, n := range []string{"a", "b", "c"} {
		r.Run(context.Background(), n, Payload{})
	}
	if run := runAndWait(t, r, "nope", Payload{}); run.Status != StatusError || !strings.Contains(run.Err, "unknown script") {
		t.Errorf("unknown: %+v", run)
	}
	if maxRunning != 2 {
		t.Errorf("max concurrent = %d, want 2", maxRunning)
	}
}

func TestRunOutputCapAndLog(t *testing.T) {
	r, _ := newTestRunner(t, map[string]Script{
		"big": {Run: []string{"/bin/sh", "-c", "head -c 100000 /dev/zero"}},
	})
	run := runAndWait(t, r, "big", Payload{Trigger: "Run now"})
	if len(run.Stdout) != OutputCap || !run.Truncated {
		t.Fatalf("stdout len %d truncated %v", len(run.Stdout), run.Truncated)
	}
	r2 := NewRunner(Options{LogPath: r.opts.LogPath})
	if err := r2.Load(); err != nil {
		t.Fatal(err)
	}
	runs := r2.Runs()
	if len(runs) != 1 || runs[0].Script != "big" || runs[0].Trigger != "Run now" || runs[0].Status != StatusOK {
		t.Fatalf("loaded %+v", runs)
	}
}

func TestRunsKeepsLast50(t *testing.T) {
	r, _ := newTestRunner(t, nil)
	for range KeepRuns + 10 {
		r.Run(context.Background(), "missing", Payload{})
	}
	runs := r.Runs()
	if len(runs) != KeepRuns || runs[0].ID != KeepRuns+10 {
		t.Fatalf("len %d newest %d", len(runs), runs[0].ID)
	}
}

func TestCloseCancels(t *testing.T) {
	r, _ := newTestRunner(t, map[string]Script{"slow": {Run: []string{"/bin/sh", "-c", "sleep 5"}}})
	r.Run(context.Background(), "slow", Payload{})
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	r.Close()
	if time.Since(start) > 3*time.Second {
		t.Error("Close waited for the script")
	}
	if run := r.Runs()[0]; run.Status != StatusTimeout || run.Err != "cancelled" {
		t.Errorf("after close: %+v", run)
	}
	if id := r.Run(context.Background(), "slow", Payload{}); r.Runs()[0].ID != id || r.Runs()[0].Status != StatusError {
		t.Error("run after close was started")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		s   Script
		bad string
	}{
		{Script{Run: []string{"a"}, Shell: "b"}, "not both"},
		{Script{}, "needs run"},
		{Script{Run: []string{""}}, "empty"},
		{Script{Shell: "x", Timeout: 11 * time.Minute}, "longer"},
		{Script{Shell: "x", Timeout: -time.Second}, "negative"},
		{Script{Shell: "x", Env: map[string]string{"1X": "y"}}, "variable name"},
		{Script{Shell: "x", Env: map[string]string{"OK_1": "y"}, Timeout: time.Minute}, ""},
	} {
		err := tc.s.Validate()
		if (tc.bad == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.bad)) {
			t.Errorf("%+v: err = %v, want %q", tc.s, err, tc.bad)
		}
	}
	scripts := map[string]Script{"x": {Shell: "x"}}
	if err := (Hook{On: "attention", Agent: "claude", Script: "x"}).Validate(scripts); err != nil {
		t.Error(err)
	}
	for _, h := range []Hook{{On: "nope", Script: "x"}, {On: "busy", Agent: "gpt", Script: "x"}, {On: "busy", Script: "y"}, {On: "busy"}} {
		if h.Validate(scripts) == nil {
			t.Errorf("%+v accepted", h)
		}
	}
	if got := (Script{Run: []string{"notify-send", "agent waits"}}).Preview(); got != "notify-send 'agent waits'" {
		t.Errorf("preview %q", got)
	}
}

package events

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogAppendTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "events.jsonl")
	l, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		e := Normalize(AgentClaude, []byte(`{"hook_event_name":"Stop","session_id":"s"}`), t0.Add(time.Duration(i)*time.Second))
		if err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := l.Tail(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].Time.Equal(t0.Add(2*time.Second)) || !got[2].Time.Equal(t0.Add(4*time.Second)) {
		t.Fatalf("tail = %+v", got)
	}
	if got[0].Kind != KindIdle || got[0].SessionID != "s" || len(got[0].Raw) == 0 {
		t.Fatalf("roundtrip lost fields: %+v", got[0])
	}
	all, _ := l.Tail(100)
	if len(all) != 5 {
		t.Fatalf("tail(100) = %d", len(all))
	}
	if none, _ := l.Tail(0); len(none) != 0 {
		t.Fatal("tail(0) should be empty")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm()&0o600 != 0o600 {
		t.Fatalf("mode = %v", fi.Mode())
	}
}

func TestLogPreservesExistingAndSkipsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	existing := `{"time":"2026-01-01T00:00:00Z","agent":"codex","session_id":"a","kind":"idle","name":"x"}` + "\n" +
		"{corrupt line\n" +
		"\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{Agent: AgentClaude, SessionID: "b", Kind: KindPrompt, Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	got, err := l.Tail(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != "a" || got[1].SessionID != "b" {
		t.Fatalf("got %+v", got)
	}
	_ = l.Close()
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), existing) {
		t.Fatal("existing lines changed")
	}
	if err := l.Append(Event{}); err == nil {
		t.Fatal("append after close should fail")
	}
	if err := l.Close(); err != nil {
		t.Fatal("double close should be fine")
	}
}

func TestLogConcurrentAppend(t *testing.T) {
	l, err := OpenLog(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = l.Append(Event{Agent: AgentClaude, SessionID: "s", Kind: KindOther, Message: strings.Repeat("x", 500)})
			}
		}()
	}
	wg.Wait()
	got, err := l.Tail(1000)
	if err != nil || len(got) != 500 {
		t.Fatalf("got %d events, err %v", len(got), err)
	}
}

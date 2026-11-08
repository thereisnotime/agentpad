package notify

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/thereisnotime/agentpad/internal/events"
)

type call struct {
	method string
	args   []any
}

type fakeBus struct {
	mu     sync.Mutex
	calls  []call
	nextID uint32
	err    error
	block  bool
	sigs   chan Signal
}

func newFakeBus() *fakeBus { return &fakeBus{sigs: make(chan Signal, 4)} }

func (b *fakeBus) Call(ctx context.Context, method string, args ...any) ([]any, error) {
	b.mu.Lock()
	b.calls = append(b.calls, call{method, args})
	block, err := b.block, b.err
	b.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if method == iface+".Notify" {
		b.mu.Lock()
		defer b.mu.Unlock()
		if r := args[1].(uint32); r != 0 {
			return []any{r}, nil
		}
		b.nextID++
		return []any{b.nextID}, nil
	}
	return nil, nil
}

func (b *fakeBus) Signals() <-chan Signal { return b.sigs }
func (b *fakeBus) Close() error           { close(b.sigs); return nil }

func (b *fakeBus) last() call {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[len(b.calls)-1]
}

func TestNotifyCallAndReplace(t *testing.T) {
	b := newFakeBus()
	n := NewWithBus(b, nil)
	defer func() { _ = n.Close() }()

	s := events.Session{Agent: events.AgentClaude, SessionID: "s1", Cwd: "/home/u/src/agentpad", LastTool: "Bash"}
	if err := n.Notify(ForSession(s)); err != nil {
		t.Fatal(err)
	}
	c := b.last()
	if c.method != "org.freedesktop.Notifications.Notify" || len(c.args) != 8 {
		t.Fatalf("call %+v", c)
	}
	if c.args[0] != "agentpad" || c.args[1] != uint32(0) || c.args[3] != "Claude Code needs you" ||
		c.args[4] != "Bash in agentpad (/home/u/src/agentpad)" || c.args[7] != int32(-1) {
		t.Fatalf("args %#v", c.args)
	}
	actions := c.args[5].([]string)
	if len(actions) != 4 || actions[0] != "default" || actions[2] != ActionJump || actions[3] != "Jump" {
		t.Fatalf("actions %v", actions)
	}
	hints := c.args[6].(map[string]dbus.Variant)
	if hints["urgency"].Value() != byte(1) || hints["desktop-entry"].Value() != "dev.thereisnotime.agentpad" {
		t.Fatalf("hints %v", hints)
	}

	// Same session reuses the id; another session gets a new one.
	_ = n.Notify(ForSession(s))
	if b.last().args[1] != uint32(1) {
		t.Fatalf("replace id %v", b.last().args[1])
	}
	_ = n.Notify(ForSession(events.Session{Agent: events.AgentCodex, SessionID: "x"}))
	if c := b.last(); c.args[1] != uint32(0) || c.args[3] != "Codex needs you" || c.args[4] != "waiting for you" {
		t.Fatalf("second session %#v", c.args)
	}

	n.Dismiss(s.Key())
	if c := b.last(); c.method != iface+".CloseNotification" || c.args[0] != uint32(1) {
		t.Fatalf("dismiss %+v", c)
	}
	calls := len(b.calls)
	n.Dismiss(s.Key()) // already gone: no call
	if len(b.calls) != calls {
		t.Fatal("dismiss of unknown key called the bus")
	}
	_ = n.Notify(ForSession(s))
	if b.last().args[1] != uint32(0) {
		t.Fatal("dismissed key should not replace")
	}
}

func TestNotifyEscapesBody(t *testing.T) {
	b := newFakeBus()
	n := NewWithBus(b, nil)
	defer func() { _ = n.Close() }()
	_ = n.Notify(Notification{Key: "k", Summary: "s", Body: "a <b> & c"})
	if got := b.last().args[4]; got != "a &lt;b&gt; &amp; c" {
		t.Fatalf("body %q", got)
	}
}

func TestNotifyTimeoutAndErrors(t *testing.T) {
	b := newFakeBus()
	b.block = true
	n := NewWithBus(b, nil)
	defer func() { _ = n.Close() }()
	start := time.Now()
	if err := n.Notify(Notification{Key: "k"}); err == nil {
		t.Fatal("expected timeout")
	}
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("Notify blocked %v", d)
	}
	b.mu.Lock()
	b.block, b.err = false, errors.New("no server")
	b.mu.Unlock()
	if err := n.Notify(Notification{Key: "k"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestActionInvoked(t *testing.T) {
	b := newFakeBus()
	got := make(chan string, 4)
	n := NewWithBus(b, func(k string) { got <- k })
	s := events.Session{Agent: events.AgentClaude, SessionID: "s1"}
	_ = n.Notify(ForSession(s))

	b.sigs <- Signal{Member: "ActionInvoked", Body: []any{uint32(99), ActionJump}} // not ours
	b.sigs <- Signal{Member: "ActionInvoked", Body: []any{uint32(1), "other"}}     // unknown action
	b.sigs <- Signal{Member: "ActionInvoked", Body: []any{"bad"}}                  // malformed
	b.sigs <- Signal{Member: "ActionInvoked", Body: []any{uint32(1), ActionJump}}
	select {
	case k := <-got:
		if k != "claude/s1" {
			t.Fatalf("key %q", k)
		}
	case <-time.After(time.Second):
		t.Fatal("no jump callback")
	}
	b.sigs <- Signal{Member: "ActionInvoked", Body: []any{uint32(1), "default"}}
	if k := <-got; k != "claude/s1" {
		t.Fatalf("default action key %q", k)
	}

	// Closed by the server: the id is forgotten and not reused.
	b.sigs <- Signal{Member: "NotificationClosed", Body: []any{uint32(1), uint32(2)}}
	_ = n.Close() // drains the signal loop
	if len(got) != 0 {
		t.Fatal("unexpected callbacks")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.ids) != 0 || len(n.keys) != 0 {
		t.Fatalf("ids %v keys %v", n.ids, n.keys)
	}
}

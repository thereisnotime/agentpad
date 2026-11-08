// Package notify shows desktop notifications for sessions that wait for the
// user, with a "Jump" action, over org.freedesktop.Notifications.
//
// # API for the UI
//
//	n, err := notify.New(func(key string) { /* jump to session key */ })
//	defer n.Close()
//	_ = n.Notify(notify.ForSession(s)) // s: events.Session entering attention
//	n.Dismiss(s.Key())                 // when it stops waiting
//
// Each session key reuses its notification (replaces_id), so repeated
// prompts update one bubble instead of stacking. Notify never blocks for more
// than Timeout (200ms by default); clicking Jump (or the bubble itself) calls
// the OnJump callback with the session key on the signal goroutine.
package notify

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/thereisnotime/agentpad/internal/config"
	"github.com/thereisnotime/agentpad/internal/events"
)

const (
	service = "org.freedesktop.Notifications"
	path    = "/org/freedesktop/Notifications"
	iface   = "org.freedesktop.Notifications"

	// ActionJump is the action key of the Jump button.
	ActionJump = "jump"
	// DefaultTimeout bounds every Notify/Dismiss call.
	DefaultTimeout = 200 * time.Millisecond
)

// Signal is a notification-server signal (ActionInvoked, NotificationClosed).
type Signal struct {
	Member string // "ActionInvoked" or "NotificationClosed"
	Body   []any
}

// Bus is what the notifier needs from the session bus; all calls target
// org.freedesktop.Notifications at /org/freedesktop/Notifications.
type Bus interface {
	Call(ctx context.Context, method string, args ...any) ([]any, error)
	// Signals delivers ActionInvoked/NotificationClosed; closed on Close.
	Signals() <-chan Signal
	Close() error
}

// Notification is one bubble.
type Notification struct {
	Key     string // session key; notifications with the same key replace each other
	Summary string
	Body    string // plain text; escaped for servers that render markup
	Icon    string // icon name or path; "" for the app default
	Urgency byte   // 0 low, 1 normal, 2 critical
}

// Notifier sends notifications and dispatches Jump clicks.
type Notifier struct {
	// OnJump is called with the session key when Jump (or the bubble) is clicked.
	OnJump func(key string)
	// Timeout bounds each D-Bus call (default 200ms).
	Timeout time.Duration
	// AppName is shown by the notification server (default "agentpad").
	AppName string

	bus  Bus
	mu   sync.Mutex
	ids  map[string]uint32 // key -> notification id
	keys map[uint32]string // notification id -> key
	done chan struct{}
}

// New connects to the session bus and starts listening for actions.
func New(onJump func(key string)) (*Notifier, error) {
	b, err := ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	return NewWithBus(b, onJump), nil
}

// NewWithBus builds a notifier on an existing Bus (tests, shared connections).
func NewWithBus(b Bus, onJump func(key string)) *Notifier {
	n := &Notifier{
		OnJump:  onJump,
		Timeout: DefaultTimeout,
		AppName: "agentpad",
		bus:     b,
		ids:     map[string]uint32{},
		keys:    map[uint32]string{},
		done:    make(chan struct{}),
	}
	go n.loop()
	return n
}

// Close stops listening and closes the bus.
func (n *Notifier) Close() error {
	err := n.bus.Close()
	<-n.done
	return err
}

func (n *Notifier) timeout() time.Duration {
	if n.Timeout > 0 {
		return n.Timeout
	}
	return DefaultTimeout
}

// Notify shows or replaces the notification for nt.Key.
func (n *Notifier) Notify(nt Notification) error {
	n.mu.Lock()
	replace := n.ids[nt.Key]
	n.mu.Unlock()

	hints := map[string]dbus.Variant{
		"urgency":       dbus.MakeVariant(nt.Urgency),
		"desktop-entry": dbus.MakeVariant(config.AppID),
		"category":      dbus.MakeVariant("im"),
	}
	actions := []string{"default", "Jump", ActionJump, "Jump"}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout())
	defer cancel()
	body, err := n.bus.Call(ctx, iface+".Notify",
		n.AppName, replace, nt.Icon, nt.Summary, escape(nt.Body), actions, hints, int32(-1))
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	if len(body) != 1 {
		return errors.New("notify: unexpected reply")
	}
	id, ok := body[0].(uint32)
	if !ok {
		return errors.New("notify: unexpected reply type")
	}
	n.mu.Lock()
	if replace != 0 && replace != id {
		delete(n.keys, replace)
	}
	n.ids[nt.Key] = id
	n.keys[id] = nt.Key
	n.mu.Unlock()
	return nil
}

// Dismiss closes the notification for key, if any.
func (n *Notifier) Dismiss(key string) {
	n.mu.Lock()
	id, ok := n.ids[key]
	if ok {
		delete(n.ids, key)
		delete(n.keys, id)
	}
	n.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout())
	defer cancel()
	_, _ = n.bus.Call(ctx, iface+".CloseNotification", id)
}

func (n *Notifier) loop() {
	defer close(n.done)
	for sig := range n.bus.Signals() {
		if len(sig.Body) == 0 {
			continue
		}
		id, ok := sig.Body[0].(uint32)
		if !ok {
			continue
		}
		switch sig.Member {
		case "ActionInvoked":
			action, _ := bodyString(sig.Body, 1)
			n.mu.Lock()
			key, known := n.keys[id]
			n.mu.Unlock()
			if known && (action == ActionJump || action == "default") && n.OnJump != nil {
				n.OnJump(key)
			}
		case "NotificationClosed":
			n.mu.Lock()
			if key, known := n.keys[id]; known {
				delete(n.keys, id)
				if n.ids[key] == id {
					delete(n.ids, key)
				}
			}
			n.mu.Unlock()
		}
	}
}

func bodyString(b []any, i int) (string, bool) {
	if i >= len(b) {
		return "", false
	}
	s, ok := b[i].(string)
	return s, ok
}

var markup = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escape(s string) string { return markup.Replace(s) }

var agentNames = map[string]string{
	events.AgentClaude:   "Claude Code",
	events.AgentCodex:    "Codex",
	events.AgentOpenCode: "OpenCode",
}

// ForSession builds the notification for a session that waits for the user:
// "Claude Code needs you" / "Bash in agentpad (~/src/agentpad)".
func ForSession(s events.Session) Notification {
	name := agentNames[s.Agent]
	if name == "" {
		name = s.Agent
	}
	var parts []string
	if s.LastTool != "" {
		parts = append(parts, s.LastTool)
	}
	if s.Cwd != "" {
		parts = append(parts, "in "+filepath.Base(s.Cwd)+" ("+s.Cwd+")")
	}
	body := strings.Join(parts, " ")
	if body == "" {
		body = "waiting for you"
	}
	return Notification{Key: s.Key(), Summary: name + " needs you", Body: body, Urgency: 1}
}

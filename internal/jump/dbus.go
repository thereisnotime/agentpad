package jump

import (
	"context"
	"errors"
	"sync"

	"github.com/godbus/dbus/v5"
)

// callbackPath is where SessionBus exports the KWin feedback receiver.
const callbackPath = "/dev/thereisnotime/agentpad/Jump"

// SessionBus is a private session-bus connection implementing Bus.
type SessionBus struct {
	conn *dbus.Conn

	mu       sync.Mutex
	exported bool
	waiters  map[string]chan []any
}

// ConnectSessionBus opens a private connection to the session bus.
func ConnectSessionBus() (*SessionBus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	return &SessionBus{conn: conn, waiters: map[string]chan []any{}}, nil
}

// Call implements Bus.
func (b *SessionBus) Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
	call := b.conn.Object(dest, dbus.ObjectPath(path)).CallWithContext(ctx, method, 0, args...)
	return call.Body, call.Err
}

// Callback implements Bus.
func (b *SessionBus) Callback(token string) (string, string, <-chan []any, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.exported {
		if err := b.conn.Export(callbackReceiver{b}, callbackPath, CallbackInterface); err != nil {
			return "", "", nil, nil, err
		}
		b.exported = true
	}
	if _, dup := b.waiters[token]; dup {
		return "", "", nil, nil, errors.New("duplicate callback token")
	}
	ch := make(chan []any, 1)
	b.waiters[token] = ch
	cancel := func() {
		b.mu.Lock()
		delete(b.waiters, token)
		b.mu.Unlock()
	}
	return b.conn.Names()[0], callbackPath, ch, cancel, nil
}

func (b *SessionBus) deliver(token string, args []any) {
	b.mu.Lock()
	ch := b.waiters[token]
	delete(b.waiters, token)
	b.mu.Unlock()
	if ch != nil {
		ch <- args
	}
}

// Close implements Bus.
func (b *SessionBus) Close() error { return b.conn.Close() }

type callbackReceiver struct{ b *SessionBus }

// Result is called by the KWin script: token, whether a window was found,
// and its caption.
func (r callbackReceiver) Result(token string, found bool, caption string) *dbus.Error {
	r.b.deliver(token, []any{token, found, caption})
	return nil
}

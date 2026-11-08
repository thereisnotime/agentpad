package notify

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"
)

// SessionBus is a private session-bus connection implementing Bus.
type SessionBus struct {
	conn *dbus.Conn
	raw  chan *dbus.Signal
	out  chan Signal
	once sync.Once
}

// ConnectSessionBus connects and subscribes to the notification signals.
func ConnectSessionBus() (*SessionBus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		if err := conn.AddMatchSignal(
			dbus.WithMatchInterface(iface),
			dbus.WithMatchObjectPath(path),
			dbus.WithMatchMember(member),
		); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	b := &SessionBus{conn: conn, raw: make(chan *dbus.Signal, 16), out: make(chan Signal, 16)}
	conn.Signal(b.raw)
	go b.pump()
	return b, nil
}

func (b *SessionBus) pump() {
	defer close(b.out)
	for s := range b.raw {
		if s == nil {
			continue
		}
		switch s.Name {
		case iface + ".ActionInvoked":
			b.out <- Signal{Member: "ActionInvoked", Body: s.Body}
		case iface + ".NotificationClosed":
			b.out <- Signal{Member: "NotificationClosed", Body: s.Body}
		}
	}
}

// Call implements Bus.
func (b *SessionBus) Call(ctx context.Context, method string, args ...any) ([]any, error) {
	call := b.conn.Object(service, path).CallWithContext(ctx, method, 0, args...)
	return call.Body, call.Err
}

// Signals implements Bus.
func (b *SessionBus) Signals() <-chan Signal { return b.out }

// Close implements Bus. godbus closes the signal channel on Close, which ends
// the pump and closes Signals.
func (b *SessionBus) Close() error {
	var err error
	b.once.Do(func() { err = b.conn.Close() })
	return err
}

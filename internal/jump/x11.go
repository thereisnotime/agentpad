package jump

import (
	"errors"
	"fmt"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// X11Conn is a pure-Go X11 connection implementing X11.
type X11Conn struct {
	mu    sync.Mutex
	conn  *xgb.Conn
	root  xproto.Window
	atoms map[string]xproto.Atom
}

// DialX11 connects to $DISPLAY.
func DialX11() (*X11Conn, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	setup := xproto.Setup(c)
	if setup == nil || len(setup.Roots) == 0 {
		c.Close()
		return nil, errors.New("x11: no screens")
	}
	return &X11Conn{conn: c, root: setup.DefaultScreen(c).Root, atoms: map[string]xproto.Atom{}}, nil
}

// Close closes the connection.
func (x *X11Conn) Close() { x.conn.Close() }

func (x *X11Conn) atom(name string) (xproto.Atom, error) {
	if a, ok := x.atoms[name]; ok {
		return a, nil
	}
	r, err := xproto.InternAtom(x.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("x11: intern %s: %w", name, err)
	}
	x.atoms[name] = r.Atom
	return r.Atom, nil
}

func (x *X11Conn) cardinals(win xproto.Window, prop string) ([]uint32, error) {
	a, err := x.atom(prop)
	if err != nil {
		return nil, err
	}
	r, err := xproto.GetProperty(x.conn, false, win, a, xproto.AtomAny, 0, 1<<16).Reply()
	if err != nil {
		return nil, err
	}
	if r.Format != 32 {
		return nil, nil
	}
	out := make([]uint32, 0, r.ValueLen)
	for i := 0; i+4 <= len(r.Value); i += 4 {
		out = append(out, xgb.Get32(r.Value[i:]))
	}
	return out, nil
}

// findByPID returns the first managed client whose _NET_WM_PID is in pids,
// preferring earlier pids.
func (x *X11Conn) findByPID(pids []int) (uint32, error) {
	clients, err := x.cardinals(x.root, "_NET_CLIENT_LIST")
	if err != nil {
		return 0, fmt.Errorf("x11: _NET_CLIENT_LIST: %w", err)
	}
	owner := map[int]uint32{}
	for _, w := range clients {
		v, err := x.cardinals(xproto.Window(w), "_NET_WM_PID")
		if err != nil || len(v) == 0 {
			continue
		}
		if _, seen := owner[int(v[0])]; !seen {
			owner[int(v[0])] = w
		}
	}
	for _, p := range pids {
		if w, ok := owner[p]; ok {
			return w, nil
		}
	}
	return 0, fmt.Errorf("x11: no client window for pids %v", pids)
}

// Activate implements X11: a _NET_ACTIVE_WINDOW client message with source
// indication 2 (pager), which window managers honour over focus stealing
// prevention.
func (x *X11Conn) Activate(win uint32, pids []int) (uint32, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if win == 0 {
		w, err := x.findByPID(pids)
		if err != nil {
			return 0, err
		}
		win = w
	}
	a, err := x.atom("_NET_ACTIVE_WINDOW")
	if err != nil {
		return 0, err
	}
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: xproto.Window(win),
		Type:   a,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{2, xproto.TimeCurrentTime, 0, 0, 0}),
	}
	mask := uint32(xproto.EventMaskSubstructureRedirect | xproto.EventMaskSubstructureNotify)
	if err := xproto.SendEventChecked(x.conn, false, x.root, mask, string(ev.Bytes())).Check(); err != nil {
		return 0, fmt.Errorf("x11: send _NET_ACTIVE_WINDOW: %w", err)
	}
	return win, nil
}

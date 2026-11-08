package jump

import (
	"context"
	"strconv"
	"strings"

	"github.com/thereisnotime/agentpad/internal/events"
)

const (
	konsoleSetCurrent = "org.kde.konsole.Window.setCurrentSession"
	konsoleTitle      = "org.kde.konsole.Session.title"
	dbusPID           = "org.freedesktop.DBus.GetConnectionUnixProcessID"
)

// konsoleSessionNumber parses "/Sessions/26" -> 26.
func konsoleSessionNumber(path string) (int, bool) {
	rest, ok := strings.CutPrefix(path, "/Sessions/")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n >= 0
}

// konsoleTab selects the Konsole tab. It also returns the tab's displayed
// title (a hint for picking the right window when one Konsole process owns
// several) and the Konsole process ID, both zero-valued when unknown.
func (j *Jumper) konsoleTab(ctx context.Context, o events.Origin) (Step, string, int) {
	const be = "konsole"
	if o.KonsoleService == "" && o.KonsoleSession == "" {
		return skipped(StepTerminalTab, "", "not a Konsole terminal (no KONSOLE_DBUS_* in origin)"), "", 0
	}
	if o.KonsoleService == "" || o.KonsoleWindow == "" {
		return skipped(StepTerminalTab, be, "origin lacks KONSOLE_DBUS_SERVICE or KONSOLE_DBUS_WINDOW"), "", 0
	}
	n, ok := konsoleSessionNumber(o.KonsoleSession)
	if !ok {
		return skipped(StepTerminalTab, be, "unusable KONSOLE_DBUS_SESSION %q", o.KonsoleSession), "", 0
	}
	bus, err := j.bus()
	if err != nil {
		return failed(StepTerminalTab, be, "%v", err), "", 0
	}

	pid := 0
	if body, err := bus.Call(ctx, "org.freedesktop.DBus", "/org/freedesktop/DBus", dbusPID, o.KonsoleService); err == nil && len(body) == 1 {
		if v, ok := body[0].(uint32); ok {
			pid = int(v)
		}
	}
	if _, err := bus.Call(ctx, o.KonsoleService, o.KonsoleWindow, konsoleSetCurrent, int32(n)); err != nil {
		return failed(StepTerminalTab, be, "setCurrentSession(%d) on %s %s: %v", n, o.KonsoleService, o.KonsoleWindow, err), "", pid
	}
	hint := ""
	if body, err := bus.Call(ctx, o.KonsoleService, o.KonsoleSession, konsoleTitle, int32(1)); err == nil && len(body) == 1 {
		hint, _ = body[0].(string)
	}
	return done(StepTerminalTab, be, "selected session %d in %s", n, o.KonsoleWindow), hint, pid
}

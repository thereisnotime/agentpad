package jump

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
)

const (
	kwinService  = "org.kde.KWin"
	kwinPath     = "/Scripting"
	kwinLoad     = "org.kde.kwin.Scripting.loadScript"
	kwinStart    = "org.kde.kwin.Scripting.start"
	kwinUnload   = "org.kde.kwin.Scripting.unloadScript"
	kwinUnloadTO = time.Second

	// CallbackInterface is the D-Bus interface KWin scripts call back on.
	CallbackInterface = "dev.thereisnotime.agentpad.Jump"
)

// DetectWindowBackend picks the window backend from the desktop session:
// X11 when XDG_SESSION_TYPE=x11 (or DISPLAY is set without WAYLAND_DISPLAY),
// KWin on a KDE Wayland session, none otherwise.
func DetectWindowBackend(getenv func(string) string) WindowBackend {
	if getenv == nil {
		return WindowNone
	}
	typ := getenv("XDG_SESSION_TYPE")
	kde := strings.Contains(strings.ToUpper(getenv("XDG_CURRENT_DESKTOP")), "KDE")
	switch {
	case typ == "x11":
		return WindowX11
	case typ == "wayland" && kde:
		return WindowKWin
	case typ == "" && getenv("WAYLAND_DISPLAY") != "" && kde:
		return WindowKWin
	case typ == "" && getenv("WAYLAND_DISPLAY") == "" && getenv("DISPLAY") != "":
		return WindowX11
	}
	return WindowNone
}

// terminalPIDs lists candidate PIDs owning the terminal window, best first:
// the Konsole process (from D-Bus), then the agent and its ancestors.
func (j *Jumper) terminalPIDs(o events.Origin, konsolePID int) []int {
	var out []int
	seen := map[int]bool{}
	add := func(p int) {
		if p > 1 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(konsolePID)
	if o.AgentPID > 0 {
		if j.Ancestors != nil {
			for _, p := range j.Ancestors(o.AgentPID) {
				add(p)
			}
		} else {
			add(o.AgentPID)
		}
	}
	return out
}

func (j *Jumper) raise(ctx context.Context, o events.Origin, hint string, konsolePID int) Step {
	be := j.Backend
	if be == WindowAuto {
		be = DetectWindowBackend(j.getenv)
	}
	pids := j.terminalPIDs(o, konsolePID)
	switch be {
	case WindowKWin:
		if len(pids) == 0 {
			return skipped(StepWindow, "kwin", "no terminal or agent pid known")
		}
		return j.kwinActivate(ctx, pids, hint)
	case WindowX11:
		return j.x11Activate(o, pids)
	default:
		return skipped(StepWindow, "", "no window backend for this session (needs X11 or KDE Plasma)")
	}
}

func (j *Jumper) x11Activate(o events.Origin, pids []int) Step {
	const be = "x11"
	var win uint32
	if o.WindowID != "" {
		v, err := strconv.ParseUint(o.WindowID, 10, 32)
		if err == nil {
			win = uint32(v)
		}
	}
	if win == 0 && len(pids) == 0 {
		return skipped(StepWindow, be, "no usable WINDOWID and no pid known")
	}
	if j.X11 == nil {
		if j.DialX11 == nil {
			return failed(StepWindow, be, "no X11 connection")
		}
		x, err := j.DialX11()
		if err != nil {
			return failed(StepWindow, be, "connect to X11: %v", err)
		}
		j.X11 = x
	}
	got, err := j.X11.Activate(win, pids)
	if err != nil {
		return failed(StepWindow, be, "%v", err)
	}
	return done(StepWindow, be, "_NET_ACTIVE_WINDOW sent to 0x%x", got)
}

// kwinScript returns a KWin script that activates the normal window whose pid
// is earliest in pids (a window whose caption contains hint wins among
// windows of the same pid) and reports back over callDBus when dest is set.
func kwinScript(pids []int, hint, token, dest, path string) string {
	q := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	cb := ""
	if dest != "" {
		cb = fmt.Sprintf(`callDBus(%s, %s, %s, "Result", %s, best !== null, best ? String(best.caption) : "");`,
			q(dest), q(path), q(CallbackInterface), q(token))
	}
	return fmt.Sprintf(`// agentpad attention jump (%s); unloaded right after it runs.
(function () {
  var pids = %s, hint = %s;
  var best = null, bestRank = 1e9;
  var list = workspace.windowList();
  for (var i = 0; i < list.length; i++) {
    var w = list[i];
    if (w.normalWindow === false || w.deleted) continue;
    var r = pids.indexOf(w.pid);
    if (r < 0) continue;
    var rank = r * 2 + (hint !== "" && String(w.caption).indexOf(hint) >= 0 ? 0 : 1);
    if (rank < bestRank) { best = w; bestRank = rank; }
  }
  if (best !== null) {
    if (best.minimized) best.minimized = false;
    workspace.activeWindow = best;
  }
  %s
})();
`, token, q(pids), q(hint), cb)
}

func (j *Jumper) kwinActivate(ctx context.Context, pids []int, hint string) Step {
	const be = "kwin"
	bus, err := j.bus()
	if err != nil {
		return failed(StepWindow, be, "%v", err)
	}
	j.scripts++
	token := fmt.Sprintf("agentpad-jump-%d-%d", os.Getpid(), j.scripts)

	var results <-chan []any
	dest, path, ch, cancel, err := bus.Callback(token)
	if err == nil {
		results = ch
		defer cancel()
	} else {
		dest, path = "", ""
	}

	dir := j.ScriptDir
	if dir == "" {
		dir = os.TempDir()
	}
	file := filepath.Join(dir, token+".js")
	if err := os.WriteFile(file, []byte(kwinScript(pids, hint, token, dest, path)), 0o600); err != nil {
		return failed(StepWindow, be, "write script: %v", err)
	}
	defer func() { _ = os.Remove(file) }()

	body, err := bus.Call(ctx, kwinService, kwinPath, kwinLoad, file, token)
	if err != nil {
		return failed(StepWindow, be, "loadScript: %v", err)
	}
	if len(body) == 1 {
		if id, ok := body[0].(int32); ok && id < 0 {
			return failed(StepWindow, be, "loadScript returned %d", id)
		}
	}
	defer func() {
		uctx, ucancel := context.WithTimeout(context.Background(), kwinUnloadTO)
		defer ucancel()
		_, _ = bus.Call(uctx, kwinService, kwinPath, kwinUnload, token)
	}()
	if _, err := bus.Call(ctx, kwinService, kwinPath, kwinStart); err != nil {
		return failed(StepWindow, be, "start script: %v", err)
	}

	wait := j.ScriptWait
	if wait <= 0 {
		wait = 300 * time.Millisecond
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case args := <-results:
		found, _ := argBool(args, 1)
		caption, _ := argString(args, 2)
		if !found {
			return failed(StepWindow, be, "no window found for pids %v", pids)
		}
		return done(StepWindow, be, "activated %q", caption)
	case <-timer.C:
		if results == nil {
			return done(StepWindow, be, "script ran for pids %v (no feedback channel)", pids)
		}
		return done(StepWindow, be, "script ran for pids %v; no confirmation within %s", pids, wait)
	case <-ctx.Done():
		return failed(StepWindow, be, "%v", ctx.Err())
	}
}

func argBool(a []any, i int) (bool, bool) {
	if i >= len(a) {
		return false, false
	}
	v, ok := a[i].(bool)
	return v, ok
}

func argString(a []any, i int) (string, bool) {
	if i >= len(a) {
		return "", false
	}
	v, ok := a[i].(string)
	return v, ok
}

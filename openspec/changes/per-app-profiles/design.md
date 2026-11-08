## Backends

| Session | Backend | Mechanism |
|---|---|---|
| X11 (any DE/WM) | `x11` | root `_NET_ACTIVE_WINDOW` PropertyNotify → read `WM_CLASS`, `_NET_WM_NAME`/`WM_NAME`, `_NET_WM_PID` |
| KDE Plasma Wayland | `kwin` | load script via `org.kde.KWin /Scripting loadScript+start`; script `callDBus("dev.thereisnotime.agentpad", "/window", "dev.thereisnotime.agentpad.Window", "Activated", class, caption, pid)`; agentpad owns the bus name and exports the method; unload on exit |
| GNOME/wlroots Wayland | none | manual only (documented) |

Selection: `XDG_SESSION_TYPE=x11` or `DISPLAY` without `WAYLAND_DISPLAY` → x11;
`wayland` + `KDE` in `XDG_CURRENT_DESKTOP` → kwin; else none. Overridable in settings.

Verified 2026-11-08 on KWin 6.6.5: the script reported
`class=librewolf, caption=..., pid=7329` on activation.

## Agent detection in terminals

From the window PID walk `/proc/<pid>/task/*/children` recursively (bounded
depth/size) and match `comm`/`cmdline` against `claude`, `codex`, `opencode`
(node-based CLIs: match argv[1] basename). Multi-tab terminals share one PID; when
several agents run, prefer the one whose session cwd (from hook events) appears
in the window title, else the most recently active session.

## Rules

YAML list in `$XDG_CONFIG_HOME/agentpad/rules.yaml`:
`{name, class?: regex, title?: regex, agent?: claude|codex|opencode, profile}`;
first match wins; `default: <profile>`. Built-in starter rules: terminal + claude
→ claude-code, terminal + codex → codex, terminal + opencode → opencode,
browsers/players → media.

## Applying

Gesture mode: swap the engine's profile immediately. Plain mode: off by default;
when enabled, rewrite the pad after the focus has been stable for the dwell time
and the target differs from what the pad holds.

## Attention jump (verified pieces, 2026-11-08)

- Hook process inherits agent env: `KONSOLE_DBUS_SERVICE`, `KONSOLE_DBUS_WINDOW`
  (`/Windows/N`), `KONSOLE_DBUS_SESSION` (`/Sessions/N`), `WINDOWID` (X11/XWayland),
  `ZELLIJ_SESSION_NAME`, `ZELLIJ_PANE_ID`, `TMUX`, `TMUX_PANE`. The hook adds an
  `origin` object (these + agent PID = first ancestor whose comm/argv matches the agent,
  plus tty) to the POST as a header-free JSON envelope `{payload, origin}` (v2 path
  `/v2/events/<agent>`; v1 stays for compatibility).
- Steps, each best-effort and reported:
  1. multiplexer: zellij `zellij --session S action focus-pane-id terminal_<ID>`
     (0.44 has `focus-pane-id`); tmux `select-window -t P; select-pane -t P`.
  2. Konsole tab: D-Bus `org.kde.konsole.Window.setCurrentSession(N)` on
     `KONSOLE_DBUS_SERVICE` + `KONSOLE_DBUS_WINDOW`.
  3. raise window: KWin script activating the window whose pid owns the terminal
     (Wayland) or `_NET_ACTIVE_WINDOW` client message to `WINDOWID` / pid lookup (X11).
- Triggers: pad gesture (configurable, default knob long-press), tray menu item,
  notification action. Queue: waiting sessions ordered by when they started waiting.

### Implementation notes (2026-11-08)

- zellij 0.44.3: `focus-pane-id` sent from outside the session does not move the
  attached client's focus and does not switch tabs. The jump runs
  `list-panes --json --tab` → `go-to-tab-by-id T` → `focus-pane-id` →
  `focus-next-pane` until `list-clients` shows the pane (bounded by the tab's
  pane count). Verified against a throwaway session with one attached client.
- KWin raise: the script gets a list of candidate PIDs (Konsole's PID from
  `GetConnectionUnixProcessID`, then the agent and its ancestors) and the Konsole
  tab title as a caption hint, since one Konsole process can own several windows.
  It reports back with `callDBus` to agentpad's unique bus name
  (`dev.thereisnotime.agentpad.Jump.Result`) and is unloaded right after
  (≤300 ms). Verified on KWin 6.6.5.
- X11: `_NET_ACTIVE_WINDOW` (source 2) to `WINDOWID` when it fits 32 bits, else
  the `_NET_CLIENT_LIST` window whose `_NET_WM_PID` matches a candidate PID.
  Konsole on Wayland exports a meaningless `WINDOWID`, so X11 is only used on
  X11 sessions.
- Waiting queue = sessions in `attention` (tool pending), ordered by
  `WaitingSince`; `waiting_input` stays idle as before. Cycling uses
  `Tracker.NextWaiting(lastKey)`; an unknown key restarts at the oldest.
- The OpenCode plugin snippet posts the v2 envelope itself, with the origin read
  from `process.env` and `process.pid`.
- Notifications: `org.freedesktop.Notifications.Notify` with `default` and
  `jump` actions, `replaces_id` per session key, 200 ms call timeout.

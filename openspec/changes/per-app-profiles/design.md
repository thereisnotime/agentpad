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

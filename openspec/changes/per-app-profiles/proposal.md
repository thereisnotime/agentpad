## Why

Different apps want different pad behaviour: approve/reject keys in a terminal
running Claude Code, Codex keys where Codex runs, media keys in a browser or
player. Switching profiles by hand defeats the point of a one-handed controller.

## What Changes

- Focused-window tracking with pluggable backends:
  - **X11** (any WM/DE, incl. XWayland-only sessions): `_NET_ACTIVE_WINDOW`,
    `WM_CLASS`, `_NET_WM_NAME`, `_NET_WM_PID` via a pure-Go X11 client, event-driven
    (PropertyNotify on the root window).
  - **KDE Plasma Wayland**: a KWin script loaded over D-Bus that calls back into
    agentpad on `workspace.windowActivated` (verified on KWin 6.6.5).
  - Other Wayland compositors: reported as unsupported; manual switching only.
- Rules engine: ordered rules matching window class, title regex, or an agent
  process (`claude`, `codex`, `opencode`) in the window's process tree; first
  match picks the profile, fallback to a default.
- Applying: instant in gesture mode; in plain mode optional pad rewrite after a
  focus dwell (default 2 s) to limit USB writes.
- UI: active profile + source in the header, rules editor, pin/override.

## Capabilities

### New Capabilities

- `app-profiles`: active-window tracking, rules, automatic profile selection.

### Modified Capabilities

- `desktop-ui`: header shows active profile source; rules editor.

## Impact

New dependency for X11 (pure Go, e.g. `github.com/jezek/xgb`) and D-Bus
(`github.com/godbus/dbus/v5`). No cgo. Release builds use the X11 Fyne backend
(works on X11 and on Wayland via XWayland); `-tags wayland` stays available.

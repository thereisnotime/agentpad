# Attention jump

When an agent waits for you (a permission prompt), agentpad can bring its
terminal back into focus: raise the window, select the terminal tab and focus
the multiplexer pane. Triggers (pad gesture, tray item) come with the UI; the
backend lives in `internal/jump` and `internal/notify`.

## How it knows where the agent runs

`agentpad hook <agent>` runs as a child of the agent, so it inherits the
agent's environment. Besides the payload it sends an *origin* to
`POST /v2/events/<agent>` as `{"payload": <raw>, "origin": {...}}`:

| Origin field | Source |
|---|---|
| `konsole_service`, `konsole_window`, `konsole_session` | `KONSOLE_DBUS_SERVICE`, `KONSOLE_DBUS_WINDOW`, `KONSOLE_DBUS_SESSION` |
| `window_id` | `WINDOWID` (only meaningful on X11) |
| `zellij_session`, `zellij_pane` | `ZELLIJ_SESSION_NAME`, `ZELLIJ_PANE_ID` |
| `tmux`, `tmux_pane` | `TMUX`, `TMUX_PANE` |
| `term_program` | `TERM_PROGRAM` |
| `agent_pid`, `tty` | first ancestor process that looks like the agent (`/proc`), else the parent |

Capturing it costs a few small `/proc` reads, no subprocesses. The old
`/v1/events/<agent>` route still accepts bare payloads, so hooks from older
builds keep working (without jump support). The OpenCode plugin snippet sends
the same envelope from inside OpenCode.

## What a jump does

Each step is best effort and reported as done, skipped or failed with a reason:

1. **Multiplexer.** zellij: switch to the pane's tab (`go-to-tab-by-id`), then
   focus the pane. tmux: `select-window` and `select-pane` on the socket from
   `$TMUX`.
2. **Terminal tab.** Konsole: `setCurrentSession(N)` over D-Bus.
3. **Window.** KDE Plasma (Wayland): a tiny KWin script, loaded for a moment
   over D-Bus, activates the window of the Konsole process (or of the agent's
   nearest ancestor that owns a window). X11: `_NET_ACTIVE_WINDOW` for
   `WINDOWID`, or the client window whose `_NET_WM_PID` matches.

With several waiting sessions, successive jumps cycle through them, oldest
waiting first.

## Notifications

Optionally agentpad shows a desktop notification when a session starts
waiting ("Claude Code needs you", tool and directory) with a **Jump** button.
Clicking it (or the bubble) does the same jump. Repeated prompts from the same
session update one notification.

## Supported setups

| Piece | Works | Notes |
|---|---|---|
| Konsole tab | yes | needs Konsole's D-Bus env vars (default) |
| Window raise, KDE Plasma Wayland | yes | KWin scripting over D-Bus |
| Window raise, X11 | yes (untested on a real X11 login) | any EWMH window manager |
| Window raise, GNOME / wlroots Wayland | no | compositors offer no activation API to other apps |
| zellij | yes | 0.44+; needs `zellij` on `PATH` |
| tmux | yes | needs `tmux` on `PATH` |
| Other terminals | window raise only | tab selection is Konsole-only for now |

## Caveats

- zellij and tmux keep the environment of the terminal that *started* the
  session. If you started zellij in one Konsole tab and later attached from
  another, the Konsole fields point at the first tab.
- zellij's `focus-pane-id` does not move an attached client's focus when sent
  from outside the session (0.44.3), so agentpad steps with `focus-next-pane`
  until the client is on the pane. With several clients attached, the first
  one that reaches the pane counts.
- Agents running over SSH on another machine cannot reach the local receiver,
  so they never show up for jumping.

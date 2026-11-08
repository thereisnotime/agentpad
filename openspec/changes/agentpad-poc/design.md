## Context

Target hardware is the CH57x "3 keys + 1 knob" macro pad (`1189:8890`). It stores
its mapping in on-board memory and enumerates as a standard HID keyboard, so once
programmed it works on any host with no driver. Programming and LED mode changes
use a vendor protocol that `kriomant/ch57x-keyboard-tool` already implements.
The `8890` firmware only exposes LED mode selection by index (off / modes 1..n),
no per-key colour.

Agents expose state through hooks:
- Claude Code: command hooks in `settings.json` receive JSON on stdin
  (`hook_event_name`, `session_id`, `cwd`, `tool_name`, `notification_type`, ...).
- Codex CLI: `notify` program receives a JSON payload as its last argv on turn completion.
- OpenCode: JS plugins subscribe to bus events (`permission.updated`, `session.idle`, ...).

## Goals / Non-Goals

Goals: one static-ish Go binary, native window, works on Linux X11/XWayland,
safe defaults (loopback only, no silent config edits), small surface that a POC
can verify end to end.

Non-goals: see proposal.

## Decisions

1. **Fyne v2 for UI** instead of Wails/web: user wants a desktop app with no web
   stack and no Node toolchain. Fyne gives a tray (`desktop.App`) and needs only
   cgo + GL/X11 headers.
2. **Device writes via `ch57x-keyboard-tool` subprocess**, behind a `pad.Writer`
   interface. Mapping YAML is generated from a profile and piped to `upload`;
   LED changes call `led <n>`. A native gousb/hidraw writer can replace it later
   without touching callers. Detection is pure Go by scanning
   `/sys/bus/usb/devices/*/idVendor|idProduct` (no libusb needed).
3. **Single binary, two modes**: `agentpad` (GUI) and `agentpad hook <agent>`
   (CLI). The hook mode reads the payload (stdin for Claude/OpenCode, last argv
   for Codex), POSTs it to `http://127.0.0.1:47321/v1/events/<agent>` with a
   300 ms timeout and always exits 0 so it can never block or break an agent.
4. **Event model**: normalised `Event{Agent, SessionID, Kind, Tool, Cwd, Time, Raw}`
   with `Kind` in `{session_start, prompt, tool_pending, tool_done, waiting_input,
   idle, session_end}`. A `Tracker` keeps per-session state; aggregate state is
   `attention` if any session is `tool_pending`/`waiting_input`, `busy` if any is
   working, else `idle`.
5. **LED policy**: aggregate state -> LED mode index (default `attention=1`,
   `busy=0`, `idle=0`), applied only on change and rate-limited to one write per
   2 s, since each LED change is a device write.
6. **Profiles**: built-in presets compiled in; user overrides stored as YAML in
   `$XDG_CONFIG_HOME/agentpad/profiles/<name>.yaml`. Applying writes the active
   profile to layer 1 and the next two profiles to layers 2 and 3 (some boards
   keep only layer 1, so the active profile always goes first).
7. **Log**: JSON Lines at `$XDG_STATE_HOME/agentpad/events.jsonl`, append-only.
8. **Layout**: `cmd/agentpad`, `internal/{pad,profile,events,hook,ui,config}`.

## Default key profiles

| Profile | Key 1 | Key 2 | Key 3 | Knob ccw / press / cw |
|---|---|---|---|---|
| claude-code | `1` | `2` | `escape` | `up` / `enter` / `down` |
| codex | `y` | `a` | `escape` | `up` / `enter` / `down` |
| opencode | `enter` | `a` | `escape` | `up` / `enter` / `down` |
| media | `prev` | `play` | `next` | `volumedown` / `mute` / `volumeup` |

Agent prompt keys differ between versions; every key is editable in the GUI.

## Risks / Trade-offs

- External tool dependency -> clear "tool not found" state in UI, install hint.
- LED writes may wear device flash -> change-only + rate limiting.
- Hook JSON shapes change across agent versions -> parsers are lenient, unknown
  events are logged with `Kind=other` and the raw payload kept.
- USB-C power quirk of this pad (missing CC pull-downs) -> documented in README.

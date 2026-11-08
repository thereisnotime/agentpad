## Why

Cheap CH57x macro pads (3 keys + 1 knob, ~6 EUR on AliExpress) make a great
physical "approve once / approve always / reject" controller for AI coding agents.
Existing projects each cover one slice: clawkeys programs the pad for OpenCode,
Gavel shows Claude Code hook state on LEDs and logs decisions, CodexPad and
AgentDeck show agent status. None of them combine programming the pad, per-agent
profiles, live agent state and a decision log in one native Linux desktop app.

## What Changes

- New Go desktop application `agentpad` (Fyne UI, single binary, no web stack).
- Detect a connected CH57x pad (USB `1189:8890`, `1189:8840`, `1189:8842`).
- Built-in key profiles for Claude Code, Codex CLI and OpenCode; editable and
  persisted to the user config directory; written to the pad from the GUI.
- Local event receiver (loopback HTTP) plus an `agentpad hook <agent>` CLI that
  agent hooks call; events update a live session list and an append-only log.
- Agent state drives the pad LED mode (e.g. lit while a permission prompt waits).
- Generated hook snippets for Claude Code, Codex and OpenCode, shown in the GUI
  for the user to install. The app never edits agent config files silently.
- System tray icon reflecting aggregate agent state.

## Capabilities

### New Capabilities

- `pad-device`: detecting the macro pad and writing key mappings and LED modes to it.
- `key-profiles`: built-in and user-edited per-agent key profiles and their persistence.
- `agent-events`: receiving agent hook events, tracking sessions, logging, and mapping state to LEDs.
- `desktop-ui`: the desktop window and tray that expose the above.

### Modified Capabilities

None (new project).

## Impact

- New repository `thereisnotime/agentpad` (public, MIT).
- Runtime dependencies: `ch57x-keyboard-tool` on PATH for device writes (POC),
  a udev rule granting the user access to the pad, X11/OpenGL libraries for Fyne.
- Optional integration points in user agent configs (`~/.claude/settings.json`,
  `~/.codex/config.toml`, OpenCode plugin dir), installed manually by the user.

## Non-goals

- Native Go USB protocol implementation (shelling out to `ch57x-keyboard-tool` for the POC).
- Windows/macOS packaging.
- Per-key RGB or live LED animation (not supported by the `8890` firmware).
- Reading raw key presses from the pad.

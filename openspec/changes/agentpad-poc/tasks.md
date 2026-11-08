## 1. Scaffold

- [ ] 1.1 Repo, `.tool-versions`, justfile, README, LICENSE, CI workflow
- [ ] 1.2 Go module, package skeleton with shared types and interfaces
- [ ] 1.3 Public GitHub repo `thereisnotime/agentpad`

## 2. Device and profiles (`internal/pad`, `internal/profile`)

- [x] 2.1 sysfs pad detection for `1189:8890/8840/8842`
- [x] 2.2 `ch57x-keyboard-tool` writer: upload mapping, set LED, map errors (permission, missing tool)
- [x] 2.3 LED controller: change-only, 2 s rate limit
- [x] 2.4 Built-in profiles, validation against supported key names
- [x] 2.5 YAML profile store with override + reset in XDG config dir
- [x] 2.6 Mapping YAML generation with layer assignment
- [x] 2.7 Unit tests

## 3. Agent events (`internal/events`, `internal/hook`, `internal/setup`)

- [x] 3.1 Event model and normalisers for Claude Code, Codex, OpenCode
- [x] 3.2 Session tracker and aggregate state
- [x] 3.3 JSONL event log in XDG state dir
- [x] 3.4 Loopback HTTP receiver with subscriber fan-out
- [x] 3.5 `agentpad hook <agent>` (stdin/argv, 300 ms timeout, exit 0)
- [x] 3.6 Hook snippets for Claude Code, Codex, OpenCode plugin, udev rule
- [x] 3.7 Unit tests

## 4. Desktop UI (`internal/ui`, `cmd/agentpad`)

- [x] 4.1 Fyne app, main window with Pad / Profiles / Sessions / Log / Setup
- [x] 4.2 Visual 3-key + knob profile editor with save/reset/apply
- [x] 4.3 Live sessions + log bound to the event receiver
- [x] 4.4 Setup tab with copy buttons
- [x] 4.5 Tray icon reflecting aggregate state, close-to-tray
- [x] 4.6 LED policy wiring (aggregate state -> pad LED)

## 5. Verify

- [x] 5.1 `just lint test build` green
- [ ] 5.2 Manual: hook events from a real Claude Code session show in the UI
- [ ] 5.3 Manual: apply profile + LED on a real pad (FENRIR or Surface with adapter)

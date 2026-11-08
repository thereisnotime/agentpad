# agentpad · agent instructions

Go + Fyne desktop app that programs CH57x 3-key + knob macro pads and reacts to
AI coding agent hooks (Claude Code, Codex CLI, OpenCode).

- Specs live in `openspec/`. Update the relevant change/spec before changing behavior.
- Layout: `cmd/agentpad` (entry, `hook` subcommand), `internal/pad` (USB detect,
  ch57x writer, LED controller), `internal/profile` (profiles, store, mapping YAML),
  `internal/events` (normalise, tracker, log, HTTP receiver), `internal/hook`,
  `internal/setup` (snippets), `internal/ui` (Fyne).
- Only `internal/ui` and `cmd/agentpad` may import Fyne. Everything else stays
  testable without cgo/X11.
- `just ci` must pass before pushing. Tests must not need a real pad or network.
- The app never edits agent config files; it shows snippets instead.
- Commits: short, lowercase, conversational (e.g. `add led rate limiting`). No Co-Authored-By.

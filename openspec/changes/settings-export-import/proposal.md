## Why

Users want to customise agentpad easily, keep the configuration in dotfiles, move it
between machines and share setups (e.g. a Codex profile) with others. Settings hidden
in Fyne preferences can't be versioned, reviewed or shared.

## What Changes

- Human-editable `settings.yaml` in the agentpad config dir, schema-versioned,
  commented on first write, validated, live-reloaded on external edits; replaces
  Fyne preferences (one-time migration).
- Export/import of a bundle (settings + profiles + future rules/gestures) as one
  `.agentpad.yaml` file with preview, merge or replace, and automatic backup.
- Single-profile export/import as file or clipboard text.

## Capabilities

### New Capabilities

- `settings`: settings file, validation, live reload, bundles, profile sharing.

### Modified Capabilities

None (UI wiring follows in the UI integration).

## Impact

New package `internal/settings`; UI reads/writes settings through it. Config dir layout:
`~/.config/agentpad/{settings.yaml, profiles/*.yaml, backups/}`.

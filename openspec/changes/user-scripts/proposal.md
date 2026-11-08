## Why

Users want the pad and agent events to trigger their own automation: deploy,
mute the mic, ping a phone when Claude waits, switch a tmux layout, etc.

## What Changes

- User scripts defined in `settings.yaml` (`scripts:`), argv by default, explicit
  `shell:` opt-in, with cwd, env and timeout.
- Pad action `run:<name>`: in gesture mode any slot; in plain mode the control is
  programmed to a spare key (F19–F24) and agentpad runs the script when it observes it.
- Event hooks (`hooks:`) on agentpad events with JSON on stdin and `AGENTPAD_*` env.
- Scripts screen: edit scripts/hooks, Run now, last runs with exit code and output.
- Import safety: bundles adding scripts/hooks are flagged and need per-item consent.

## Capabilities

### New Capabilities

- `scripts`: script definitions, execution, pad triggers, event hooks, run log.

### Modified Capabilities

None.

## Impact

`internal/settings` (schema additions), new `internal/scripts`, `internal/profile`
(action grammar `run:<name>`), gesture/live wiring, UI Scripts screen, docs.

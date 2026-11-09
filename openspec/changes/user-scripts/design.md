## Settings schema (still schema 1, additive)

```yaml
scripts:
  deploy:
    run: ["~/bin/deploy.sh", "--fast"]   # argv; ~ expanded in argv[0] and cwd
    cwd: ~/projects/api
    env: {STAGE: prod}
    timeout: 30s                          # default 30s, max 10m
  mic-off:
    shell: "pactl set-source-mute @DEFAULT_SOURCE@ toggle"   # runs via /bin/sh -c
hooks:
  - on: attention                         # see events below
    agent: claude                         # optional filter: claude|codex|opencode
    script: ping-phone
```

Exactly one of `run`/`shell`. Names `[a-z0-9-]+`.

Events: `attention`, `busy`, `idle`, `session_start`, `session_end`, `gesture`,
`profile_applied`, `mode_changed`, `pad_connected`, `pad_disconnected`, `jump`.

## Execution

- `internal/scripts.Runner`: max 4 concurrent runs, per-script single-flight
  (a second trigger while running is dropped and logged), timeout kills the process group.
- stdin: JSON `{event, time, agent, session_id, cwd, tool, profile, gesture, ...}`;
  env: `AGENTPAD_EVENT`, `AGENTPAD_AGENT`, `AGENTPAD_SESSION`, `AGENTPAD_CWD`,
  `AGENTPAD_TOOL`, `AGENTPAD_PROFILE`, `AGENTPAD_CONTROL`, `AGENTPAD_GESTURE`.
- stdout/stderr captured (cap 64 KiB each), runs appended to
  `$XDG_STATE_HOME/agentpad/scripts.jsonl`; last 50 kept in memory for the UI.
- Runs never block the receiver, UI or gesture engine.

## Pad triggers

- Gesture mode: action `run:<name>` is intercepted like `agentpad:*` actions.
- Plain mode: controls whose action is `run:<name>` are programmed to F19..F24
  (`profile.PlainLayers`). Keys go first come, first served over the uploaded
  layers (layer 1 first) and, within a layer, in key1, key2, key3, knob-ccw,
  knob-press, knob-cw order, one key per control. While keys are left every
  script control on the pad has its own key, so the key also identifies the
  layer; a layer whose script controls don't fit in what is left starts over at
  F19 and the layer guess picks the script. A profile has six controls, so it
  can never need more than six keys (no extra validation error).
  The plain-mode observer maps those keycodes back to the script of the guessed
  layer. Without agentpad running the pad just sends F19.. which nothing binds
  by default.
- Profile validation accepts `run:<name>` only for defined scripts through a
  package-level resolver (`profile.SetScriptResolver`) the UI sets from
  settings.yaml; with no resolver any well-formed name passes. Import plans use
  the bundle's scripts (`profile.ValidateWith`).

## Import safety

`PlanImport` marks script/hook additions or changes as `Kind=add|change` with
`Area=scripts|hooks` and `Dangerous=true`; the import dialog requires ticking each
dangerous item; unticked ones are skipped.
`Plan.Consent(accept)` returns the plan with refused items reverted (a refused
script keeps its current definition, a refused hook is not added). Merge imports
union scripts and hooks with the current ones; removals (Replace) are not
dangerous. If a refused script is still needed by an imported hook or profile,
Apply fails validation before writing anything.

## Hooks

`attention` / `busy` / `idle` fire on per-session state transitions (a session
first seen idle fires only `session_start`). Identical events (event, agent,
session, control, gesture) within 500 ms fire once.

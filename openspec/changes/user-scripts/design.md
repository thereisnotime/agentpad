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
  (first come, first served; > 6 scripts in one profile → validation error);
  the plain-mode observer maps those keycodes back to the script. Without
  agentpad running the pad just sends F19.. which nothing binds by default.

## Import safety

`PlanImport` marks script/hook additions or changes as `Kind=add|change` with
`Area=scripts|hooks` and `Dangerous=true`; the import dialog requires ticking each
dangerous item; unticked ones are skipped.

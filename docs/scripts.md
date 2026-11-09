# Scripts and event hooks

Run your own commands from the pad and when agentpad events happen: mute the mic, ping your phone when Claude
waits, switch a tmux layout, pull the repo an agent works in.

- **Scripts** are named commands in `settings.yaml` (`scripts:`), or made on the **Scripts** screen.
- **Pad triggers:** put the action `run:<name>` on any control, in plain or gesture mode.
- **Hooks** (`hooks:`) run a script when an event happens (an agent needs you, a session starts, a gesture, ...).
- **Run now** on the Scripts screen or in the tray (Run script ▸), with the result in a toast.

Every run is listed under **Last runs** with the trigger, exit code or timeout, duration and its output.

## Defining scripts

```yaml
scripts:
  deploy:
    run: ["~/bin/deploy.sh", "--fast"]   # command and arguments, no shell
    cwd: ~/projects/api                   # optional working directory
    env: {STAGE: prod}                    # optional extra environment
    timeout: 2m                           # optional, default 30s, max 10m
  mic-toggle:
    shell: "pactl set-source-mute @DEFAULT_SOURCE@ toggle"   # runs via /bin/sh -c
```

| Key | Meaning |
|---|---|
| `run` | Command and arguments as a list. Run directly, **not through a shell**: no globbing, pipes or `$VARS`. `~` is expanded in the first item. |
| `shell` | A command line for `/bin/sh -c`, when you need pipes, `&&` or variables. Set either `run` or `shell`, not both. |
| `cwd` | Working directory, `~` expanded. Must exist. Default: where agentpad was started. |
| `env` | Extra variables, added after agentpad's own environment and the `AGENTPAD_*` values. |
| `timeout` | `30s`, `2m`, ... (default 30s, at most 10m). On timeout the script's whole process group gets SIGTERM, then SIGKILL two seconds later. |

Names use `a-z`, `0-9` and `-`. In the editor on the Scripts screen, **Command** takes the arguments quoted like in
a shell (`notify-send 'agent waits'`); they are split once and still run without a shell unless you tick
**Run through /bin/sh -c**.

### What a script gets

The event as JSON on **stdin** (fields that don't apply are left out):

```json
{"event":"attention","time":"2026-11-09T10:12:03Z","agent":"claude","session_id":"3f9c1d2e",
 "cwd":"/home/me/src/api","tool":"Bash","message":"Claude needs your permission to use Bash",
 "profile":"claude-code","mode":"plain","trigger":"hook attention"}
```

Pad triggers add `control` (`key2`, `knob-cw`, `key1+key3`), `gesture` (`hold`, ...; gesture mode only) and
`action`. `event` is the hook event, `pad` for a pad trigger and `manual` for Run now.

And the same as **environment variables**: `AGENTPAD_EVENT`, `AGENTPAD_AGENT`, `AGENTPAD_SESSION`, `AGENTPAD_CWD`,
`AGENTPAD_TOOL`, `AGENTPAD_PROFILE`, `AGENTPAD_CONTROL`, `AGENTPAD_GESTURE` (empty when they don't apply).

stdout and stderr are kept (64 KiB each) and shown when you tap a run under Last runs. Runs are appended to
`scripts.jsonl` next to the event log (`~/.local/state/agentpad/`), which also brings Last runs back after a
restart.

### How runs are limited

- At most 4 scripts run at once; more wait for a free slot.
- A script is never started twice at the same time: a trigger while it still runs is dropped and listed as
  `skipped`.
- Nothing waits for a script. The event receiver, the UI and gesture mode carry on while it runs.
- Quitting agentpad stops running scripts (SIGTERM, then SIGKILL).

## Running scripts from the pad

Set a control's action to `run:<name>`. In the Profiles editor use **Run script ▸** under the slots or the
`run:<name>` quick pick chips; the action field suggests them too. A profile can only use scripts that exist.

**Gesture mode:** any slot works (tap, hold, long hold, double-tap, turn, press + turn, chords). agentpad runs the
script instead of typing anything, like the `agentpad:` actions.

**Plain mode:** the pad firmware can only send keys, so Apply programs each `run:<name>` control with a spare key,
**F19 to F24**, and agentpad runs the script when it sees that key. Keys are handed out first come, first served:

1. layer 1 first, then layers 2 and 3;
2. within a layer in the order key 1, key 2, key 3, knob left, knob press, knob right;
3. one key per control, even when two controls run the same script.

While keys are left every script control on the pad has its own key, so the key also tells agentpad which layer the
pad is on. When a layer's script controls don't fit in what is left, that layer starts over at F19 and the
[layer guess](profiles.md#layers) decides which script runs. A single profile never needs more than six.

Example: layer 1 has `run:deploy` on key 1 and `run:mic-toggle` on knob press, layer 2 has `run:pull` on key 3.
Apply writes F19 (key 1) and F20 (knob press) on layer 1 and F21 (key 3) on layer 2.

Plain-mode triggers need live input (the same input permission as the live view, see
[Gestures](gestures.md#permissions)). Without agentpad running the pad just sends F19 and friends, which nothing
binds by default; F19-F24 still reach the focused window while agentpad runs, so don't bind them elsewhere.

## Hooks

```yaml
hooks:
  - on: attention          # event, see below
    agent: claude          # optional: claude | codex | opencode
    script: ping-phone
  - on: gesture
    script: log-gesture
```

| Event | When |
|---|---|
| `attention` | an agent session starts waiting for you (permission prompt) |
| `busy` | a session starts working (prompt sent, tool approved) |
| `idle` | a session finished its turn |
| `session_start` / `session_end` | a session starts or ends |
| `gesture` | gesture mode recognised a gesture (with `control`, `gesture`, `action`) |
| `profile_applied` | a profile was applied (`profile`) |
| `mode_changed` | plain / gesture mode switched (`mode`) |
| `pad_connected` / `pad_disconnected` | the pad was plugged in / removed (`pad` is `vendor:product`) |
| `jump` | agentpad jumped to a waiting session |

`attention`, `busy` and `idle` fire when a session's state **changes**, not on every agent event. A new session
that starts out idle only fires `session_start`. The same event for the same session (and control) within 500 ms
fires once. `agent:` only matches events that come from an agent session.

On the Scripts screen, pick the event, optionally an agent and the script, then **Add**.

## Recipes

### Mic mute toggle

```yaml
scripts:
  mic-toggle:
    run: [pactl, set-source-mute, "@DEFAULT_SOURCE@", toggle]
```

Put `run:mic-toggle` on the knob press, or on a long hold in gesture mode.

### Push to your phone when Claude waits (ntfy.sh)

```yaml
scripts:
  ping-phone:
    shell: >-
      curl -s -H "Title: $AGENTPAD_AGENT needs you"
      -d "${AGENTPAD_TOOL:-a decision} in ${AGENTPAD_CWD##*/}"
      "https://ntfy.sh/$TOPIC"
    env: {TOPIC: my-secret-agentpad-topic}
    timeout: 10s
hooks:
  - on: attention
    agent: claude
    script: ping-phone
```

Anyone who knows an ntfy.sh topic can read it, so pick a long random one (or run your own ntfy server).

### Switch the tmux or zellij layout

```yaml
scripts:
  layout-review:
    run: [tmux, select-layout, -t, "agents:0", even-horizontal]
  zellij-next:
    run: [zellij, action, next-swap-layout]
```

`tmux` needs to reach your server; agentpad started from your desktop session usually does. For zellij, set
`env: {ZELLIJ_SESSION_NAME: main}` if you run more than one session.

### Desktop notification with the tool name

```yaml
scripts:
  notify-tool:
    shell: 'notify-send -a agentpad "$AGENTPAD_AGENT wants $AGENTPAD_TOOL" "$AGENTPAD_CWD"'
hooks:
  - on: attention
    script: notify-tool
```

(agentpad's own attention notification already has a Jump button; this is for when you want your own text or
urgency.)

### git pull in the session's directory

```yaml
scripts:
  pull-here:
    shell: 'cd "${AGENTPAD_CWD:?no session cwd}" && git pull --ff-only'
    timeout: 1m
hooks:
  - on: idle
    agent: codex
    script: pull-here
```

From the pad (`run:pull-here`) there is no session, so `AGENTPAD_CWD` is empty and the script stops with a
message instead of pulling wherever agentpad was started. The output is under Last runs.

## Security

- Scripts run as you, with your environment, whenever their trigger fires. Only add commands you would run
  yourself.
- Prefer `run:` over `shell:`. With `run:` the event data (`AGENTPAD_TOOL`, `AGENTPAD_CWD`, the JSON on stdin) is
  passed as data and never parsed by a shell. In `shell:` scripts, always quote variables (`"$AGENTPAD_CWD"`): the
  values come from the agent and can contain spaces or shell characters.
- Event data comes from agent hooks on your machine (the receiver only listens on loopback), but a tool name or
  message is whatever the agent reported. Don't `eval` it.
- **Imported bundles** that add or change scripts or hooks show them under a red **Runs commands** heading in the
  import preview, each with its own checkbox, all unticked. Only ticked items are imported; the rest of the bundle is
  imported normally. If you skip a script that a hook or profile in the bundle needs, the import stops before
  anything is written. Removing scripts (Replace) needs no consent.
- `settings.yaml` is plain text; keep tokens in a file only you can read and have the script load it, rather than
  in `env:`, if you keep the folder in [dotfiles](configuration.md#keeping-it-in-dotfiles).

[← Back to the docs index](README.md)

# Configuration

[← Docs index](README.md)

All of agentpad's configuration is plain YAML in one folder. You can edit it by hand, keep it in your dotfiles,
copy it between machines and share pieces of it.

## Folder layout

`$XDG_CONFIG_HOME/agentpad/` (usually `~/.config/agentpad/`):

```text
~/.config/agentpad/
  settings.yaml          # app settings (this page)
  profiles/<name>.yaml   # profile overrides, see Profiles
  rules.yaml             # per-app profile rules (when that feature is in use)
  backups/<timestamp>/   # automatic backups taken before an import or restore
```

- `settings.yaml` is created with comments and defaults on first start. Older versions kept these values in Fyne
  preferences; they are copied into `settings.yaml` once, the first time the file is created (orientation, active
  profile, input mode, LED follow, LED policies and labels, notifications, gesture timings). Values that don't
  validate are skipped. The old preferences are left in place but no longer read.
- `profiles/` only holds profiles you changed or added. Built-ins are compiled in; deleting an override restores
  the built-in. See [Profiles](profiles.md) for the action grammar.
- `backups/` is managed by agentpad. Each backup is a folder named after the UTC time it was taken
  (`20261108T235900Z`) with copies of `settings.yaml`, `profiles/` and `rules.yaml` as they were.

## settings.yaml

Edit it in any editor. agentpad notices the save within about a second and applies it without a restart.

- **Invalid values** are reported with the field and line (for example
  `settings.yaml: line 3, column 16: pad.orientation: "sideways" is not allowed (want normal | clockwise | upsidedown | counterclockwise)`)
  and ignored: the last valid settings stay active until you fix the file. The app shows a red
  `settings.yaml: error on line N` pill in the header; click it for the full message and an **Open settings.yaml**
  button. The pill goes away when the file is valid again.
- Profile files in `profiles/` are picked up too (checked every 2 seconds). If you have unsaved edits in the
  Profiles editor they are kept; the other profiles refresh.
- If `settings.yaml` can't be opened at all (for example the folder isn't writable), agentpad starts anyway on the
  old toolkit preferences and says so in a message; export and import are off until the next start.
- **Unknown keys** are kept untouched and listed as warnings. Handy for notes or settings from a newer version.
- **Comments are kept.** When agentpad itself changes a setting (for example when you rotate the pad in the UI) it
  edits only that value in place, so your comments, key order and quoting survive.
- Missing keys take their default, so a file with just the values you care about is fine.
- Writes are atomic (temp file + rename). If `settings.yaml` is a symlink (dotfiles managers like stow do this),
  agentpad writes through the link and keeps it.

### Reference (schema 1)

This is the file agentpad writes on first start, with every default:

```yaml
# Settings format version. Leave as is.
schema: 1
# The macro pad.
pad:
  # How the pad sits on the desk:
  #   normal (knob right) | upsidedown (knob left) | clockwise (knob bottom) | counterclockwise (knob top)
  orientation: normal
  # Profile last written to layer 1 (agentpad updates this when you apply):
  #   claude-code | codex | opencode | media | the name of any custom profile | "" (none yet)
  active_profile: ""
  # How key presses are handled:
  #   plain (the pad types the keys itself) | gesture (agentpad reads the pad: tap, hold, double-tap, chords)
  mode: plain
# LEDs.
led:
  # Switch the LEDs when an agent needs attention: true | false
  follow_agents: true
  # 0x8890 boards: built-in effects by index, no colours.
  #   attention = while an agent waits for you, normal = the rest of the time.
  #   Format: {mode: 0-5} (0 = off, 1-5 = built-in effects)
  "8890":
    attention: {mode: 1}
    normal: {mode: 0}
  # 0x8840/0x8842 boards: named effects with colours, set per layer.
  #   Format: {layer: 0-2, effect: off | backlight | shock | shock2 | press, color: COLOR}
  #   COLOR: red | orange | yellow | green | cyan | blue | purple (white for backlight only);
  #   leave color out for effect off.
  884x:
    attention: {layer: 0, effect: backlight, color: orange}
    normal: {layer: 0, effect: "off"}
  # Your names for the 0x8890 effects shown in the UI, keyed by mode index 0-5,
  #   e.g. labels: {"1": Steady, "2": Breathing}
  labels: {}
# Gesture mode timing, in milliseconds.
gestures:
  # Press at least this long to count as a hold: 100-5000
  hold_ms: 450
  # Press at least this long to count as a long hold: more than hold_ms, up to 10000
  long_hold_ms: 1500
  # Max gap between two taps of a double-tap: 100-1000
  double_tap_ms: 300
  # Max gap between the two key presses of a chord: 10-500
  chord_window_ms: 80
# Desktop notifications.
notifications:
  # Notify when an agent needs attention: true | false
  attention: true
# Desktop UI.
ui:
  # Colour theme: system | dark | light
  theme: system
```

| Key | Values | Default |
|---|---|---|
| `schema` | `1` | `1` |
| `pad.orientation` | `normal`, `upsidedown`, `clockwise`, `counterclockwise` ([Hardware](hardware.md)) | `normal` |
| `pad.active_profile` | a profile name, or `""` | `""` |
| `pad.mode` | `plain`, `gesture` ([Gestures](gestures.md)) | `plain` |
| `led.follow_agents` | `true`, `false` | `true` |
| `led.8890.attention` / `.normal` | `{mode: 0-5}` | `{mode: 1}` / `{mode: 0}` |
| `led.884x.attention` / `.normal` | `{layer: 0-2, effect: …, color: …}` | orange backlight / off |
| `led.labels` | map of `"0"`–`"5"` to a name (max 32 characters) | empty |
| `gestures.hold_ms` | 100–5000 | 450 |
| `gestures.long_hold_ms` | above `hold_ms`, up to 10000 | 1500 |
| `gestures.double_tap_ms` | 100–1000 | 300 |
| `gestures.chord_window_ms` | 10–500 | 80 |
| `notifications.attention` | `true`, `false` | `true` |
| `ui.theme` | `system`, `dark`, `light` | `system` |
| `scripts.<name>` | `{run: [cmd, args…]}` or `{shell: "…"}`, plus optional `cwd`, `env`, `timeout` ([Scripts](scripts.md)) | none |
| `hooks` | list of `{on: EVENT, agent: claude \| codex \| opencode, script: NAME}` ([Scripts](scripts.md#hooks)) | none |

### Scripts and hooks

`scripts:` and `hooks:` are optional and not in the first-run file. They follow the same rules (validated with line
numbers, comments kept when the Scripts screen edits them). A hook must name a script that exists, and a profile
action `run:<name>` must too. The full reference and recipes are in [Scripts and event hooks](scripts.md).

```yaml
scripts:
  mic-toggle:
    run: [pactl, set-source-mute, "@DEFAULT_SOURCE@", toggle]
hooks:
  - on: attention
    agent: claude
    script: mic-toggle
```

LED policies are kept per board family because the two families describe their LEDs differently; the one matching
the connected pad is used. Each family is validated against what that board can do (see [Hardware](hardware.md)).
Quote `"off"` if another tool reads the file: YAML 1.1 parsers treat a bare `off` as false (agentpad doesn't).

## Keeping it in dotfiles

The folder is safe to keep in git:

- Everything except `backups/` is small, human-readable and machine-independent. No secrets live here.
- Add `backups/` to `.gitignore`; it is agentpad's own safety net and changes on every import.
- Symlinking either the whole folder or individual files works. agentpad edits `settings.yaml` through the link.
- agentpad updates `pad.active_profile` and other values when you use the UI, so expect small diffs. Commit them
  or keep them, it's your call; comments you add are preserved.

## Bundles: export and import

A bundle is a single `*.agentpad.yaml` file with your settings, profiles and rules, for moving a setup to another
machine or sharing it.

```yaml
kind: agentpad-bundle
schema: 1
exported_by: agentpad v0.2.0
exported_at: "2026-11-08T23:59:00Z"
settings:              # optional; may be partial
  pad: {orientation: clockwise}
  led:
    "8890": {attention: {mode: 4}, normal: {mode: 0}}
profiles:              # optional; your overrides, or every profile when "all profiles" is chosen
  - name: zellij-claude
    description: Claude Code in zellij
    keys: ["1", "2", escape]
    knob: {ccw: up, press: enter, cw: down}
rules:                 # optional; rules.yaml content, passed through as is
  ...
```

- `kind` and `schema` are required. A bundle from a newer agentpad with a higher schema is refused instead of
  half-understood.
- Export includes `settings`, the profiles you changed or added (or all profiles), and `rules.yaml` if it exists.
- Unknown keys in settings are reported as warnings and not imported.

### In the app

**About > Settings** has the buttons:

- **Export…** asks whether to include the built-in profiles too (off: only the profiles you changed or added), then
  where to save. The suggested name is `agentpad-<date>.agentpad.yaml`.
- **Import…** opens a bundle and shows a preview grouped by settings, profiles and rules: additions in green, changes
  in blue with old and new values, removals in red, warnings (such as unknown keys) last. Pick **Merge**, or
  **Replace…** to see the replace preview first (with **Back** to return). A bundle that doesn't validate is refused
  with the problems listed; nothing is changed. After an import the card shows `Imported · backup <name>` with a
  **Restore backup** button that undoes it.
- **Backups ▸** lists the backups, newest first, each with **Restore** (asks first).
- **Open settings folder** opens the config folder in your file manager.

### Import

Import never writes anything until you confirm:

1. **Validate everything.** Settings as above, LED values against their board family, every profile action with
   the same checks as the editor. Any problem refuses the whole import, naming the field or the profile and control
   with the bundle line, e.g. `line 6, column 5: profiles.zellij-claude: key 2: unknown key "foo"`.
2. **Preview.** A list of what would be added, changed and removed, with old and new values.
3. **Back up.** On confirm, the current `settings.yaml`, `profiles/` and `rules.yaml` are copied to
   `backups/<UTC timestamp>/`.
4. **Apply.** Each file is written atomically.

Two modes:

| | Merge | Replace |
|---|---|---|
| Settings | values in the bundle overwrite yours; keys missing from it keep your values | start from defaults, then the bundle's values |
| Profiles | added or updated; your other overrides stay | overrides not in the bundle are removed (built-ins come back) |
| Rules | replaced when the bundle has them | replaced when the bundle has them |

A section missing from the bundle is left alone in both modes, so a bundle with only `profiles:` never touches
your settings.

### Restore

Every backup can be restored from the app. A restore validates the backup first, then backs up the current state
(so the restore itself can be undone) and puts back `settings.yaml`, the profile overrides and `rules.yaml`
exactly as they were, removing overrides that didn't exist at the time. You can also copy files out of
`backups/` by hand; agentpad picks up the change.

## Sharing a single profile

On the **Profiles** screen, the **Share** button under Save / Reset / Apply has: **Copy as text**, **Save to file…**,
**Import from clipboard** and **Import from file…**. Sharing uses the profile as shown in the editor (it must be
valid). Import shows the profile first, then saves it and selects it; a toast says when it was renamed.

A single profile can be copied as text (clipboard) or saved as a file:

```yaml
kind: agentpad-profile
schema: 1
name: codex
description: 'Codex CLI approval: y approve, a always, Esc deny'
keys: ["y", "a", escape]
knob: {ccw: up, press: enter, cw: down}
```

- Importing validates the profile first. Plain profile YAML (a `profiles/<name>.yaml` file without `kind`) is
  accepted too.
- If you already have a changed profile with that name and different content, the import is renamed: `codex`
  becomes `codex-2`, then `codex-3`, and so on. Importing an identical profile does nothing.
- Every profile field travels, including gesture actions.

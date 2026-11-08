## Layout

```text
~/.config/agentpad/
  settings.yaml          # app settings (this change)
  profiles/<name>.yaml   # profile overrides (existing)
  rules.yaml             # per-app rules (per-app-profiles change)
  backups/<timestamp>/   # automatic backups before import
```

## settings.yaml (schema 1)

```yaml
# agentpad settings — edit freely, agentpad reloads on save.
schema: 1
pad:
  orientation: normal          # normal | upsidedown | clockwise | counterclockwise
  active_profile: claude-code
  mode: plain                  # plain | gesture
led:
  follow_agents: true
  attention: {mode: 1}         # 8890: {mode: N}; 884x: {layer, effect, color}
  normal:    {mode: 0}
  labels: {"1": "Steady", "2": "Running"}   # 8890 mode labels
gestures:
  hold_ms: 450
  long_hold_ms: 1500
  double_tap_ms: 300
notifications:
  attention: true
ui:
  theme: system                # system | dark | light
```

- Strongly typed Go struct with defaults; unknown keys preserved (yaml.Node round-trip)
  and reported as warnings, never dropped.
- Writes are atomic (temp + rename) and keep user comments where possible
  (edit the yaml.Node tree rather than re-marshal when the file exists).
- Watch the directory with inotify (`golang.org/x/sys/unix`) or a 1 s mtime poll
  fallback; debounce 250 ms; ignore our own writes (content hash).
- Invalid edits: keep the last good settings, surface the error with line numbers.
- Migration: if no settings.yaml exists, import values from a provided map (the UI
  passes Fyne preferences) once and write the file.

## Bundle (`*.agentpad.yaml`)

```yaml
kind: agentpad-bundle
schema: 1
exported_by: agentpad v0.2.0
exported_at: 2026-11-08T23:59:00Z
settings: {...}            # optional
profiles: [{...}, ...]     # optional, only overrides unless "all" requested
rules: [...]               # optional, future
```

- Import = parse → validate everything (profiles via profile.Validate, LED via pad.ValidateLED
  where caps known) → compute a Plan (adds, changes, removals for replace, warnings)
  → caller shows preview → Apply(plan) after backing up current files.
- Single profile export: `kind: agentpad-profile`, schema 1, profile fields; import into
  the store (rename on conflict: `name-2`).

## Implementation notes and deviations (core, tasks 1.x / 3.1)

- **LED policy per board family.** The UI keeps one LED policy per family, so the file has
  `led."8890"` and `led.884x`, each `{attention, normal}`, instead of a single
  `led.attention` / `led.normal`. Each family is validated with its own caps
  (`pad.CapsFor(0x8890)` / `pad.CapsFor(0x8840)`), so import needs no model hint.
- **`pad.active_profile` defaults to `""`** (no profile applied yet), matching the UI's
  "last applied profile" semantics. Any value must be a valid profile name.
- **`gestures.chord_window_ms`** added next to the three thresholds (default 80, from `internal/gesture`).
- **Migration keys** (Fyne preference names used by the UI, plus aliases): `orientation`,
  `profile.active` / `activeProfile`, `led.follow` / `ledFollow`, `led.policy.8890`,
  `led.policy.884x`, `ledPolicy` (JSON `{"attention":…,"otherwise":…}`), `led.attentionMode` /
  `attentionMode` (ignored when `led.policy.8890` is given), `ledLabel.0`…`ledLabel.5`, `theme`.
  `Migrate` returns `(applied bool, err error)`; bad values are skipped and listed in `Warnings()`.
- **Replace mode** applies per section: sections missing from the bundle are left alone; settings
  start from defaults; profile overrides absent from the bundle are reset (built-ins come back).
- **Export** writes settings from the typed struct, so unknown keys and comments in `settings.yaml`
  are not exported (they stay in the local file).
- **Restore** validates the backup, takes a safety backup of the current state first, and removes
  profile overrides that were not in the backup.
- **Symlinks:** writes resolve a symlinked `settings.yaml` and replace the target, keeping the link;
  the watcher also watches the target's directory.
- **Watcher** runs a 1 s content-hash poll alongside inotify as a safety net; a truncated (empty)
  file seen mid-save is ignored. `onChange` fires for external edits and after `Apply` / `Restore`;
  not for `Update` (the caller knows), except when `Update` first adopted an unseen external edit.
- **Single-profile share** lives in `internal/settings` (`ExportProfile`, `ParseProfile`,
  `ImportProfile`); `internal/profile` only gained `ValidName` and `IsBuiltin`.

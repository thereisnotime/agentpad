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

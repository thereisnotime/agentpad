## 1. Core (internal/settings)

- [x] 1.1 Settings struct, defaults, schema 1, validation with field paths and line numbers
- [x] 1.2 Comment-preserving load/save (yaml.Node), unknown-key preservation, atomic writes
- [x] 1.3 Watcher with debounce and self-write suppression
- [x] 1.4 Migration from a preferences map
- [x] 1.5 Bundle export, import plan (merge/replace), backup, apply
- [x] 1.6 Single-profile export/import with rename on conflict
- [x] 1.7 Tests

## 2. UI (after UI redesign merge)

- [ ] 2.1 Swap ui prefs implementation to settings store, live reload
- [ ] 2.2 Export / Import (preview dialog) / Open folder; per-profile Share (copy/paste/file)

## 3. Docs

- [x] 3.1 docs/configuration.md with full settings reference and bundle format

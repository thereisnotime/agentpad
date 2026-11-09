# Profiles

[← Docs index](README.md)

A profile maps the pad's **3 keys** and the **knob** (rotate left, press, rotate right) to actions.

## Built-in profiles

| Profile | Key 1 | Key 2 | Key 3 | Knob ⟲ / press / ⟳ |
|---|---|---|---|---|
| `claude-code` | `1` approve | `2` always | `Esc` reject | `↑` / `Enter` / `↓` |
| `codex` | `y` approve | `a` always | `Esc` reject | `↑` / `Enter` / `↓` |
| `opencode` | `Enter` once | `a` always | `Esc` reject | `↑` / `Enter` / `↓` |
| `media` | prev | play/pause | next | vol − / mute / vol + |

Prompt shortcuts differ between agent versions; adjust them in the editor if yours differ.

## Layers

**Apply** writes the selected profile to **layer 1** and the next two profiles to layers 2 and 3. The side button on the pad
switches layers. Some boards only keep layer 1, which is why the selected profile always goes first.

The pad doesn't report the side button, so agentpad guesses the layer from the keys it sends: after Apply it assumes
layer 1, and a key only another layer sends moves the guess there (`y` with claude-code, codex, opencode applied means
layer 2). The Profiles editor shows `Pad on layer 2 · codex` and the header pill gets an `L2` suffix; a key several
layers send (`a` on codex and opencode) shows both until another key settles it. The "pad holds a different mapping"
warning only appears for keys no layer sends. In gesture mode every layer holds the same F13–F18 keys, so layers don't
matter there.

## Action grammar

Actions use the [`ch57x-keyboard-tool`](https://github.com/kriomant/ch57x-keyboard-tool) syntax. agentpad validates every action before writing.

| Kind | Examples | Notes |
|---|---|---|
| Key | `a`, `1`, `enter`, `escape`, `up`, `f13` | lowercase |
| Modifiers | `ctrl-c`, `ctrl-shift-t`, `alt-tab`, `win-1` | `ctrl shift alt win` (+ `r` variants) |
| Sequence | `ctrl-a,d` | up to 5 steps; only the first may carry modifiers |
| Media | `play`, `next`, `prev`, `mute`, `volumeup`, `volumedown` | no modifiers, not in sequences |
| Mouse | `click`, `rclick`, `wheelup`, `wheeldown`, `ctrl-click` | not in sequences |
| Raw code | `<104>` | HID usage 0–255 |
| Script | `run:mic-toggle` | runs one of your [scripts](scripts.md); plain mode programs F19–F24 for it |

The editor offers all supported names as suggestions.

## Storage

- Built-in profiles are compiled in.
- Your edits are stored as YAML overrides in `~/.config/agentpad/profiles/<name>.yaml`; **Reset** deletes the override.
  An edited built-in shows a small "edited" tag on its chip.
- Orientation and LED settings are app preferences, not part of profiles.

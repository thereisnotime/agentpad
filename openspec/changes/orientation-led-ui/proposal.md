## Why

The pad can sit in any of four orientations on a desk, so key mapping and the
editor must follow how it is placed. Users want control over the RGB lighting,
and the first UI was hard to read, cramped and did not adapt to window size.

## What Changes

- Pad orientation setting (normal, upside down, clockwise, counter-clockwise).
  Profiles stay in "as seen" order; the generated mapping carries the orientation
  and the button grid shape the device expects.
- Model-aware LED control: `8890` exposes firmware effect modes by index (no
  colour on this firmware); `8840`/`8842` expose per-layer effects with 7 colours
  (+ white for backlight). LED policy for agents uses a full LED setting.
- UI redesign: custom theme, header with status pills, drawn pad that rotates
  with the orientation, click-to-edit keys, LED panel per model, session cards,
  responsive layout for narrow and wide windows, no blocking work on the UI or
  receiver goroutines.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pad-device`: LED writes take a model-aware LED setting; capability discovery per model.
- `key-profiles`: mapping generation supports orientation.
- `desktop-ui`: redesigned, responsive layout; orientation and LED panels.

## Impact

`internal/pad`, `internal/profile`, `internal/ui`. No new runtime dependencies.

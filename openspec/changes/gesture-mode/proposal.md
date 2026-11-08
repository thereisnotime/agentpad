## Why

The pad firmware only stores one action per control. Users want richer input:
hold for a different action, double-tap, and "press while rotating" on the knob.
Measurements on a real `1189:8890` board (libinput, 2026-11-08) show the pad
reports true press/release timing for keys and knob press, sends each knob
detent as a ~8 ms press+release, and force-releases a held key in the same
millisecond the first rotation detent arrives.

## What Changes

- Optional **gesture mode**: the pad is programmed so every control sends a unique
  unused key (F13–F24); agentpad grabs the pad's input devices exclusively,
  recognises gestures and emits the mapped actions through a virtual keyboard.
- Gestures per key: tap, hold, long hold, double-tap; knob: rotate cw/ccw,
  press, hold, press+rotate cw/ccw (detected via the coincident forced release);
  key chords (two keys together).
- Plain mode stays the default; switching modes re-programs the pad.

## Capabilities

### New Capabilities

- `gestures`: reading pad input, gesture recognition, action emission, gesture profiles.

### Modified Capabilities

None (key-profiles and desktop-ui gain gesture fields in this change's design; requirements live in `gestures`).

## Impact

- Needs read/grab access to the pad's `/dev/input/event*` nodes (udev rule with `uaccess` on the input subsystem) and write access to `/dev/uinput` or a running `ydotoold`.
- Gestures only work while agentpad runs; plain mode works anywhere.

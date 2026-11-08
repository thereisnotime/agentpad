# Gesture mode

The pad firmware stores one action per control. Gesture mode gets more out of
the same three keys and knob: tap, hold, long hold, double-tap, two-key chords
and "press while rotating" on the knob. Each gesture can emit its own action.

Plain mode stays the default and works without agentpad running. Gesture mode
only works **while agentpad runs**.

## How it works

1. agentpad programs the pad so every control sends a unique key nobody else
   uses (all three layers are the same):

   | Control (as seen) | Key sent |
   | ----------------- | -------- |
   | key 1             | F13      |
   | key 2             | F14      |
   | key 3             | F15      |
   | knob left (ccw)   | F16      |
   | knob press        | F17      |
   | knob right (cw)   | F18      |

   The mapping uses the configured orientation, so "key 1" is always the key
   you see first (left, or top when the pad stands vertically).
   `ch57x-keyboard-tool show-keys` lists f13..f24 and `validate` accepts the
   mapping in every orientation; F19..F24 are spare.
2. agentpad opens the pad's `/dev/input/event*` nodes (found via
   `/sys/class/input/event*/device/id`) and grabs them (`EVIOCGRAB`), so the raw
   F13..F18 presses never reach other apps.
3. A recogniser turns presses and releases into gestures.
4. The active profile's action for that gesture is typed through a virtual
   keyboard: `/dev/uinput` if writable, otherwise `ydotool` (needs a running
   `ydotoold`; the socket is taken from `YDOTOOL_SOCKET`, `/run/ydotoold.socket`
   or `$XDG_RUNTIME_DIR/.ydotool_socket`).

If the pad is unplugged, agentpad keeps retrying every second and picks it up
again when it comes back. Leaving gesture mode (or quitting agentpad) releases
the grab and uploads the active profile's plain mapping again.

## Measured firmware behaviour (1189:8890)

Recorded with `libinput debug-events` (see `just gesture-trace` and
`internal/gesture/testdata/trace-8890.txt`):

- Keys and the knob press report real press and release times, so holds can
  be measured (one hold: pressed +0.000 s, released +3.056 s).
- Every knob detent is a press and a release about 8 ms apart.
- Holding the knob and then rotating: the firmware releases the held press in
  **the same millisecond** as the first detent's press. A genuine release
  never lines up with a detent like that, which is how press+rotate is
  detected.
- All key events come from one keyboard node; the pad also exposes a second
  keyboard node (media keys) and a mouse node. agentpad grabs all of them.

## Gestures and thresholds

| Gesture       | Controls          | Rule (defaults)                                                         |
| ------------- | ----------------- | ----------------------------------------------------------------------- |
| `tap`         | keys, knob press  | released before the hold threshold (450 ms)                             |
| `hold`        | keys, knob press  | held at least 450 ms                                                    |
| `longhold`    | keys, knob press  | held at least 1500 ms                                                   |
| `doubletap`   | keys, knob press  | second press within 300 ms of the first release                         |
| `chord`       | two keys          | second key pressed within 80 ms of the first (`key1+key3` etc.)        |
| `rotate`      | knob ccw / cw     | one detent                                                              |
| `pressrotate` | knob ccw / cw     | knob press released within 2 ms of a detent; continues until no detent for 500 ms |

What gets delayed depends on what the profile configures:

- A tap is only delayed by the double-tap window when that control has a
  `doubletap` action. Otherwise it fires on release.
- A key `hold` fires at 450 ms while you are still holding, unless the key
  also has a `longhold` action. Then the hold is decided on release (between
  450 and 1500 ms) and the long hold fires at 1500 ms.
- Without `hold`/`longhold` actions a long press is just a tap, so plain
  profiles behave as expected.
- The knob press hold is always decided **on release**, because holding the
  knob and then rotating must give only press+rotate. The firmware only sends
  a genuine release when no rotation happened. (The recogniser has a
  `KnobEagerHold` option to fire it at the threshold instead; then a hold that
  already fired before you started rotating stays fired.)
- A chord swallows both keys: no tap or hold for either.
- A knob tap waits 2 ms for a possible coincident detent.

Tap falls back to the profile's plain key (or knob press) action and rotate to
the plain knob action, so every profile works in gesture mode.

## Profiles

Gestures live in an optional `gestures:` map (control -> gesture -> action) in
the profile YAML. Old profiles without it keep working.

```yaml
name: claude-code
keys: ["1", "2", escape]
knob: {ccw: up, press: enter, cw: down}
gestures:
  key3:
    hold: ctrl-c
  knob-ccw:
    pressrotate: shift-tab
  knob-cw:
    pressrotate: shift-tab
  key1+key3:
    chord: escape,escape
```

Controls: `key1`, `key2`, `key3`, `knob-ccw`, `knob-press`, `knob-cw`, and the
chords `key1+key2`, `key1+key3`, `key2+key3`. Actions use the same grammar as
the plain mapping (`ctrl-c`, `ctrl-a,b`, `volumeup`, `<110>`), except mouse
actions.

### Defaults

Only the built-in `claude-code` profile ships gestures:

| Control  | Gesture       | Action      | Why                                   |
| -------- | ------------- | ----------- | ------------------------------------- |
| key 3    | `hold`        | `ctrl-c`    | interrupt Claude (tap is still Esc)   |
| knob ccw | `pressrotate` | `shift-tab` | cycle the permission mode             |
| knob cw  | `pressrotate` | `shift-tab` | cycle the permission mode             |
| knob press | `longhold`  | `agentpad:jump` | jump to the waiting agent        |

### agentpad actions

Gesture actions starting with `agentpad:` are not typed. agentpad runs them
itself (gesture mode only; the pad firmware can't send them in plain mode):

| Action                  | Does                                                                 |
| ----------------------- | -------------------------------------------------------------------- |
| `agentpad:jump`         | [jump](attention-jump.md) to the waiting agent; again cycles through several, oldest first |
| `agentpad:next-profile` | switch to the next profile (in gesture mode the pad mapping stays)   |

Any other `agentpad:` name is rejected when the profile is saved.

## Using it

- **Switch modes** on the Pad screen (Plain / Gesture) or in the tray menu
  (Mode). Switching to gesture programs the F13..F18 mapping, grabs the pad and
  shows which virtual keyboard is in use (uinput or ydotool). Switching back to
  plain, quitting (window, tray Quit, Ctrl-C or SIGTERM) stops gesture mode and
  uploads the active profile's plain mapping again, so the pad keeps working
  without agentpad.
- **Edit gestures** on the Profiles screen: in gesture mode the inspector shows
  one slot per gesture for the selected control (tap / hold / long hold /
  double-tap, or turn / press + turn for the knob) plus the three chords. An
  empty slot is not used; tap and turn fall back to the plain action, shown as
  the placeholder.
- **Watch it work**: in gesture mode the Profiles screen lists the last
  recognised gestures, e.g. `key 1 · hold → ctrl-c`.
- Applying a profile in gesture mode switches the profile agentpad uses; the
  pad keeps its gesture mapping.
- Thresholds (hold, long hold, double-tap) come from the settings; there is no
  editor for them yet.

## Permissions

Gesture mode needs read access to the pad's input nodes and write access to
`/dev/uinput` (or a running `ydotoold`). `just udev-install` installs these
rules to `/etc/udev/rules.d/50-agentpad.rules`, giving the logged-in user
access through `uaccess`:

```
SUBSYSTEM=="usb", ATTR{idVendor}=="1189", ATTR{idProduct}=="8890|8840|8842", TAG+="uaccess"
SUBSYSTEM=="input", ATTRS{idVendor}=="1189", ATTRS{idProduct}=="8890|8840|8842", TAG+="uaccess"
KERNEL=="uinput", SUBSYSTEM=="misc", TAG+="uaccess", OPTIONS+="static_node=uinput"
```

Replug the pad afterwards. If `/dev/uinput` is still root-only, log out and
back in, or run `ydotoold`.

## Live view in plain mode

agentpad can also watch the pad without grabbing it, so the keys still reach
your apps and the pad drawing lights up as you press. In plain mode it maps
keys back to controls from the active profile. If two controls send the same
key (say key 1 and the knob press both send Enter), it can't tell them apart.
It shows the first one in key1, key2, key3, knob-ccw, knob-press, knob-cw
order and the editor warns about it ("Key 1 and knob press both send Enter, so
live input can't tell them apart"). Mouse actions can't be seen this way.
Live input needs the same input permission as gesture mode (`just
udev-install`, then replug the pad); without it the Profiles screen says so.

## Limitations

- Only works while agentpad runs. Quitting uploads the plain mapping again;
  if agentpad is killed hard (SIGKILL, crash, pad unplugged at the moment of
  quitting) the pad keeps sending F13..F18 until you apply a profile.
- Mouse actions (`click`, `wheel(...)`, `move(...)`, `drag(...)`) are not
  supported in gesture mode.
- Press+rotate ends after 500 ms without a detent because the firmware does
  not report when you let go of the knob after rotating. A plain rotation
  within 500 ms of a press+rotate counts as press+rotate too. In the recorded
  trace a slow fourth detent came 792 ms after the third, so it counted as a
  plain rotate. Raise `PressRotateIdle` if you rotate slowly.
- Custom codes `<n>` work only for HID usages agentpad knows a Linux keycode
  for (the standard keyboard page). Others are rejected when emitted.
- Linux only (evdev, uinput).

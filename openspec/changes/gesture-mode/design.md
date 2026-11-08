## Observations (real 8890, libinput debug-events)

- All key events arrive on one evdev node (`HID 1189:8890`, keyboard); a second
  keyboard node and a mouse node exist for consumer/mouse actions.
- Key hold: `pressed` +0.000 s, `released` +3.056 s -> true hold timing.
- Knob detent: press+release 8 ms apart per detent.
- Knob held then rotated: the held key's `released` has the same timestamp as the
  first detent's `pressed` (forced release). A genuine release never coincides.

## Decisions

1. **Unique keycodes**: gesture mode writes a mapping with F13..F18 for keys 1–3,
   knob ccw/press/cw (F19–F24 spare). Detection maps keycode -> control.
2. **Input**: pure-Go evdev reader (read `input_event` structs from the node,
   `EVIOCGRAB` ioctl via `golang.org/x/sys/unix`), nodes found via sysfs by USB ID.
   Re-scan on disconnect.
3. **Recogniser** (pure, time-injected state machine, fully unit-tested):
   - tap: release < hold threshold (default 450 ms); double-tap: second tap within
     300 ms (tap emission delayed by that window only if a double-tap action is set);
   - hold fires at threshold while still pressed; long hold at 1500 ms;
   - press+rotate: knob press release with |t(release) - t(detent)| <= 2 ms enters
     "press-rotate" mode; detents emit press+rotate actions until no detent for 500 ms;
   - chord: two keys pressed within 80 ms of each other.
4. **Output**: `/dev/uinput` virtual keyboard via Go (preferred), fallback `ydotool`.
   Actions use the same action grammar as profiles (plus agentpad-internal actions
   like `agentpad:next-session` later).
5. **Profiles**: a profile gains optional `gestures:` map control -> gesture -> action.
   Plain fields remain the plain-mode mapping.

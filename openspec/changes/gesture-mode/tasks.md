## 1. Core

- [x] 1.1 Gesture recogniser state machine (pure, injected clock) + exhaustive tests from recorded traces
- [x] 1.2 evdev reader + grab for the pad nodes (sysfs discovery by USB ID)
- [x] 1.3 uinput virtual keyboard output, ydotool fallback
- [x] 1.4 Gesture fields in profiles, gesture-mode mapping (F13–F18)

## 2. UI

- [ ] 2.1 Mode switch (plain / gesture) with permission checks
- [ ] 2.2 Inspector shows tap / hold / long hold / double-tap / press+rotate actions per control
- [ ] 2.3 Live gesture monitor (what agentpad recognised)

## 3. Setup

- [x] 3.1 udev rules for input nodes and uinput, `just udev-install` update, README section (docs/gestures.md; README link pending)

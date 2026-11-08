## 1. Window tracking

- [ ] 1.1 Backend interface + session detection
- [ ] 1.2 X11 backend (pure Go, event-driven) + tests against a fake/Xvfb where available
- [ ] 1.3 KWin backend (script load/unload, D-Bus callback) + tests with a private bus where available
- [ ] 1.4 Process-tree agent detection

## 2. Rules and switching

- [ ] 2.1 Rules model, YAML store, starter rules, evaluation tests
- [ ] 2.2 Switching: gesture mode instant, plain mode dwell + dedupe

## 3. UI

- [ ] 3.1 Header source pill, pin/override
- [ ] 3.2 Rules editor

## 4. Verify

- [ ] 4.1 Manual: KDE Wayland focus switching
- [ ] 4.2 Manual: X11 session (Xvfb/Xephyr or a real X11 login)

## 5. Attention jump

- [x] 5.1 Hook origin capture (env + agent pid + tty), v2 event envelope, events.Origin
- [x] 5.2 Jump backends: zellij, tmux, Konsole D-Bus, window raise (KWin / X11)
- [x] 5.3 Waiting queue + cycle, desktop notification with Jump action
- [ ] 5.4 UI/tray/gesture triggers (after UI + gesture merges)

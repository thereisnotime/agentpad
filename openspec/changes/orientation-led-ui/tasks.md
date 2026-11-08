## 1. Backend

- [x] 1.1 `profile.Orientation`, `MappingYAMLOriented`, knob/key placement helpers + tests (validated by the real tool)
- [x] 1.2 `pad.LEDSetting`, `pad.CapsFor`, `ToolWriter.SetLED(LEDSetting)` arg building + validation + tests
- [x] 1.3 `LEDController` on `LEDSetting`
- [x] 1.4 `just led-probe` recipe to cycle 8890 modes on real hardware

## 2. UI

- [x] 2.1 Custom theme, header status pills, adaptive navigation
- [x] 2.2 Drawn, rotatable pad editor with click-to-edit
- [x] 2.3 LED panel per model, LED policy using `LEDSetting`
- [x] 2.4 Session cards, log polish, setup polish
- [x] 2.5 Device write worker; no USB on UI or receiver goroutines
- [x] 2.6 Screenshots at 500 px and 1200 px
- [x] 2.7 Setup: Edit file opens (or creates empty) the snippet's target file

## 3. Verify

- [ ] 3.1 `just ci`
- [ ] 3.2 Real pad: rotate + apply, LED modes

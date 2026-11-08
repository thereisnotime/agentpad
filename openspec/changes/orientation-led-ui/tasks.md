## 1. Backend

- [ ] 1.1 `profile.Orientation`, `MappingYAMLOriented`, knob/key placement helpers + tests (validated by the real tool)
- [ ] 1.2 `pad.LEDSetting`, `pad.CapsFor`, `ToolWriter.SetLED(LEDSetting)` arg building + validation + tests
- [ ] 1.3 `LEDController` on `LEDSetting`
- [ ] 1.4 `just led-probe` recipe to cycle 8890 modes on real hardware

## 2. UI

- [ ] 2.1 Custom theme, header status pills, adaptive navigation
- [ ] 2.2 Drawn, rotatable pad editor with click-to-edit
- [ ] 2.3 LED panel per model, LED policy using `LEDSetting`
- [ ] 2.4 Session cards, log polish, setup polish
- [ ] 2.5 Device write worker; no USB on UI or receiver goroutines
- [ ] 2.6 Screenshots at 500 px and 1200 px

## 3. Verify

- [ ] 3.1 `just ci`
- [ ] 3.2 Real pad: rotate + apply, LED modes

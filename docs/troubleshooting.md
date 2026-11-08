# Troubleshooting

[← Docs index](README.md)

| Symptom | Likely cause | Fix |
|---|---|---|
| Pad has no light, nothing in `lsusb`, kernel log silent | USB-C host + pad without CC resistors, or charge-only cable | Use a USB-C → USB-A adapter + USB-A → USB-C data cable. See [Hardware](hardware.md#usb-c-power-quirk) |
| Pad lights up but keys type nothing useful | Pad still has the factory mapping (often every key = same letter) | **Profiles → Apply to pad** |
| "Permission denied" when applying | udev rule missing | `just udev-install`, then replug |
| "ch57x-keyboard-tool not found" | backend not installed | `just tool-install` |
| "Pad not connected" | not detected over USB | `just detect`; check cable/port |
| "Live events disabled" / receiver off | port 47321 already used, usually another agentpad instance | quit the other instance |
| Agent activity never shows up | hooks not installed, or wrong binary path | copy the snippet from **Setup**; `just fake-event` to test the app side |
| Codex never shows "attention" | `notify` only reports finished turns | expected for now; see [Agent integration](agents.md#codex-cli) |
| LED colours wrong on `8840` | upstream bug in ch57x-keyboard-tool v1.7.0 | see [Hardware → LEDs](hardware.md#leds) |

Still stuck? **About → Report a bug** in the app pre-fills version, OS and pad, or open a
[bug report](https://github.com/thereisnotime/agentpad/issues/new?template=bug_report.yml).

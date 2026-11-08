# Development

[← Docs index](README.md)

## Toolchain

Pinned in [`.tool-versions`](../.tool-versions) (asdf): **Go 1.27**, **just 1.58**. Optional: `golangci-lint` v2.

Fyne needs cgo and the X11/GL headers for the default (X11) backend:

```bash
# Fedora
sudo dnf install gcc libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel mesa-libGL-devel
# Debian / Ubuntu
sudo apt install gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev
```

## Build backends

| Backend | Command | Runs on |
|---|---|---|
| **X11** (default, used for releases) | `just build` | X11 sessions and Wayland via XWayland |
| Native Wayland | `TAGS=wayland just build` | Wayland sessions |

`just` picks the Wayland backend automatically when the X11 headers are missing.

## Recipes

Run `just` to list everything. The main ones:

| Group | Recipe | What it does |
|---|---|---|
| build | `build`, `run`, `install`, `clean` | build into `bin/`, start, install to `~/.local/bin` |
| test | `test`, `cover` | race-enabled tests, coverage |
| quality | `fmt`, `lint`, `tidy`, `ci` | gofmt, golangci-lint (or go vet), everything CI runs |
| device | `detect`, `udev-rule`, `udev-install`, `tool-install`, `led-probe` | pad detection, permissions, backend, LED mode discovery |
| dev | `fake-event`, `log` | fake Claude event, tail the event log |
| spec | `spec`, `spec-list` | validate / list OpenSpec changes |

## Tests

- Unit tests never touch a real pad, the network (beyond loopback) or real input devices.
- Tests that need `ch57x-keyboard-tool` (config validation) are skipped when it isn't installed.
- UI tests use Fyne's headless `test` driver; pure UI logic lives in Fyne-free files.

## Specs

Behaviour is specified with [OpenSpec](https://openspec.dev) in [`openspec/`](../openspec/). Every change starts as a change
proposal (`openspec new change <name>`) with proposal, design, delta specs and tasks, and is archived when done.

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs golangci-lint, tests and the build on Ubuntu for every push and PR.

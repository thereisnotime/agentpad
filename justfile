# agentpad task runner. Run `just` to list recipes.

set shell := ["bash", "-euo", "pipefail", "-c"]
set dotenv-load := true

bin := "bin/agentpad"
pkg := "./cmd/agentpad"
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X main.version=" + version
# Fyne backend: native Wayland when X11 dev headers are missing (override with TAGS=...)
tags := env("TAGS", `pkg-config --exists xcursor xrandr xinerama xi xxf86vm 2>/dev/null && echo "" || echo wayland`)

# colors
c_reset := '\033[0m'
c_bold := '\033[1m'
c_dim := '\033[2m'
c_green := '\033[32m'
c_yellow := '\033[33m'
c_blue := '\033[34m'
c_red := '\033[31m'

# List all recipes
[group('help')]
default:
    @just --list --unsorted

# Show tool versions and environment
[group('help')]
info:
    @printf "{{ c_bold }}agentpad{{ c_reset }} {{ c_dim }}{{ version }}{{ c_reset }}\n"
    @printf "{{ c_blue }}go{{ c_reset }}        %s\n" "$(go version | cut -d' ' -f3)"
    @printf "{{ c_blue }}just{{ c_reset }}      %s\n" "$(just --version | cut -d' ' -f2)"
    @printf "{{ c_blue }}ch57x{{ c_reset }}     %s\n" "$(ch57x-keyboard-tool --version 2>/dev/null || echo 'not installed')"
    @printf "{{ c_blue }}pad{{ c_reset }}       %s\n" "$(grep -l '^1189$' /sys/bus/usb/devices/*/idVendor 2>/dev/null | head -1 | xargs -r dirname | xargs -r -I{} cat {}/idProduct || echo 'not connected')"

# Build the binary into bin/
[group('build')]
build:
    @printf "{{ c_blue }}▸ build{{ c_reset }} {{ bin }}\n"
    @mkdir -p bin
    go build -trimpath -tags "{{ tags }}" -ldflags "{{ ldflags }}" -o {{ bin }} {{ pkg }}
    @printf "{{ c_green }}✔ built{{ c_reset }} {{ bin }} ({{ version }}{{ if tags != "" { ", " + tags } else { "" } }})\n"

# Build and launch the desktop app
[group('build')]
run: build
    ./{{ bin }}

# Install the binary to ~/.local/bin
[group('build')]
install: build
    install -Dm755 {{ bin }} "$HOME/.local/bin/agentpad"
    @printf "{{ c_green }}✔ installed{{ c_reset }} ~/.local/bin/agentpad\n"

# Remove build artifacts
[group('build')]
clean:
    rm -rf bin dist coverage.out
    @printf "{{ c_green }}✔ clean{{ c_reset }}\n"

# Run unit tests
[group('test')]
test *args:
    @printf "{{ c_blue }}▸ test{{ c_reset }}\n"
    go test -race -count=1 -tags "{{ tags }}" {{ args }} ./...

# Run tests with coverage report
[group('test')]
cover:
    go test -race -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out | tail -1

# Format Go code
[group('quality')]
fmt:
    gofmt -s -w .
    @printf "{{ c_green }}✔ formatted{{ c_reset }}\n"

# Lint (golangci-lint if installed, else go vet)
[group('quality')]
lint:
    @printf "{{ c_blue }}▸ lint{{ c_reset }}\n"
    @test -z "$(gofmt -s -l .)" || { printf "{{ c_red }}✘ gofmt needed:{{ c_reset }}\n"; gofmt -s -l .; exit 1; }
    @if command -v golangci-lint >/dev/null && golangci-lint version >/dev/null 2>&1; then golangci-lint run --build-tags "{{ tags }}" ./...; else printf "{{ c_yellow }}! golangci-lint missing, using go vet{{ c_reset }}\n"; go vet -tags "{{ tags }}" ./...; fi
    @printf "{{ c_green }}✔ lint ok{{ c_reset }}\n"

# Tidy go.mod / go.sum
[group('quality')]
tidy:
    go mod tidy

# Everything CI runs: lint, test, build
[group('quality')]
ci: lint test build

# Show the udev rule needed to program the pad without root
[group('device')]
udev-rule:
    @printf 'SUBSYSTEM=="usb", ATTR{idVendor}=="1189", ATTR{idProduct}=="8890|8840|8842", TAG+="uaccess"\n'

# Install the udev rule (needs sudo)
[group('device')]
udev-install:
    just udev-rule | sudo tee /etc/udev/rules.d/50-agentpad.rules >/dev/null
    sudo udevadm control --reload && sudo udevadm trigger
    @printf "{{ c_green }}✔ udev rule installed{{ c_reset }}, replug the pad\n"

# Install ch57x-keyboard-tool (device backend) to ~/.local/bin
[group('device')]
tool-install tag="v1.7.0":
    #!/usr/bin/env bash
    set -euo pipefail
    tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
    curl -fsSL "https://github.com/kriomant/ch57x-keyboard-tool/releases/download/{{ tag }}/ch57x-keyboard-tool-x86_64-unknown-linux-gnu.tar.gz" | tar xz -C "$tmp"
    install -Dm755 "$tmp/ch57x-keyboard-tool" "$HOME/.local/bin/ch57x-keyboard-tool"
    printf "{{ c_green }}✔ installed{{ c_reset }} ch57x-keyboard-tool {{ tag }}\n"

# Show whether a supported pad is connected
[group('device')]
detect:
    @lsusb -d 1189: || printf "{{ c_yellow }}no CH57x pad found{{ c_reset }}\n"

# Send a fake Claude Code event to the running app
[group('dev')]
fake-event kind="permission_prompt":
    @printf '{"hook_event_name":"Notification","notification_type":"{{ kind }}","session_id":"dev-1","cwd":"%s","message":"Claude needs your permission to use Bash"}' "$PWD" | go run -tags "{{ tags }}" {{ pkg }} hook claude
    @printf "{{ c_green }}✔ sent{{ c_reset }} {{ kind }}\n"

# Tail the event log
[group('dev')]
log:
    tail -f "${XDG_STATE_HOME:-$HOME/.local/state}/agentpad/events.jsonl"

# Validate the OpenSpec specs
[group('spec')]
spec:
    openspec validate --all --strict

# List OpenSpec changes
[group('spec')]
spec-list:
    openspec list

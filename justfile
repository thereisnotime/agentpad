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
    "$HOME/.local/bin/agentpad" desktop-install
    @printf "{{ c_green }}✔ installed{{ c_reset }} ~/.local/bin/agentpad\n"

# Remove agentpad, its desktop entry and icons from ~/.local
[group('build')]
uninstall:
    -"$HOME/.local/bin/agentpad" desktop-remove
    rm -f "$HOME/.local/bin/agentpad"
    @printf "{{ c_green }}✔ uninstalled{{ c_reset }} (settings in ~/.config/agentpad kept)\n"

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
    git ls-files -z "*.go" | xargs -0 gofmt -s -w
    @printf "{{ c_green }}✔ formatted{{ c_reset }}\n"

# Lint with the pinned golangci-lint (tools/go.mod)
[group('quality')]
lint:
    @printf "{{ c_blue }}▸ lint{{ c_reset }}\n"
    @test -z "$(git ls-files -z "*.go" | xargs -0 gofmt -s -l)" || { printf "{{ c_red }}✘ gofmt needed:{{ c_reset }}\n"; git ls-files -z "*.go" | xargs -0 gofmt -s -l; exit 1; }
    go tool -modfile=tools/go.mod golangci-lint run --build-tags "{{ tags }}" ./...
    @printf "{{ c_green }}✔ lint ok{{ c_reset }}\n"

# Tidy go.mod / go.sum
[group('quality')]
tidy:
    go mod tidy

# Everything CI runs: lint, test, build
[group('quality')]
ci: lint test build

# Show the udev rules: program the pad, read its input nodes and use /dev/uinput (gesture mode) without root
[group('device')]
udev-rule:
    @go run -tags "{{ tags }}" {{ pkg }} udev-rules

# Install the udev rules (needs sudo)
[group('device')]
udev-install:
    just udev-rule | sudo tee /etc/udev/rules.d/50-agentpad.rules >/dev/null
    sudo udevadm control --reload && sudo udevadm trigger
    sudo udevadm trigger --subsystem-match=misc --sysname-match=uinput
    @printf "{{ c_green }}✔ udev rules installed{{ c_reset }}, replug the pad (log out and in if /dev/uinput stays root-only)\n"

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

# Cycle 8890 LED modes on the real pad; watch it and note what each mode does
[group('device')]
led-probe from="0" to="6" secs="4":
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'ch57x-keyboard-tool led 0 >/dev/null 2>&1 || true; exit 130' INT TERM
    for n in $(seq {{ from }} {{ to }}); do
        printf "{{ c_bold }}{{ c_blue }}mode %s{{ c_reset }}\n" "$n"
        ch57x-keyboard-tool led "$n"
        sleep {{ secs }}
    done
    ch57x-keyboard-tool led 0
    printf "{{ c_green }}✔ done{{ c_reset }}, leds back to mode 0 (off)\n"

# Send a fake Claude Code event to the running app
[group('dev')]
fake-event kind="permission_prompt":
    @printf '{"hook_event_name":"Notification","notification_type":"{{ kind }}","session_id":"dev-1","cwd":"%s","message":"Claude needs your permission to use Bash"}' "$PWD" | go run -tags "{{ tags }}" {{ pkg }} hook claude
    @printf "{{ c_green }}✔ sent{{ c_reset }} {{ kind }}\n"

# Render the README demo offscreen into docs/demo (gif + mp4, 1.5x; needs ffmpeg, gifsicle optional)
[group('dev')]
demo-gif width="native":
    #!/usr/bin/env bash
    set -euo pipefail
    command -v ffmpeg >/dev/null || { printf "{{ c_red }}✘ ffmpeg not found{{ c_reset }}\n"; exit 1; }
    frames=$(mktemp -d); trap 'rm -rf "$frames"' EXIT
    out="$PWD/docs/demo"; mkdir -p "$out"
    printf "{{ c_blue }}▸ rendering frames{{ c_reset }} {{ c_dim }}(offscreen, takes a couple of minutes){{ c_reset }}\n"
    DEMO_DIR="$frames" go test -count=1 -timeout 15m -tags "{{ tags }} screenshots" -run '^TestDemo$' ./internal/ui
    in=(-loglevel error -y -framerate 12 -i "$frames/frame-%04d.png")
    scale="null"; [ "{{ width }}" = native ] || scale="scale={{ width }}:-2:flags=lanczos"
    printf "{{ c_blue }}▸ encoding{{ c_reset }} agentpad-demo.gif\n"
    ffmpeg "${in[@]}" -vf "$scale,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=sierra2_4a:diff_mode=rectangle" \
        -loop 0 "$out/agentpad-demo.gif"
    if command -v gifsicle >/dev/null; then gifsicle -O3 --batch "$out/agentpad-demo.gif"; else printf "{{ c_dim }}gifsicle not installed, skipping -O3{{ c_reset }}\n"; fi
    printf "{{ c_blue }}▸ encoding{{ c_reset }} agentpad-demo.mp4\n"
    ffmpeg "${in[@]}" -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" -c:v libx264 -preset slow -crf 23 -pix_fmt yuv420p -movflags +faststart "$out/agentpad-demo.mp4"
    printf "{{ c_green }}✔ demo{{ c_reset }}\n"
    du -h "$out/agentpad-demo.gif" "$out/agentpad-demo.mp4" | sed "s|$PWD/||"

# Record raw key events from the pad for a new gesture trace (libinput, Ctrl-C to stop)
[group('dev')]
gesture-trace:
    #!/usr/bin/env bash
    set -euo pipefail
    command -v libinput >/dev/null || { printf "{{ c_red }}✘ libinput not found{{ c_reset }} (install libinput-utils)\n"; exit 1; }
    mapfile -t nodes < <(awk '
        /^I:/ { ok = ($0 ~ /Vendor=1189/ && $0 ~ /Product=(8890|8840|8842)/) }
        /^H:/ && ok { for (i = 1; i <= NF; i++) if ($i ~ /^event[0-9]+$/) print "/dev/input/" $i }
    ' /proc/bus/input/devices)
    if [ ${#nodes[@]} -eq 0 ]; then printf "{{ c_yellow }}no CH57x pad input nodes found{{ c_reset }}\n"; exit 1; fi
    args=(); for n in "${nodes[@]}"; do args+=(--device "$n"); done
    sudo=""; [ -r "${nodes[0]}" ] || sudo="sudo"
    printf "{{ c_blue }}▸ tracing{{ c_reset }} %s {{ c_dim }}(save the output under internal/gesture/testdata/){{ c_reset }}\n" "${nodes[*]}"
    $sudo libinput debug-events --show-keycodes "${args[@]}"

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

#!/bin/sh
# agentpad installer: downloads the latest release for this machine, verifies
# its checksum and installs it for the current user.
#
#   curl -fsSL https://raw.githubusercontent.com/thereisnotime/agentpad/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --udev            # also install the udev rules (sudo)
#
# Options:
#   --version vX.Y.Z   install a specific release (default: latest)
#   --prefix DIR       install directory (default: ~/.local/bin)
#   --udev             install /etc/udev/rules.d/50-agentpad.rules with sudo
#   --no-tool          skip ch57x-keyboard-tool (needed to program the pad)
#   --no-desktop       skip the desktop entry and icon
#   --uninstall        remove agentpad (keeps ~/.config/agentpad)
set -eu

REPO="thereisnotime/agentpad"
TOOL_REPO="kriomant/ch57x-keyboard-tool"
TOOL_VERSION="v1.7.0"
VERSION="latest"
PREFIX="${HOME}/.local/bin"
UDEV=0
TOOL=1
DESKTOP=1
UNINSTALL=0

if [ -t 1 ]; then
	B=$(printf '\033[1m'); G=$(printf '\033[32m'); Y=$(printf '\033[33m'); R=$(printf '\033[31m'); D=$(printf '\033[2m'); N=$(printf '\033[0m')
else
	B=""; G=""; Y=""; R=""; D=""; N=""
fi
say() { printf '%s▸%s %s\n' "$B" "$N" "$*"; }
ok() { printf '%s✔%s %s\n' "$G" "$N" "$*"; }
warn() { printf '%s!%s %s\n' "$Y" "$N" "$*" >&2; }
die() { printf '%s✘ %s%s\n' "$R" "$*" "$N" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case "$1" in
	--version) VERSION="$2"; shift 2 ;;
	--prefix) PREFIX="$2"; shift 2 ;;
	--udev) UDEV=1; shift ;;
	--no-tool) TOOL=0; shift ;;
	--no-desktop) DESKTOP=0; shift ;;
	--uninstall) UNINSTALL=1; shift ;;
	-h | --help) sed -n '2,17p' "$0" 2>/dev/null || true; exit 0 ;;
	*) die "unknown option: $1" ;;
	esac
done

[ "$(uname -s)" = "Linux" ] || die "agentpad runs on Linux only"
need() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }
need curl
need tar
need uname

if [ "$UNINSTALL" = 1 ]; then
	[ -x "$PREFIX/agentpad" ] && "$PREFIX/agentpad" desktop-remove || true
	rm -f "$PREFIX/agentpad"
	ok "removed $PREFIX/agentpad (settings in ~/.config/agentpad kept; ch57x-keyboard-tool and udev rules left in place)"
	exit 0
fi

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64; TOOL_ARCH=x86_64 ;;
aarch64 | arm64) ARCH=arm64; TOOL_ARCH=aarch64 ;;
*) die "unsupported CPU: $(uname -m) (amd64 and arm64 are supported)" ;;
esac

if [ "$VERSION" = "latest" ]; then
	BASE="https://github.com/$REPO/releases/latest/download"
else
	BASE="https://github.com/$REPO/releases/download/$VERSION"
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

ASSET="agentpad_linux_${ARCH}.tar.gz"
say "downloading ${B}agentpad${N} ($VERSION, linux/$ARCH)"
curl -fsSL "$BASE/$ASSET" -o "$TMP/$ASSET" || die "download failed: $BASE/$ASSET"
curl -fsSL "$BASE/checksums.txt" -o "$TMP/checksums.txt" || die "download failed: checksums.txt"

say "verifying checksum"
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$TMP" && grep " $ASSET\$" checksums.txt | sha256sum -c - >/dev/null) || die "checksum mismatch for $ASSET"
else
	warn "sha256sum not found; skipping checksum verification"
fi

tar -C "$TMP" -xzf "$TMP/$ASSET"
mkdir -p "$PREFIX"
install -m 0755 "$TMP/agentpad/agentpad" "$PREFIX/agentpad"
ok "installed $PREFIX/agentpad ($("$PREFIX/agentpad" version 2>/dev/null || echo "?"))"

if [ "$TOOL" = 1 ]; then
	if command -v ch57x-keyboard-tool >/dev/null 2>&1; then
		ok "ch57x-keyboard-tool already installed ($(command -v ch57x-keyboard-tool))"
	else
		say "installing ch57x-keyboard-tool $TOOL_VERSION (programs the pad)"
		TA="ch57x-keyboard-tool-${TOOL_ARCH}-unknown-linux-gnu.tar.gz"
		curl -fsSL "https://github.com/$TOOL_REPO/releases/download/$TOOL_VERSION/$TA" -o "$TMP/$TA" ||
			die "download failed: ch57x-keyboard-tool ($TA)"
		mkdir -p "$TMP/tool" && tar -C "$TMP/tool" -xzf "$TMP/$TA"
		install -m 0755 "$TMP/tool/ch57x-keyboard-tool" "$PREFIX/ch57x-keyboard-tool"
		ok "installed $PREFIX/ch57x-keyboard-tool"
	fi
fi

if [ "$DESKTOP" = 1 ]; then
	"$PREFIX/agentpad" desktop-install >/dev/null 2>&1 && ok "desktop entry and icon installed" ||
		warn "could not install the desktop entry (the app still works)"
fi

RULES=/etc/udev/rules.d/50-agentpad.rules
if [ "$UDEV" = 1 ]; then
	say "installing udev rules to $RULES (sudo)"
	"$PREFIX/agentpad" udev-rules | sudo tee "$RULES" >/dev/null
	sudo udevadm control --reload && sudo udevadm trigger
	ok "udev rules installed; unplug and replug the pad"
elif [ ! -f "$RULES" ]; then
	warn "udev rules not installed: without them agentpad can't program or read the pad."
	printf '  %srun:%s %s udev-rules | sudo tee %s && sudo udevadm control --reload && sudo udevadm trigger\n' "$D" "$N" "$PREFIX/agentpad" "$RULES"
fi

case ":$PATH:" in
*":$PREFIX:"*) ;;
*) warn "$PREFIX is not on your PATH; add it to your shell profile" ;;
esac

printf '\n%sDone.%s Start it with %sagentpad%s (or from your app launcher).\n' "$G" "$N" "$B" "$N"
printf '%sPad dead on a USB-C laptop? Use a USB-C → USB-A adapter: https://github.com/%s/blob/main/docs/hardware.md#usb-c-power-quirk%s\n' "$D" "$REPO" "$N"

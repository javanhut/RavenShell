#!/bin/sh
# install.sh - build and install RavenShell.
#
# Usage:
#   ./install.sh                 install to /usr/local/bin (uses sudo if needed)
#   PREFIX=~/.local ./install.sh install to ~/.local/bin (no sudo)
set -eu

BINARY=ravenshell
PREFIX="${PREFIX:-/usr/local}"
BINDIR="$PREFIX/bin"

if ! command -v go >/dev/null 2>&1; then
	echo "error: Go is required to build RavenShell (https://go.dev/dl/)" >&2
	exit 1
fi

# Build from the directory containing this script.
cd "$(dirname "$0")"
SRCDIR="$(pwd)"

echo "Building $BINARY..."
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
# Stamp in the source dir so `raven-update` can rebuild from here later.
go build -ldflags "-X main.version=$VERSION -X main.sourceDir=$SRCDIR" -o "$BINARY" .

# Choose a writable install directory, falling back to sudo then ~/.local/bin.
use_sudo=0
if mkdir -p "$BINDIR" 2>/dev/null && [ -w "$BINDIR" ]; then
	target="$BINDIR"
elif command -v sudo >/dev/null 2>&1; then
	use_sudo=1
	target="$BINDIR"
else
	target="$HOME/.local/bin"
	echo "No write access to $BINDIR and no sudo; using $target"
	mkdir -p "$target"
fi

if [ "$use_sudo" = "1" ]; then
	echo "Installing to $target (requires sudo)..."
	sudo mkdir -p "$target"
	sudo install -m 0755 "$BINARY" "$target/$BINARY"
else
	install -m 0755 "$BINARY" "$target/$BINARY"
fi

echo "Installed: $target/$BINARY (RavenShell $VERSION)"

# Warn if the install directory is not on PATH.
case ":$PATH:" in
	*":$target:"*) ;;
	*)
		echo
		echo "Note: $target is not on your PATH. Add this to your shell profile:"
		echo "  export PATH=\"$target:\$PATH\""
		;;
esac

echo
echo "Done. Start it with: $BINARY"
echo "To make RavenShell your default login shell:"
echo "  make register-shell   # add it to /etc/shells (needs sudo)"
echo "  make set-default      # chsh to RavenShell"

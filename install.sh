#!/bin/sh
# install.sh: build pvault from this clone and run its setup.
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
  echo "pvault needs macOS" >&2
  exit 1
fi

if ! command -v brew >/dev/null 2>&1; then
  echo "Homebrew is required: https://brew.sh" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  brew install go
  export PVAULT_INSTALLED_GO=1
fi

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS="-X github.com/atavares-klaviyo/pass-vault/internal/cli.Version=$VERSION"

mkdir -p "$HOME/.local/bin"
go build -ldflags "$LDFLAGS" -o "$HOME/.local/bin/pvault" ./cmd/pvault

case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) echo "Add $HOME/.local/bin to your PATH" ;;
esac

exec "$HOME/.local/bin/pvault" setup --source "$(pwd -P)" "$@"

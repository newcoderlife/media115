#!/bin/sh
set -e

REPO="newcoderlife/media115"
INSTALL_DIR="/usr/local/bin"

# Allow overrides
VERSION="${VERSION:-}"
INSTALL="${INSTALL:-$INSTALL_DIR}"

detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
  esac
}

get_latest_version() {
  if [ -n "$VERSION" ]; then
    return
  fi
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
  if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version" >&2
    exit 1
  fi
}

download_binary() {
  name=$1
  url="https://github.com/$REPO/releases/download/$VERSION/${name}-${OS}-${ARCH}"
  echo "Downloading $name $VERSION ($OS/$ARCH)..."
  curl -fsSL -o "$TMPDIR/$name" "$url"
  chmod +x "$TMPDIR/$name"
}

install_binaries() {
  if [ -w "$INSTALL" ]; then
    mv "$TMPDIR/cloud115" "$INSTALL/cloud115"
    mv "$TMPDIR/media115" "$INSTALL/media115"
  else
    echo "Need sudo to install to $INSTALL"
    sudo mv "$TMPDIR/cloud115" "$INSTALL/cloud115"
    sudo mv "$TMPDIR/media115" "$INSTALL/media115"
  fi
}

main() {
  detect_platform
  get_latest_version

  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT

  download_binary "cloud115"
  download_binary "media115"

  mkdir -p "$INSTALL" 2>/dev/null || true
  install_binaries

  echo ""
  echo "Installed to $INSTALL:"
  echo "  cloud115 $VERSION"
  echo "  media115 $VERSION"
}

main

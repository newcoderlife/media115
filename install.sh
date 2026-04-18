#!/bin/sh
set -e

REPO="newcoderlife/media115"
BRANCH="master"
RAW_BASE="https://raw.githubusercontent.com/$REPO/$BRANCH"
INSTALL_DIR="/usr/local/bin"
SKILLS="cloud115-auth cloud115-dedup cloud115-doctor cloud115-sync media115-organize media115-scan media115-scrape media115-scrape-fix media115-subscribe media115-wishlist"

VERSION="${VERSION:-}"
INSTALL="${INSTALL:-$INSTALL_DIR}"

# ── Binary install ────────────────────────────────────────────────────────────

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
  if [ -n "$VERSION" ]; then return; fi
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
  if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version" >&2; exit 1
  fi
}

install_binaries() {
  detect_platform
  get_latest_version

  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT

  for name in cloud115 media115; do
    url="https://github.com/$REPO/releases/download/$VERSION/${name}-${OS}-${ARCH}"
    echo "Downloading $name $VERSION ($OS/$ARCH)..."
    curl -fsSL -o "$TMPDIR/$name" "$url"
    chmod +x "$TMPDIR/$name"
  done

  mkdir -p "$INSTALL" 2>/dev/null || true
  if [ -w "$INSTALL" ]; then
    mv "$TMPDIR/cloud115" "$INSTALL/cloud115"
    mv "$TMPDIR/media115" "$INSTALL/media115"
  else
    echo "Need sudo to install to $INSTALL"
    sudo mv "$TMPDIR/cloud115" "$INSTALL/cloud115"
    sudo mv "$TMPDIR/media115" "$INSTALL/media115"
  fi

  echo ""
  echo "Installed to $INSTALL:"
  echo "  cloud115 $VERSION"
  echo "  media115 $VERSION"
}

# ── Skill install ─────────────────────────────────────────────────────────────

install_skills() {
  dest=$1
  echo "Installing skills → $dest"
  for skill in $SKILLS; do
    mkdir -p "$dest/$skill"
    curl -fsSL "$RAW_BASE/skills/$skill/SKILL.md" -o "$dest/$skill/SKILL.md"
  done
  echo "  $(echo $SKILLS | wc -w | tr -d ' ') skills installed"
}

# ── Main ──────────────────────────────────────────────────────────────────────

install_binaries

install_skills "$HOME/.agents/skills"
if [ -d "$HOME/.claude" ]; then
  install_skills "$HOME/.claude/skills"
fi
if [ -d "$HOME/.kiro" ]; then
  install_skills "$HOME/.kiro/skills"
fi

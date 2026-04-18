#!/bin/sh
set -ex

REPO="newcoderlife/media115"
BRANCH="master"
RAW_BASE="https://raw.githubusercontent.com/$REPO/$BRANCH"
INSTALL_DIR="/usr/local/bin"
SKILLS="auth dedup doctor organize scan scrape scrape-fix sync"

VERSION="${VERSION:-}"
INSTALL="${INSTALL:-$INSTALL_DIR}"
AGENT="${AGENT:-all}"

usage() {
  echo "Install media115 CLI binaries and agent skills"
  echo ""
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  --agent AGENT       Target agent: claude, agents (Cursor/Codex/OpenCode), or all (default: all)"
  echo "  -h, --help          Show this help"
  echo ""
  echo "Examples:"
  echo "  $0                              # Install binaries + skills for all agents"
  echo "  $0 --agent claude               # Install binaries + skills for Claude Code"
  echo "  $0 --agent agents               # Install binaries + skills for Cursor/Codex/OpenCode"
  echo ""
  echo "Environment:"
  echo "  VERSION=v0.1.0      Pin binary version (default: latest)"
  echo "  INSTALL=~/.local/bin  Install path (default: /usr/local/bin)"
  echo "  AGENT=claude        Target agent (default: all)"
}

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
  mkdir -p "$dest"
  curl -fsSL "$RAW_BASE/skills/SKILL.md" -o "$dest/SKILL.md"
  for skill in $SKILLS; do
    mkdir -p "$dest/$skill"
    curl -fsSL "$RAW_BASE/skills/$skill/SKILL.md" -o "$dest/$skill/SKILL.md"
  done
  echo "  $(echo $SKILLS | wc -w | tr -d ' ') skills installed"
}

# ── Main ──────────────────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
  case "$1" in
    --agent)  AGENT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

install_binaries

case "$AGENT" in
  claude) install_skills "$HOME/.claude/skills/media115" ;;
  agents) install_skills "$HOME/.agents/skills/media115" ;;
  all)
    install_skills "$HOME/.claude/skills/media115"
    install_skills "$HOME/.agents/skills/media115"
    ;;
  *) echo "Unknown agent: $AGENT (use claude|agents|all)" >&2; exit 1 ;;
esac

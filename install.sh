#!/bin/sh

set -ex

REPO="newcoderlife/media115"
SKILLS="cloud115-auth cloud115-dedup cloud115-doctor cloud115-sync media115-organize media115-scan media115-scrape media115-scrape-fix media115-subscribe media115-wishlist"

install_skills() {
  for skill in $SKILLS; do
    mkdir -p "$1/$skill"
    curl -fsSL "https://raw.githubusercontent.com/$REPO/master/skills/$skill/SKILL.md" -o "$1/$skill/SKILL.md"
  done
}

# ── Binaries ──────────────────────────────────────────────────────────────────

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS"   in linux) ;; darwin) ;; *) echo "Unsupported OS: $OS" >&2; exit 1 ;; esac
case "$ARCH" in x86_64|amd64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; esac

VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')

TMPDIR=$(mktemp -d); trap 'rm -rf "$TMPDIR"' EXIT
for name in cloud115 media115; do
  curl -fsSL -o "$TMPDIR/$name" "https://github.com/$REPO/releases/download/$VERSION/${name}-${OS}-${ARCH}"
  chmod +x "$TMPDIR/$name"
done

DEST="/usr/local/bin"; mkdir -p "$DEST" 2>/dev/null || true
MV="mv"; [ ! -w "$DEST" ] && MV="sudo mv"
for name in cloud115 media115; do $MV "$TMPDIR/$name" "$DEST/$name"; done

# ── Config ────────────────────────────────────────────────────────────────────

case "$OS" in darwin) CFG="$HOME/Library/Application Support/media115" ;; *) CFG="${XDG_CONFIG_HOME:-$HOME/.config}/media115" ;; esac
mkdir -p "$CFG"
[ ! -f "$CFG/config.toml" ] && curl -fsSL "https://raw.githubusercontent.com/$REPO/master/config.example.toml" -o "$CFG/config.toml"

# ── Skills ────────────────────────────────────────────────────────────────────

install_skills "$HOME/.agents/skills"
[ -d "$HOME/.claude" ] && install_skills "$HOME/.claude/skills"
[ -d "$HOME/.kiro" ]   && install_skills "$HOME/.kiro/skills"

echo "Done: cloud115 & media115 $VERSION installed"

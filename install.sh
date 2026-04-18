#!/bin/sh

set -ex

REPO="newcoderlife/media115"
SKILLS="cloud115-auth cloud115-dedup cloud115-doctor cloud115-sync media115-organize media115-scan media115-scrape media115-scrape-fix media115-subscribe media115-wishlist"

VERSION="${VERSION:-}"
INSTALL="${INSTALL:-/usr/local/bin}"

install_skills() {
  dest=$1
  echo "Installing skills → $dest"
  for skill in $SKILLS; do
    mkdir -p "$dest/$skill"
    curl -fsSL "https://raw.githubusercontent.com/$REPO/master/skills/$skill/SKILL.md" -o "$dest/$skill/SKILL.md"
  done
  echo "  $(echo $SKILLS | wc -w | tr -d ' ') skills installed"
}

# ── Install binaries ──────────────────────────────────────────────────────────

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS"   in linux) ;; darwin) ;; *) echo "Unsupported OS: $OS" >&2; exit 1 ;; esac
case "$ARCH" in x86_64|amd64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; esac

if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
  [ -n "$VERSION" ] || { echo "Failed to fetch latest version" >&2; exit 1; }
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

for name in cloud115 media115; do
  curl -fsSL -o "$TMPDIR/$name" "https://github.com/$REPO/releases/download/$VERSION/${name}-${OS}-${ARCH}"
  chmod +x "$TMPDIR/$name"
done

mkdir -p "$INSTALL" 2>/dev/null || true
if [ -w "$INSTALL" ]; then
  mv "$TMPDIR/cloud115" "$INSTALL/cloud115"
  mv "$TMPDIR/media115" "$INSTALL/media115"
else
  sudo mv "$TMPDIR/cloud115" "$INSTALL/cloud115"
  sudo mv "$TMPDIR/media115" "$INSTALL/media115"
fi

echo "Installed cloud115 & media115 $VERSION → $INSTALL"

# ── Install skills ────────────────────────────────────────────────────────────

install_skills "$HOME/.agents/skills"
[ -d "$HOME/.claude" ] && install_skills "$HOME/.claude/skills"
[ -d "$HOME/.kiro" ]   && install_skills "$HOME/.kiro/skills"

#!/bin/sh
set -e

REPO="newcoderlife/media115"
SKILL_NAME="media115"
BRANCH="master"
RAW_BASE="https://raw.githubusercontent.com/$REPO/$BRANCH"

# Claude Code has its own path; all others use ~/.agents/ standard
CLAUDE_DIR="$HOME/.claude/skills/$SKILL_NAME"
AGENTS_DIR="$HOME/.agents/skills/$SKILL_NAME"

SKILLS="auth dedup doctor organize scan scrape scrape-fix sync"

usage() {
  echo "Install media115 agent skills"
  echo ""
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  -a, --agent AGENT   Install for specific agent (claude|agents|all)"
  echo "                      claude  → ~/.claude/skills/"
  echo "                      agents  → ~/.agents/skills/ (Cursor, Codex, OpenCode)"
  echo "  -h, --help          Show this help"
  echo ""
  echo "Examples:"
  echo "  $0                  # Install for both locations"
  echo "  $0 -a claude        # Claude Code only"
  echo "  $0 -a agents        # ~/.agents/ only (Cursor/Codex/OpenCode)"
}

download_skills() {
  dest=$1
  echo "Installing → $dest"
  mkdir -p "$dest"

  curl -fsSL "$RAW_BASE/skills/SKILL.md" -o "$dest/SKILL.md"
  for skill in $SKILLS; do
    mkdir -p "$dest/$skill"
    curl -fsSL "$RAW_BASE/skills/$skill/SKILL.md" -o "$dest/$skill/SKILL.md"
  done

  echo "  $(echo $SKILLS | wc -w | tr -d ' ') skills installed"
}

# Parse args
TARGET="all"
while [ $# -gt 0 ]; do
  case "$1" in
    -a|--agent) TARGET="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

echo "media115 skills installer"
echo ""

case "$TARGET" in
  claude) download_skills "$CLAUDE_DIR" ;;
  agents) download_skills "$AGENTS_DIR" ;;
  all)
    download_skills "$CLAUDE_DIR"
    download_skills "$AGENTS_DIR"
    ;;
  *) echo "Unknown target: $TARGET" >&2; usage; exit 1 ;;
esac

echo ""
echo "Done. Skills: /$SKILL_NAME:{auth,sync,scan,scrape,organize,...}"

#!/bin/sh
set -e

REPO="newcoderlife/media115"
SKILL_NAME="media115"
BRANCH="master"
RAW_BASE="https://raw.githubusercontent.com/$REPO/$BRANCH"

# Agent config directories (global install)
CLAUDE_DIR="$HOME/.claude/skills/$SKILL_NAME"
CURSOR_DIR="$HOME/.cursor/skills/$SKILL_NAME"
OPENCODE_DIR="$HOME/.config/opencode/skills/$SKILL_NAME"
CODEX_DIR="$HOME/.codex/skills/$SKILL_NAME"

SKILLS="auth dedup doctor organize scan scrape scrape-fix sync"

usage() {
  echo "Install media115 agent skills"
  echo ""
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  -a, --agent AGENT   Install for specific agent (claude|cursor|opencode|codex|all)"
  echo "  -h, --help          Show this help"
  echo ""
  echo "Examples:"
  echo "  $0                  # Install for all detected agents"
  echo "  $0 -a claude        # Install for Claude Code only"
  echo "  $0 -a cursor        # Install for Cursor only"
}

detect_agents() {
  agents=""
  [ -d "$HOME/.claude" ] && agents="$agents claude"
  [ -d "$HOME/.cursor" ] && agents="$agents cursor"
  [ -d "$HOME/.config/opencode" ] && agents="$agents opencode"
  [ -d "$HOME/.codex" ] && agents="$agents codex"
  # If none detected, install for all
  if [ -z "$agents" ]; then
    agents="claude cursor opencode codex"
  fi
  echo "$agents"
}

download_skill() {
  skill=$1
  dest_dir=$2
  mkdir -p "$dest_dir/$skill"
  curl -fsSL "$RAW_BASE/skills/$skill/SKILL.md" -o "$dest_dir/$skill/SKILL.md"
}

download_root_skill() {
  dest_dir=$1
  curl -fsSL "$RAW_BASE/skills/SKILL.md" -o "$dest_dir/SKILL.md"
}

install_for_agent() {
  agent=$1
  case "$agent" in
    claude)   dest="$CLAUDE_DIR" ;;
    cursor)   dest="$CURSOR_DIR" ;;
    opencode) dest="$OPENCODE_DIR" ;;
    codex)    dest="$CODEX_DIR" ;;
    *) echo "Unknown agent: $agent" >&2; return 1 ;;
  esac

  echo "Installing for $agent → $dest"
  mkdir -p "$dest"

  download_root_skill "$dest"
  for skill in $SKILLS; do
    download_skill "$skill" "$dest"
  done

  echo "  Installed $(echo $SKILLS | wc -w | tr -d ' ') skills"
}

# Parse args
AGENT=""
while [ $# -gt 0 ]; do
  case "$1" in
    -a|--agent) AGENT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [ "$AGENT" = "all" ] || [ -z "$AGENT" ]; then
  if [ -z "$AGENT" ]; then
    agents=$(detect_agents)
  else
    agents="claude cursor opencode codex"
  fi
else
  agents="$AGENT"
fi

echo "media115 skills installer"
echo ""

for agent in $agents; do
  install_for_agent "$agent"
done

echo ""
echo "Done. Skills available:"
for skill in $SKILLS; do
  echo "  /$SKILL_NAME:$skill"
done

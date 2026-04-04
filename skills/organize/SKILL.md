---
name: organize
description: Rename and move 115 files to Jellyfin standard naming
version: 1.0
---

Organize media files on 115 to Jellyfin standard: `中文名 (年份)/中文名 (年份).ext`

## Input
$ARGUMENTS — category: `电影`, `AV`, `剧目`, or `all`

## Steps

### 1. Ensure scrape is done
file_map cache must exist (created by batch-scrape). If not, tell user to run `/scrape` first.

### 2. Dry-run
```bash
.venv/bin/python -m media115.cli organize $CATEGORY
```
Show the plan table to user. Ask for confirmation.

### 3. Execute (after user confirms)
```bash
.venv/bin/python -m media115.cli organize $CATEGORY --execute
```
This does: mkdir target folder → move file → rename file. Never renames existing dirs.

### 4. Clean up empty dirs
After organize, old empty directories may remain. Tell the user they can delete them manually from 115 web, or note them for later cleanup.

### 5. Verify
Re-export tree to confirm:
```bash
.venv/bin/python -m media115.cli export-tree
.venv/bin/python -m media115.cli ls /影音/$CATEGORY
```

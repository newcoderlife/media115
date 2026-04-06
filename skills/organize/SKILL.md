---
name: organize
description: Rename, move, upload NFO, and cleanup — the main operation
version: 3.0
---

The single operation that does everything: move files to correct dirs, rename to standard format, upload NFO/poster, and clean up old dirs.

## Input
$ARGUMENTS — category: `电影`, `AV`, or `剧目`.

## Preconditions
- `/scrape` must have been run (creates file_map cache with correct names).
- Tree cache must be fresh. If you just ran organize for another category, run `/sync` first.

## Steps

### 1. Dry-run
```bash
media115 organize $CATEGORY
```
Show the plan table to user. Check for:
- Target conflicts (two files → same name) — resolve before executing
- Unexpected matches — verify titles look correct
- "0 to rename" when anomalies exist — means you forgot `batch-scrape --force`

### 2. Execute (after user confirms)
```bash
media115 organize $CATEGORY --execute
```

This does 6 phases automatically:
1. Resolve file IDs on 115
2. Create target directories
3. Move files to target dirs
4. Rename files to standard format
5. Upload NFO/poster (named to match video)
6. Delete old source directories (if no video files remain)

### 3. Refresh tree cache
organize invalidates the tree cache. **Always refresh after execute:**
```bash
media115 export-tree /影音
```

### 4. Verify
```bash
media115 scan-tree $CATEGORY
```
Check: 0 anomalies, all files have NFO.

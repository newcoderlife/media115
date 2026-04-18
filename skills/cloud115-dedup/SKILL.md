---
name: cloud115-dedup
description: Clean duplicate files (same-name copies created by 115's upload behavior)
metadata:
  version: "2.0"
---

Clean duplicate files in a 115 directory. Groups files by filename and keeps one copy, deletes the rest.

## When to use

- `scan` shows "Duplicate NFO" anomalies
- 115 web upload created same-name copies (e.g., `SONE-001.nfo` and `SONE-001(1).nfo` in the same dir)
- After bulk organize or upload operations that may have created duplicates

## Step 1: Dry-run (always do this first)

```bash
cloud115 dedup "/影音/电影"
cloud115 dedup "/影音/剧目"
cloud115 dedup "/影音/AV"
```

Expected output shows which subdirectories have duplicate files and how many would be deleted.

If output says "No duplicates found" — nothing to do.

## Step 2: Execute

```bash
cloud115 dedup "/影音/剧目" --execute
```

This:
- Deletes duplicates by fid (file ID on 115)
- Keeps the first copy, deletes the rest
- Batches deletes in groups of 50
- Refreshes affected directories in SQLite cache after completion

## Step 3: Verify

```bash
media115 scan $CATEGORY
```

The "Duplicate NFO" anomalies should be gone.

## Error handling

If dedup reports an error deleting a specific fid:
- The file may have already been deleted by another operation
- Check with `cloud115 ls /影音/.../目录名/` to confirm current state
- Re-run dry-run to see if duplicates still exist

---
name: media115-organize
description: Rename, move, upload NFO, and cleanup — the main operation
metadata:
  version: "5.0"
---

The single operation that does everything: move files to correct dirs, rename to standard format, upload NFO/poster, clean up old dirs, and verify.

## Input

$ARGUMENTS — category: `电影`, `AV`, or `剧目`.

## Preconditions

- `/scrape` must have been run (creates scrape cache with correct names).
- SQLite cache must be fresh (run `/sync` if you haven't yet today).

## Step 1: Dry-run

```bash
media115 organize $CATEGORY
```

Expected output:
```
Organize plan for '电影': 5 to rename, 120 unchanged
...plan table...
```

Show the plan table to the user. Check for:
- Target conflicts (two files → same name) — resolve before executing
- Unexpected titles — verify they look correct
- "0 to rename" when anomalies exist — means you forgot `media115 scrape --force`

## Step 2: Execute (after user confirms)

```bash
media115 organize $CATEGORY --execute
```

This does 6 phases automatically:
1. Resolve file IDs on 115
2. Create target directories
3. Move files to target dirs
4. Rename files to standard format
5. Upload NFO/poster (named to match video)
6. Verify and refresh tree cache

At the end you will see:
```
✓ N 个文件验证通过
```

If it says "0 files verified" but you expected more, something went wrong — report to user.

## Step 3: Verify

```bash
media115 scan $CATEGORY
```

Expected: 0 anomalies, all files show `skip` (has NFO).

If anomalies remain after organize, check:
1. Was `media115 scrape --force` run if there were non-standard names?
2. Did organize actually move/rename the files? Check the phase output.

## Note: Automatic cleanup

`--execute` automatically cleans up old source directories that no longer contain video files after the move. No separate `--cleanup` flag is needed.

## Note: No need to re-run sync after organize

Organize's verify phase refreshes the tree cache automatically. You only need to re-run `/sync` if you are starting work on a different category or the user made manual changes on 115.

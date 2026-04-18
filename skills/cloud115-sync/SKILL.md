---
name: cloud115-sync
description: Refresh local SQLite cache from 115 cloud. Required before scan/scrape.
metadata:
  version: "5.0"
---

Refresh the local cache from 115 cloud. This populates the SQLite `tree_entry` table, which is the source of truth for `scan`, `scrape`, and `organize`.

## Preconditions

- 115 must be logged in. Run `/auth` first if needed.

## Step 1: Export tree + populate SQLite cache

```bash
cloud115 sync /影音
```

This does:
- Exports the 115 directory tree (2-3 API calls total)
- Saves a human-readable backup to `tree_cache.txt`
- Parses everything into the SQLite `tree_entry` table

Expected output:
```
Exported 2158 entries to tree_cache.txt
Parsed 2158 entries into SQLite (1452 video, 706 NFO)
```

## Step 2: Verify

```bash
cloud115 cache status
```

Check that `tree_entry` shows a reasonable number of entries (e.g., `✓ 2158 条目 (1452 视频, 706 NFO)`).

## Step 3 (Optional): Warm directory listing cache

Only needed before `organize` on large libraries to avoid API calls during execution.

```bash
cloud115 sync /影音 --deep --depth 2
```

Pre-fetches directory listings into SQLite. Each directory = 2 API calls. Shows progress. `--depth 2` means 2 levels below the path you specified.

## When to re-run sync

- Before starting work on any category
- If the user has manually moved/renamed files on 115 outside of this tool
- If `scan` shows stale or unexpected results

You do NOT need to re-run sync after `organize --execute`. Organize's verify phase refreshes the tree cache automatically.

## Error handling

115 经常返回临时 EOF（限流）。遇到时：
1. 等待 5-10 秒后重试，至少重试 3 次
2. 不要因为 sync 失败就跳过后续验证步骤
3. 如果持续失败，告诉用户可能需要等一段时间再试

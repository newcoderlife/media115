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
media115 sync /影音
```

### 3.5 补传缺失的 NFO（当 organize 输出 "0 to rename" 时）

如果 organize 输出 "0 to rename" 或 "Nothing to rename"，文件已命名正确但可能缺少 NFO/海报。

```bash
# 查看 scrape_output 中是否有待上传的内容
ls ~/.cache/media115/scrape_output/$CATEGORY/
```

对于每个有 scrape_output 但 115 上对应目录缺少 NFO 的条目：

```bash
# 检查远程目录
media115 ls /影音/$CATEGORY/目录名/

# 上传 scrape_output 中的所有 NFO 和图片
# 电影：目录名.nfo + poster.jpg
# 剧目：tvshow.nfo + 各集 NFO（如 迷宫饭 S01E01.nfo）+ poster.jpg
# AV：番号.nfo + poster.jpg
```

逐个上传 scrape_output 目录下的所有 `.nfo`、`.jpg`、`.png` 文件：
```bash
media115 put ~/.cache/media115/scrape_output/$CATEGORY/源目录名/文件.nfo /影音/$CATEGORY/目录名/
media115 put ~/.cache/media115/scrape_output/$CATEGORY/源目录名/poster.jpg /影音/$CATEGORY/目录名/
```

**注意：** NFO 文件名必须和视频文件名一致。对于剧目，每集有独立 NFO（如 `迷宫饭 S01E01.nfo`），另有一个 `tvshow.nfo` 描述整部剧。上传时保持原文件名不变即可。

### 4. Verify
```bash
media115 scan-tree $CATEGORY
```
Check: 0 anomalies, all files have NFO.

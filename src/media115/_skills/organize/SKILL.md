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

### 3.5 补传缺失的 NFO（当 organize 输出 "0 to rename" 时）

如果 organize 输出 "0 to rename" 或 "Nothing to rename"，说明文件已命名正确但可能缺少 NFO/海报。检查 scrape_output 并手动上传：

```bash
# 查看 scrape_output 中是否有待上传的 NFO
ls ~/.cache/media115/scrape_output/$CATEGORY/
```

对于每个有 NFO 但 115 上对应目录缺少 NFO 的条目：

```bash
# 检查远程目录是否有 NFO
media115 ls /影音/$CATEGORY/目录名/

# 如果没有 NFO，上传
media115 put ~/.cache/media115/scrape_output/$CATEGORY/影音_$CATEGORY_目录名/目录名.nfo /影音/$CATEGORY/目录名/
media115 put ~/.cache/media115/scrape_output/$CATEGORY/影音_$CATEGORY_目录名/poster.jpg /影音/$CATEGORY/目录名/
```

注意 NFO 文件名必须和视频文件名一致（如 `满江红 (2023).nfo` 对应 `满江红 (2023).mkv`）。

### 4. Verify
```bash
media115 scan-tree $CATEGORY
```
Check: 0 anomalies, all files have NFO.

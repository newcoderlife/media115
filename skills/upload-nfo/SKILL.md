---
name: upload-nfo
description: Upload NFO/poster for already-organized files (use /organize instead in most cases)
version: 1.1
---

Upload locally generated NFO and poster files to 115 directories.

**You usually don't need this.** `/organize --execute` already uploads NFO/poster as part of its pipeline. This skill is only for files that organize SKIPPED — files already correctly named on 115 that just need NFO added.

## When to use
- Files were manually organized on 115 (correct names already)
- batch-scrape generated NFOs locally
- NFOs need to be pushed to 115 without renaming anything

## Steps

### 1. Upload
```bash
.venv/bin/python -m media115.cli upload-nfo $CATEGORY
```

This:
- Lists 115 dirs under the category
- Checks tree cache for dirs that already have NFO (skips them)
- Uploads .nfo + poster for dirs that don't have NFO yet

### 2. Refresh and verify
```bash
.venv/bin/python -m media115.cli export-tree /影音
.venv/bin/python -m media115.cli scan-tree $CATEGORY
```

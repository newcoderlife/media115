---
name: scrape
description: Scrape metadata for media files on 115 cloud
version: 2.0
---

Scrape metadata (NFO + artwork) for media files in the 115 影音 directory.

## Input
$ARGUMENTS — category to scrape: `电影`, `AV`, `剧目`, or `all`

## Steps

### 1. Ensure login
Run `/auth` first (or check inline):
```bash
.venv/bin/python -m media115.cli auth --check
```

### 2. Export directory tree (if not recent)
```bash
.venv/bin/python -m media115.cli export-tree
```

### 3. Scan to see what needs scraping
```bash
.venv/bin/python -m media115.cli scan-tree $CATEGORY
```
Report the summary to the user.

### 4. Batch scrape (CLI handles most files)
```bash
.venv/bin/python -m media115.cli batch-scrape $CATEGORY
```
This uses regex analyzer + TMDB/jav321/javfree with caching.

### 5. Handle failures (YOUR job as agent)
Check batch-scrape output for `not_found` or `error` files.
For each failed file:
1. Look at the filename, use YOUR judgment to determine title/type/year
2. Search: `.venv/bin/python -m media115.cli scrape "your search query"`
3. If multiple results, pick the best match
4. Save as regression case via `/scrape-fix`

### 6. Report
Output summary table: how many scraped, how many failed, how many agent-fixed.

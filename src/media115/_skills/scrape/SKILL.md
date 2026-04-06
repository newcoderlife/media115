---
name: scrape
description: Batch scrape metadata for a category, agent handles failures
version: 4.0
---

Scrape metadata (NFO + poster) for media files on 115 cloud.

## Input
$ARGUMENTS — category: `电影`, `AV`, or `剧目`. Ask the user if not provided.

## Preconditions
- 115 must be logged in (`/auth`)
- Tree cache must exist. If not, run `/sync` first.

## Steps

### 1. Check if --force is needed
Run `/scan` first. If it shows **"Non-standard name" anomalies**, you MUST use `--force`:
```bash
media115 scan-tree $CATEGORY
```

**Why:** Without `--force`, batch-scrape skips files that already have NFOs on 115. Those files get NO file_map entry, so organize will silently skip them later. This is the #1 source of "organize did nothing" bugs.

### 2. Batch scrape
```bash
# Normal: only scrape files without NFO
media115 batch-scrape $CATEGORY

# If anomalies detected: re-scrape ALL files including those with NFO
media115 batch-scrape $CATEGORY --force
```

### 3. Handle failures (YOUR job as agent)
Check batch-scrape output for `not_found` or `error` files.

For each failed file:
1. Look at the filename, use YOUR judgment to determine title/type/year
2. Search: `media115 scrape "your search query"`
3. Try alternate queries, sources (`--source bangumi` for anime)
4. If multiple results, pick the best match
5. Save as regression case via `/scrape-fix`

### 4. Report
Output summary: how many scraped, how many failed, how many agent-fixed.
Ask user if they want to proceed to `/organize`.

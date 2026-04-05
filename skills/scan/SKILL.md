---
name: scan
description: Show what needs scraping and detect anomalies from cached tree (zero API calls)
version: 2.0
---

Analyze the cached directory tree. Reports what needs scraping AND detects anomalies. Zero API calls.

## Input
$ARGUMENTS — category: `电影`, `AV`, `剧目`. If not provided, run all three.

## Preconditions
- Tree cache must exist. If not, tell user to run `/sync-tree` first.

## Steps

### 1. Scan
```bash
.venv/bin/python -m media115.cli scan-tree $CATEGORY
```

If no category given, run for each:
```bash
.venv/bin/python -m media115.cli scan-tree 电影
.venv/bin/python -m media115.cli scan-tree AV
.venv/bin/python -m media115.cli scan-tree 剧目
```

### 2. Report
Show the summary table to the user:
- Total files per category
- How many need scraping
- How many already have NFO

### 3. Check anomalies
scan-tree automatically detects three types of anomalies:

| Type | Meaning | Fix |
|------|---------|-----|
| **Non-standard name** | Has NFO but dir name doesn't match `中文名 (年份)` or `番号` | Run `/scrape --force` then `/organize` |
| **Duplicate NFO** | Multiple NFO files in one dir (e.g. `X.nfo` + `X(1).nfo`) | Delete the `(1)` duplicates via CLI |
| **Residual dir** | Has NFO/images but no video file | Delete the empty dir via CLI |

If anomalies are found, report them to the user and suggest fixes.

**Non-standard naming** is the most common issue — it means Codex or a previous agent scraped the file but didn't rename the directory. Fix with:
```bash
.venv/bin/python -m media115.cli batch-scrape $CATEGORY --force
.venv/bin/python -m media115.cli organize $CATEGORY
```

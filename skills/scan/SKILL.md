---
name: scan
description: Show what needs scraping and detect anomalies from SQLite cache (zero API calls)
version: 4.0
---

Analyze the SQLite tree cache. Reports what needs scraping AND detects anomalies. Zero API calls.

## Preconditions

- SQLite cache must be populated. If not, run `/sync` first.

## Step 1: Scan

```bash
media115 scan 电影
media115 scan AV
media115 scan 剧目
```

Run all three unless the user specified a category.

## Step 2: Read the output

Each row in the table has an `Action` column:

| Action | Meaning |
|--------|---------|
| `scrape` | No NFO found — needs scraping |
| `skip` | Already has NFO — will be skipped by scrape |
| `unrecognized` | Filename cannot be parsed — needs manual handling |

At the bottom:
```
Summary: 184 files — 1 to scrape, 183 skip (has NFO), 0 unrecognized, 0 known cases

No API calls were made (tree cache only).
```

Report these numbers to the user.

## Step 3: Check for anomalies

scan automatically detects three types of anomalies:

| Type | Meaning | Fix |
|------|---------|-----|
| **Non-standard name** | Has NFO but directory name is wrong format | Run `media115 scrape --force` then `media115 organize --execute` |
| **Duplicate NFO** | Multiple NFO files in one dir (e.g., `X.nfo` + `X(1).nfo`) | Run `/dedup` on that directory |
| **Residual dir** | Has NFO/images but no video file | Delete the directory: `cloud115 rm -r /影音/.../目录名` |

If anomalies are found, report them and suggest the fix above.

**Non-standard naming is the most common issue.** It means a previous operation scraped the file but did not rename the directory. Fix with:

```bash
media115 scrape $CATEGORY --force
media115 organize $CATEGORY --execute
```

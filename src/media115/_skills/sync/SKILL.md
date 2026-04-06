---
name: sync
description: Export 115 directory tree to local cache (2-3 API calls)
version: 2.0
---

Refresh the local directory tree cache from 115 cloud. This is a prerequisite for `/scan` and `/scrape`.

## Input
$ARGUMENTS — 115 path to export, e.g. `/影音`. Ask the user if not provided.

## Preconditions
- 115 must be logged in. Run `/auth` first if needed.

## Steps

### 1. Export tree
```bash
media115 sync $PATH
```
This makes 2-3 API calls (create export task → download result → delete temp file on 115).

### 2. Report
Tell the user the tree cache is updated. Mention file count if shown in output.

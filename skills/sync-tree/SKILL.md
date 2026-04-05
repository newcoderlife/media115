---
name: sync-tree
description: Export 115 directory tree to local cache (2-3 API calls)
version: 1.0
---

Refresh the local directory tree cache from 115 cloud. This is a prerequisite for `/scan` and `/scrape`.

## Input
$ARGUMENTS — 115 path to export, e.g. `/影音`. Ask the user if not provided.

## Preconditions
- 115 must be logged in. Run `/auth` first if needed.

## Steps

### 1. Export tree
```bash
.venv/bin/python -m media115.cli export-tree $PATH
```
This makes 2-3 API calls (create export task → download result → delete temp file on 115).

### 2. Report
Tell the user the tree cache is updated. Mention file count if shown in output.

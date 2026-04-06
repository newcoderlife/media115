---
name: scrape-fix
description: Correct a scraping result and save as regression test case
version: 2.1
---

Correct a wrong scraping result, re-scrape with correct info, and save as regression test.

This skill is used in two scenarios:
1. User says "that result is wrong" — triggered by user feedback
2. Agent resolves a failed file during `/scrape` — triggered by agent

## Input
$ARGUMENTS — correction instruction, e.g. "xxx.mkv 是电影《满江红》" or "The Running Man is movie The Running Man (2025)"

## Steps

### 1. Parse correction
Extract: filename, correct type (movie/tv/anime/av), correct title, source, source_id (if given).

### 2. Search for correct match
```bash
media115 scrape "correct title"
media115 scrape "correct title" --source bangumi
```

### 3. Save regression case
```bash
.venv/bin/python -c "
from media115.scraper.analyzer import add_case, AnalysisResult
from pathlib import Path
r = AnalysisResult(
    filename='FILENAME', media_type='TYPE', title='TITLE',
    year=YEAR, source='SOURCE', source_id='SOURCE_ID',
)
add_case(Path('tests/scrape_cases.json'), r)
"
```

### 4. Run regression tests
```bash
.venv/bin/pytest tests/test_scrape_regression.py -v
```

### 5. Report
What was corrected, new case ID, test results (all green?).

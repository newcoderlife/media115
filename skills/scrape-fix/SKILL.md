---
name: scrape-fix
description: Correct a scraping result and save as regression test case
version: 2.0
---

Correct a wrong scraping result, re-scrape with correct info, and save as regression test.

## Input
$ARGUMENTS — correction instruction, e.g. "The Running Man 是电影 The Running Man (2025)"

## Steps

### 1. Parse correction
Extract: filename, correct type (movie/tv/anime/av), correct title, source, source_id (if given).

### 2. Search for correct match
```bash
.venv/bin/python -m media115.cli scrape "correct title"
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
- What was corrected
- New case ID
- Test results (all green?)

If tests fail, diagnose and report to user.

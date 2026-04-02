# 115-media Agent Instructions

You are working on **115-media**, a media library management tool for 115 cloud drive. This file tells you everything you need to operate.

## Setup

Before running any command, ensure the virtual environment exists:

```bash
test -d .venv || (python3 -m venv .venv && .venv/bin/pip install -e .)
```

API keys are in `.env` (not committed). Required: `TMDB_READ_ACCESS_TOKEN`. Optional: `BANGUMI_ACCESS_TOKEN`, `CLOUD_115_*`.

## Project Layout

```
src/media115/
├── cli.py             # CLI entry point (run via: .venv/bin/python -m media115.cli)
├── client.py          # 115 OpenAPI client
├── proxy.py           # Jellyfin reverse proxy + 302 redirect
├── organizer.py       # SHA1 hashing, rapid upload, STRM generation
└── scraper/
    ├── tmdb.py        # TMDB API (movies, TV shows)
    ├── bangumi.py     # Bangumi API (anime)
    ├── javbus.py      # JavBus HTML scraper (AV)
    ├── nfo.py         # NFO generation + parsing (Kodi format)
    ├── artwork.py     # Poster/fanart download
    └── analyzer.py    # Filename analysis + regression case management

tests/
├── scrape_cases.json  # Regression test cases (user corrections saved here)
├── conftest.py        # Loads .env for tests
└── test_*.py          # 93 tests
```

## CLI Reference

```bash
.venv/bin/python -m media115.cli scrape "QUERY"                    # Search TMDB (default)
.venv/bin/python -m media115.cli scrape "QUERY" --source bangumi   # Search Bangumi
.venv/bin/python -m media115.cli scrape "QUERY" --source javbus    # Search JavBus
.venv/bin/python -m media115.cli ls DIR_ID                         # List 115 cloud directory
.venv/bin/python -m media115.cli upload FILE --remote-dir DIR_ID   # Rapid upload to 115
.venv/bin/python -m media115.cli serve --port 9000                 # Start strm-proxy
```

## Running Tests

```bash
.venv/bin/pytest tests/ -v                          # All tests (93)
.venv/bin/pytest tests/test_scrape_regression.py -v  # Regression tests only
```

---

## Workflows

### Workflow: Scrape

Input: a folder path containing media files.

**Step 1 — Scan.** List all video files (`*.mkv`, `*.mp4`, `*.avi`, `*.ts`, `*.rmvb`) recursively. For each file:
- Check if a `.nfo` exists in the same directory. If complete (has `<uniqueid>` + poster exists), mark as "skip".
- If `.nfo` exists but incomplete, extract `<uniqueid>` or `<title>` + `<year>` for targeted lookup.
- If no `.nfo`, analyze the filename to determine type (movie/tv/anime/av), title, year, season, episode.

**Step 2 — Plan.** Output a table:

```
| # | File | NFO | Type | Search Term | Source | Action |
|---|------|-----|------|-------------|--------|--------|
```

Ask user to confirm or correct rows.

**Step 3 — Execute.** For each row needing action, use the Python scraper:

```bash
# Search
.venv/bin/python -c "
from media115.scraper.tmdb import TMDBClient
import os, json
c = TMDBClient(read_access_token=os.environ['TMDB_READ_ACCESS_TOKEN'])
print(json.dumps(c.search_movie('QUERY')[:3], ensure_ascii=False, indent=2))
"

# Generate NFO + download poster
.venv/bin/python -c "
from media115.scraper.tmdb import TMDBClient
from media115.scraper.nfo import generate_movie_nfo
from media115.scraper.artwork import save_poster
from pathlib import Path
import os
c = TMDBClient(read_access_token=os.environ['TMDB_READ_ACCESS_TOKEN'])
d = c.movie_detail(TMDB_ID)
imgs = c.movie_images(TMDB_ID)
meta = {
    'title': d['title'], 'originaltitle': d['original_title'],
    'year': int(d['release_date'][:4]), 'plot': d['overview'],
    'runtime': d['runtime'], 'rating': d['vote_average'],
    'premiered': d['release_date'],
    'genres': [g['name'] for g in d['genres']],
    'uniqueids': {'tmdb': str(d['id']), 'imdb': d.get('imdb_id','')},
}
out = Path('OUTPUT_DIR')
generate_movie_nfo(meta, out / 'movie.nfo')
if imgs.get('posters'): save_poster(imgs['posters'][0]['file_path'], out)
"
```

For Bangumi (anime), use `BangumiClient`. For JavBus (AV), use `parse_detail_page`.

**Step 4 — Report.** Output results table. Ask user if any row is wrong.

**Step 5 — Correct.** If user says a row is wrong:
1. Re-scrape with corrected info
2. Save the correction as a regression case (see Scrape-Fix workflow)
3. Run regression tests

### Workflow: Scrape-Fix

Input: a correction, e.g. `xxx.mkv should be movie "满江红" tmdb:945729`

1. Parse the correction to extract filename, type, title, source, source_id.
2. If no source_id given, search for the title using the CLI.
3. Regenerate NFO + artwork.
4. Save as regression case:

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

5. Run regression tests: `.venv/bin/pytest tests/test_scrape_regression.py -v`
6. Report: what changed, new case added, test results.

### Workflow: Scrape-Test

1. Run: `.venv/bin/pytest tests/test_scrape_regression.py -v`
2. If all pass, report count.
3. If any fail, show case ID, expected vs actual, suggest fix.
4. Optionally run full suite: `.venv/bin/pytest tests/ -v`

## Rules

- **Never** modify video file binaries (only create/modify .nfo, .jpg, .strm)
- **Always** output results as markdown tables
- **Always** save corrections to `tests/scrape_cases.json`
- **Always** run regression tests after corrections
- **Always** verify tests pass before claiming work is done

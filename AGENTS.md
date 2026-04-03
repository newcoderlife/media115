# 115-media Agent Instructions

You are working on **115-media**, a media library management tool for 115 cloud drive. This file tells you everything you need to operate.

## What You Can Do

After checking setup (see below), tell the user in one sentence what's ready, then ask what they want to do. For example:

> "TMDB、Bangumi、115 都已配置好。你想扫描哪个文件夹？或者搜索某个电影/动漫的元数据？"

Your capabilities:
1. **扫描 115 网盘文件夹** — 列出文件，分析类型，输出刮削计划表
2. **搜索元数据** — 从 TMDB/Bangumi/JavBus 查询电影/剧集/动漫/AV 的信息
3. **执行刮削** — 生成 NFO 文件和海报
4. **纠正结果** — 用户说"不对"时修正，自动写入回归测试
5. **跑测试** — 验证所有历史纠正没有被破坏

Do NOT list CLI commands to the user. Just describe what you can do and ask what they want.

## Setup

Before running any command, ensure the virtual environment exists:

```bash
test -d .venv || (python3 -m venv .venv && .venv/bin/pip install -e .)
```

### Check existing config

**Read `.env` first** to see what's already configured. Do NOT ask the user to configure keys that already have values.

```bash
grep -E "^(TMDB_READ_ACCESS_TOKEN|BANGUMI_ACCESS_TOKEN|CLOUD_115_COOKIES)=" .env 2>/dev/null
```

- If `TMDB_READ_ACCESS_TOKEN` has a value → TMDB is ready, no action needed
- If `BANGUMI_ACCESS_TOKEN` has a value → Bangumi is ready
- If `CLOUD_115_COOKIES` has a value → 115 is ready. Optionally verify: `.venv/bin/python -m media115.cli auth --check`
- If `CLOUD_115_COOKIES` is missing or empty → 115 needs login (see "115 Login" below)

Only ask the user to configure keys that are missing or empty. If all keys are present, skip setup and go straight to "What You Can Do".

### 115 Login

**IMPORTANT**: The `115-media auth` command is INTERACTIVE — it prints a URL and waits for the user to scan a QR code. You CANNOT run it via Bash tool (it will block forever).

When 115 login is needed, tell the user to run it themselves:

> "115 还没登录。请在终端运行以下命令，然后用 115 App 扫码：
>
> `.venv/bin/115-media auth`
>
> 扫码完成后告诉我，我继续操作。"

Do NOT attempt to run `115-media auth` yourself. Wait for the user to confirm login is done, then verify with:
```bash
.venv/bin/python -m media115.cli auth --check
```

## Project Layout

```
src/media115/
├── cli.py             # CLI entry point (run via: .venv/bin/python -m media115.cli)
├── client.py          # 115 client (cookie mode + OpenAPI mode)
├── _crypto.py         # M115 encryption (RSA+XOR, for download URLs)
├── proxy.py           # Jellyfin reverse proxy + 302 redirect (async)
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
└── test_*.py          # 108 tests
```

## CLI Reference

```bash
.venv/bin/python -m media115.cli auth                              # QR login, saves cookies to .env
.venv/bin/python -m media115.cli ls /影音                           # List directory (path or dir_id)
.venv/bin/python -m media115.cli scan /影音/电影                    # Dry-run scan: analyze + plan table
.venv/bin/python -m media115.cli scan /影音 --no-recursive          # Scan single directory only
.venv/bin/python -m media115.cli scrape "QUERY"                    # Search TMDB (default)
.venv/bin/python -m media115.cli scrape "QUERY" --source bangumi   # Search Bangumi
.venv/bin/python -m media115.cli scrape "QUERY" --source javbus    # Search JavBus
.venv/bin/python -m media115.cli upload FILE --remote-dir DIR_ID   # Rapid upload to 115
.venv/bin/python -m media115.cli serve --port 9000                 # Start strm-proxy
```

## Running Tests

```bash
.venv/bin/pytest tests/ -v                          # All tests (108)
.venv/bin/pytest tests/test_scrape_regression.py -v  # Regression tests only
```

---

## Workflows

### Workflow: Scrape

Input: a folder path containing media files (local or 115 cloud path like `/影音/电影/`).

**Step 1 — Scan.** Run the scan command first to get an overview:

```bash
.venv/bin/python -m media115.cli scan /path/to/folder
```

Or list files and analyze manually. For each video file:
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

### You are an OPERATOR, not a developer

You are here to **use** this tool, not to modify it. Your job is to run CLI commands, analyze output, and interact with the user.

**DO NOT:**
- Modify any `.py` file under `src/` or `tests/`
- Change `pyproject.toml`, `.env.example`, `.gitignore`, or this file
- Refactor, "improve", or "fix" the codebase
- Add new features, dependencies, or files
- Run `pip install` for new packages

**DO:**
- Run CLI commands: `115-media auth`, `115-media scan`, `115-media scrape`, etc.
- Run Python one-liners to call the scraper API (as shown in Workflows above)
- Read files to understand structure
- Write/modify ONLY these file types: `.nfo`, `.jpg`, `.png`, `.strm`
- Append to `tests/scrape_cases.json` (via the `add_case` API only)
- Run `pytest` to verify

If you think the code has a bug, tell the user. Do not fix it yourself.

### Other rules

- **Never** modify video file binaries
- **Always** output results as markdown tables
- **Always** save corrections to `tests/scrape_cases.json`
- **Always** run regression tests after corrections
- **Always** verify tests pass before claiming work is done

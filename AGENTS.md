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

Before running any command, ensure the virtual environment and skills are set up:

```bash
test -d .venv || (python3 -m venv .venv && .venv/bin/pip install -e .)
```

### Install skills (Claude Code only, one-time)

Skills are in `skills/` (cross-tool). Claude Code needs them in `.claude/skills/`. Create a symlink if it doesn't exist:

```bash
mkdir -p .claude && test -L .claude/skills || ln -s ../skills .claude/skills
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

Input: a category (电影, AV, 剧目) or the whole /影音 directory.

**Step 1 — Export tree** (if not cached recently):

```bash
.venv/bin/python -m media115.cli export-tree
```

**Step 2 — Batch scrape** (CLI handles most files automatically):

```bash
.venv/bin/python -m media115.cli batch-scrape 电影
.venv/bin/python -m media115.cli batch-scrape AV
.venv/bin/python -m media115.cli batch-scrape 剧目
```

This uses the analyzer (regex rules) + TMDB/jav321/javfree APIs with caching.
Most files will succeed. Some will fail (`not_found` or `unknown`).

**Step 3 — Agent handles failures.** Check batch-scrape output for failed files.
For each failed file, YOU (the agent) should:

1. Look at the filename and use your own judgment to determine:
   - What is this? (movie / tv / anime / av)
   - What's the title? (translate if needed, remove encoding info)
   - What year?

2. Search TMDB/Bangumi to find the correct match:
   ```bash
   .venv/bin/python -m media115.cli scrape "your search query"
   .venv/bin/python -m media115.cli scrape "alternate query" --source bangumi
   ```

3. If multiple results, pick the best match based on year, title similarity.

4. Save the correction as a regression case (see Scrape-Fix workflow).

**Step 4 — Organize** (rename + move files to Jellyfin standard):

```bash
.venv/bin/python -m media115.cli organize 电影           # dry-run first
.venv/bin/python -m media115.cli organize 电影 --execute  # apply
```

**Step 5 — Report.** Output summary: how many scraped, organized, failed.
Ask user if any results look wrong.

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
- Call 115/TMDB/Bangumi APIs directly (e.g., via `curl` or `httpx`). Always use the project CLI or Python one-liners from Workflows section. The project has built-in rate limiting; bypassing it risks getting the account banned.

**DO:**
- Run CLI commands: `115-media auth`, `115-media ls`, `115-media scan`, `115-media scrape`, etc.
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

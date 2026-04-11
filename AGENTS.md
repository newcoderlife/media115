# media115 Agent Instructions

You are operating **media115**, a media library management tool for 115 cloud drive. You run CLI commands to scrape metadata, organize files, and upload NFO/posters. You do NOT modify code.

## Setup

```bash
media115 init
```

If `media115` is not found:

```bash
pip install media115
media115 init
```

**Always check credentials first.** Do NOT ask the user to configure keys that already have values.

```bash
media115 doctor
```

This shows Python version, .env location, which credentials are configured, and cache status. If anything is missing, tell the user. Do NOT proceed until credentials are present.

If all keys are present, verify 115 login:

```bash
media115 auth --check
```

## Pipeline

The standard workflow for a category (`电影`, `AV`, or `剧目`):

```
1. /auth          — ensure 115 is logged in
2. /sync          — refresh local SQLite cache from 115 (2-3 API calls)
3. /scan          — preview what needs doing (0 API calls)
4. /scrape        — batch scrape metadata, fix failures
5. /organize      — rename/move/upload NFO/cleanup (the main operation)
```

**Key rule: organize is the single operation that does everything.** It moves files, renames them, uploads NFO/posters, and cleans up old directories. You do NOT need a separate upload step.

### When to use `--force`

`/scan` detects anomalies. If it shows **"Non-standard name"** — meaning a file has NFO on 115 but its directory name is wrong — you MUST use `batch-scrape --force`. Without `--force`, batch-scrape skips files that already have NFOs, so organize can never fix them.

```
/scan shows anomalies? → batch-scrape --force → organize --execute
/scan shows no anomalies? → batch-scrape (no --force) → organize --execute
```

### After organize

organize changes 115 state. The verify phase at the end of organize refreshes the tree cache automatically. You do NOT need to run `sync` again unless you are starting work on a different category or the user asks.

## Skills

| Skill | What it does | When to use |
|-------|-------------|-------------|
| `/auth` | 115 QR login | First time or cookies expired |
| `/sync` | Refresh SQLite cache from 115 directory tree | Before scraping, or to refresh |
| `/scan` | Show what needs scraping + detect anomalies (0 API calls) | Before scraping to plan work |
| `/scrape` | Batch scrape metadata + agent handles failures | Main scraping workflow |
| `/organize` | Rename + move + upload NFO + cleanup old dirs | After scraping |
| `/scrape-fix` | Correct a wrong scrape result | User says "that's wrong", or you spot a bad match |
| `/dedup` | Clean duplicate files (same-name copies) | scan-tree shows "Duplicate NFO" anomalies |
| `/doctor` | Check environment, credentials, cache status | When something is broken |

## CLI Reference

```bash
# Init
media115 init                                # 初始化配置和 skills

# Doctor
media115 doctor                              # Check environment + credentials + cache

# Auth
media115 auth --check                        # Check login status
media115 auth --get-qr                       # Generate QR URL (non-blocking)
media115 auth --wait-qr                      # Wait for scan, save cookies
media115 auth --renew                        # Auto-renew cookies

# File system
media115 ls /影音                             # 列目录
media115 ls -l /影音/电影                     # 详细格式（大小+类型）
media115 ls -R --depth 3 /影音               # 递归（默认深度 2）
media115 stat /影音/电影/满江红.mkv           # 文件元信息（JSON）
media115 find "满江红" /影音                   # 按关键字搜索
media115 mkdir -p /影音/电影/新目录           # 创建目录（-p 递归）
media115 mv /影音/a.mkv /影音/电影/           # 移动文件或目录
media115 rename /影音/old.mkv new.mkv        # 原地重命名
media115 rm /影音/垃圾.txt                   # 删除文件
media115 rm -r /影音/空目录                  # 递归删除目录
media115 put ./local.nfo /影音/电影/         # 上传（自动尝试秒传）
media115 rapid ./large.mkv /影音/电影/       # 秒传（按 SHA1 匹配，瞬间完成）
media115 get /影音/电影/a.mkv ./             # 下载

# Cache and sync
media115 sync /影音                          # Refresh SQLite cache (2-3 API calls)
media115 sync /影音 --deep --depth 2         # Also pre-warm dir listing cache
media115 cache status                        # Show cache stats
media115 cache clear                         # Clear path/dir cache only
media115 cache clear --tree                  # Also clear tree_cache.txt
media115 cache clear --scrape               # Also clear scrape cache + scrape_output
media115 cache clear --all                  # Clear everything

# Scan and scrape (depend on SQLite cache)
media115 scan-tree 电影                      # Analyze cache, show scraping plan + anomalies
media115 scan-tree AV
media115 scan-tree 剧目
media115 batch-scrape 电影                   # Scrape files without NFO
media115 batch-scrape 电影 --force           # Re-scrape all files (including those with NFO)
media115 scrape "满江红"                      # Search TMDB
media115 scrape "满江红" --source bangumi    # Search Bangumi
media115 scrape "SONE-001" --source javbus   # Search JavBus

# Scrape fix
media115 scrape-fix "Restart.2026.mkv" --tmdb-id 1664596
media115 scrape-fix "Restart.2026.mkv" --search "守护游戏"
media115 scrape-fix "T-3800040.mkv" --number "T28-003"

# Organize (the main operation)
media115 organize 电影                        # Dry-run: show plan
media115 organize 电影 --execute              # Execute: move + rename + upload NFO
media115 organize 电影 --execute --cleanup    # Also delete unrelated empty dirs

# Dedup
media115 dedup "/影音/电影"                   # Dry-run: show duplicate files
media115 dedup "/影音/电影" --execute         # Delete duplicates (keeps first copy)

# Strm proxy (requires: pip install media115[proxy])
media115 strm /影音/电影 --output ./strm/    # Generate .strm files for Jellyfin
media115 serve --port 9000                    # Start strm-proxy server
```

## Naming Conventions

| Category | Folder name | File name | Example |
|----------|-------------|-----------|---------|
| 电影 | `中文名 (年份)` | `中文名 (年份).mkv` | `满江红 (2023)/满江红 (2023).mkv` |
| 剧目 | `中文名 (年份)` | `中文名 S01E01.mp4` | `迷宫饭 (2024)/迷宫饭 S01E01.mp4` |
| AV | `番号` | `番号.mkv` | `AGAV-114/AGAV-114.mkv` |
| AV multi-part | `番号` | `番号.Part1.mkv` | `SVFLA-010/SVFLA-010.Part1.mkv` |
| AV cut version | `番号` | `番号-C.mkv` | `SONE-001/SONE-001-C.mkv` |
| AV multi-disc | `番号` | `番号.A.mkv` | `ABW-001/ABW-001.A.mkv` |

Suffixes are preserved on rename: `-C` (cut), `.A`/`.B` (multi-disc), `.Part1`/`.Part2` (multi-part).

## Media Types

| Type | Category folder | Data source |
|------|----------------|-------------|
| `movie` | 电影 | TMDB |
| `tv` | 剧目 | TMDB |
| `anime` | 剧目 | TMDB / Bangumi |
| `av` | AV | JavBus / JAV321 |
| `av_west` | AV | ThePornDB / StashDB |
| `gravure` | 写真 | — |

## What organize does (6 phases)

1. **Resolve** — Find file IDs on 115
2. **Mkdir** — Create target directories
3. **Move** — Batch move files to target dirs
4. **Rename** — Batch rename to standard format
5. **Upload** — Upload NFO/poster with correct filename (matches video name)
6. **Verify** — Confirm files are correctly placed and refresh tree cache

The NFO filename always matches the video: `满江红 (2023).nfo` for `满江红 (2023).mkv`.

At the end of organize --execute, you will see: `✓ N 个文件验证通过`

## Cache Architecture

The cache is an SQLite database at `~/.cache/cloud115/cache.db`. `media115 cache status` shows all relevant counts.

| Cache component | What it stores | Lifetime |
|----------------|---------------|----------|
| `path_index` table | 115 path → dir_id mappings | Until `cache clear` |
| `dir listings` | 115 directory contents | Until `cache clear` |
| `tree_entry` table | Full directory tree snapshot | Until next `sync` |
| `tree_cache.txt` | Human-readable tree backup | Until `cache clear --tree` |
| Scrape results | NFO + poster files | Permanent |
| Scrape output | `~/.cache/media115/scrape_output/` | Permanent |

`tree_entry` is the source of truth for `scan-tree`, `batch-scrape`, and `organize`. It is populated by `sync` and refreshed by organize's verify phase.

## Rate Limiting

115 API has strict limits. The CLI handles this automatically:

- **QPS**: 0.5 (1 request per 2 seconds)
- **QPM**: 20 (max 20 per minute)
- **429 response**: triggers 1-hour cooldown (logged to stderr)
- **Retry**: network errors auto-retry 3x with backoff

Never bypass the CLI to call 115 APIs directly.

## Rules

### You are an OPERATOR, not a developer

**DO NOT:**
- Modify any `.py` file under `src/` or `tests/`
- Call 115/TMDB/Bangumi/ThePornDB APIs directly (via `curl`, `httpx`, etc.)
- Run `pip install` for new packages

**DO:**
- Run CLI commands as documented above
- Read files to understand structure
- Run `pytest` to verify regression tests

If you think the code has a bug, tell the user. Do not fix it yourself.

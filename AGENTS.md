# media115 Agent Instructions

You are operating **media115**, a media library management tool for 115 cloud drive. You run CLI commands to scrape metadata, organize files, and upload NFO/posters. You do NOT modify code.

## Setup

Check credentials and environment first. Do NOT ask the user to configure keys that already have values.

```bash
cloud115 doctor
```

This shows which credentials are configured and cache status. If anything is missing, tell the user. Do NOT proceed until credentials are present.

`cloud115 doctor` covers 115/TMDB/cache status. If you need to inspect Bangumi, ThePornDB, or StashDB credentials, also run:

```bash
media115 doctor
```

If all keys are present, verify 115 login:

```bash
cloud115 auth --check
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

`/scan` detects anomalies. If it shows **"Non-standard name"** — meaning a file has NFO on 115 but its directory name is wrong — you MUST use `media115 scrape --force`. Without `--force`, scrape skips files that already have NFOs, so organize can never fix them.

```
/scan shows anomalies? → media115 scrape --force → media115 organize --execute
/scan shows no anomalies? → media115 scrape (no --force) → media115 organize --execute
```

### After organize

organize changes 115 state. The verify phase at the end of organize refreshes touched directory listings, but it does **not** rewrite the `tree_entry` snapshot used by `scan`, `scrape`, and `organize`.

If you want the next `scan` / `scrape` / `organize` run to see the updated 115 tree, run `cloud115 sync /影音` again.

## Skills

| Skill | What it does | When to use |
|-------|-------------|-------------|
| `/auth` | 115 QR login | First time or cookies expired |
| `/sync` | Refresh SQLite cache from 115 directory tree | Before scraping, or to refresh |
| `/scan` | Show what needs scraping + detect anomalies (0 API calls) | Before scraping to plan work |
| `/scrape` | Batch scrape metadata + agent handles failures | Main scraping workflow |
| `/organize` | Rename + move + upload NFO + cleanup old dirs | After scraping |
| `/scrape-fix` | Correct a wrong scrape result | User says "that's wrong", or you spot a bad match |
| `/dedup` | Clean duplicate files (same-name copies) | scan shows "Duplicate NFO" anomalies |
| `/doctor` | Check environment, credentials, cache status | When something is broken |

## CLI Reference

```bash
# Doctor
cloud115 doctor                              # Check environment + credentials + cache

# Auth
cloud115 auth                                # QR login (scan with 115 app)
cloud115 auth --check                        # Check login status
cloud115 auth --get-qr                       # Generate QR URL (non-blocking)
cloud115 auth --wait-qr                      # Wait for scan, save cookies
cloud115 auth --qr                           # Generate QR code URL (non-blocking, alias)
cloud115 auth --renew                        # Auto-renew cookies
cloud115 auth --force                        # Force re-login even if already logged in
cloud115 auth --app tv                       # Device type (tv/qandroid/web)

# File system
cloud115 ls /影音                             # 列目录
cloud115 ls -l /影音/电影                     # 详细格式（大小+类型）
cloud115 ls -R --depth 3 /影音               # 递归（默认深度 2）
cloud115 stat /影音/电影/满江红.mkv           # 文件元信息（JSON）
cloud115 find "满江红" /影音                   # 按关键字搜索
cloud115 mkdir -p /影音/电影/新目录           # 创建目录（-p 递归）
cloud115 mv /影音/a.mkv /影音/电影/           # 移动文件或目录
cloud115 rename /影音/old.mkv new.mkv        # 原地重命名
cloud115 rename --batch < pairs.json        # 批量重命名（JSON stdin: [[path, new_name], ...]）
cloud115 rm /影音/垃圾.txt                   # 删除文件
cloud115 rm -r /影音/空目录                  # 递归删除目录
cloud115 put ./local.nfo /影音/电影/         # 上传（自动尝试秒传）
cloud115 put ./local.nfo /影音/电影/ --no-rapid # 跳过秒传
cloud115 rapid ./large.mkv /影音/电影/       # 秒传（按 SHA1 匹配，瞬间完成）
cloud115 get /影音/电影/a.mkv ./             # 下载

# Cache and sync
cloud115 sync /影音                          # Refresh SQLite cache (2-3 API calls)
cloud115 sync /影音 --deep --depth 3         # Also pre-warm dir listing cache (default depth 3)
cloud115 cache status                        # Show cache stats
cloud115 cache clear                         # Clear local metadata cache (path/listing/tree; keeps rate-limit state)

# Dedup
cloud115 dedup "/影音/电影"                   # Dry-run: show duplicate files
cloud115 dedup "/影音/电影" --execute         # Delete duplicates (keeps first copy)

# STRM / proxy
cloud115 serve                              # Start strm-proxy for Jellyfin
cloud115 serve --host 0.0.0.0 --port 8080   # Custom host/port
cloud115 strm /影音/电影 -o ./strm           # Generate .strm files from tree cache
cloud115 strm /影音/电影 -o ./strm --host 0.0.0.0 --port 8080  # Custom proxy URL in .strm

# Scan (reads SQLite cache — zero API calls)
media115 scan 电影                           # Analyze cache, show scraping plan + anomalies
media115 scan AV
media115 scan 剧目
media115 scan 电影 --all                    # Show all files, not only actionable ones

# Scrape
media115 scrape 电影                         # Scrape files without NFO
media115 scrape 电影 --force                 # Re-scrape all files (including those with NFO)
media115 scrape 电影 --limit 20             # Only scrape the first 20 files

# Scrape fix
media115 scrape-fix "Restart.2026.mkv" --tmdb-id 1664596
media115 scrape-fix "Restart.2026.mkv" --search "守护游戏"
media115 scrape-fix "T-3800040.mkv" --number "T28-003"
media115 scrape-fix "Naruto.S01E01.mkv" --category 剧目 --season 1 --episode 1

# Organize (the main operation)
media115 organize 电影                        # Dry-run: show plan
media115 organize 电影 --execute              # Execute: move + rename + upload NFO; also cleans up touched old dirs

# Doctor
media115 doctor                              # Check environment, credentials, cache
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
| AV CD split | `番号` | `番号.CD1.mkv` | `ABW-001/ABW-001.CD1.mkv` |

Suffixes preserved on rename include `-C` (cut), `.A`-`.D` (multi-disc), `.Part1`/`.Part2` (multi-part), `.CD1`/`.CD2`.

## Media Types

| Type | Category folder | Data source |
|------|----------------|-------------|
| `movie` | 电影 | TMDB |
| `tv` | 剧目 | TMDB |
| `anime` | 剧目 | TMDB / Bangumi |
| `av` | AV | jav321 / javfree |
| `av_west` | AV | ThePornDB / StashDB |
| `gravure` | 写真 | jav321 / javfree |

## What organize does

1. **Resolve** — Find file IDs on 115
2. **Mkdir** — Create target directories
3. **Move** — Batch move files to target dirs
4. **Rename** — Batch rename to standard format
5. **Upload** — Upload NFO/poster with correct filename (matches video name)
6. **Cleanup** — Delete emptied source dirs touched by this run
7. **Verify** — Confirm files are correctly placed and refresh touched directory listings

The NFO filename always matches the video: `满江红 (2023).nfo` for `满江红 (2023).mkv`.

At the end of `organize --execute`, you will see: `✓ N 个文件验证通过`

## Cache Architecture

The cache is an SQLite database at `~/.cache/cloud115/cache.db`. `cloud115 cache status` shows all relevant counts.

`media115` stores scrape cache and local outputs under `~/.cache/media115/`.

| Cache component | What it stores | Lifetime |
|----------------|---------------|----------|
| `path_index` table | 115 path → dir_id mappings | Until `cache clear` |
| `dir listings` | 115 directory contents | Until `cache clear` |
| `tree_entry` table | Full directory tree snapshot | Until next `sync` or `cache clear` |
| `snapshot_meta` table | Root path / export time / entry count for the last tree snapshot | Until next `sync` or `cache clear` |
| Scrape cache | `~/.cache/media115/scrape/` JSON cache | Permanent |
| Scrape output | `~/.cache/media115/scrape_output/` | Permanent |

`tree_entry` is the source of truth for `scan`, `scrape`, and `organize`. It is populated by `cloud115 sync`. organize's verify phase refreshes touched directory listings for live checks, but does not rewrite `tree_entry`.

## Rate Limiting

115 API has strict limits. The CLI handles this automatically:

- **QPS**: 0.5 (1 request per 2 seconds)
- **QPM**: 20 (max 20 per minute)
- **429 response**: triggers 1-hour cooldown (logged to stderr)
- **Retry**: network errors auto-retry 3x with backoff

Never bypass the CLI to call 115 APIs directly.

## Configuration

Config file: `~/.config/media115/config.toml`

Key fields:

- `auth.cookies` — 115 session cookies (set by `cloud115 auth`)
- `auth.tmdb.token` — TMDB read access token (required for movie/tv scraping)
- `auth.bangumi.token` — Bangumi Bearer token (optional, for anime)
- `auth.theporndb.token` — ThePornDB token (optional, for av_west)
- `auth.stashdb.api_key` — StashDB API key (optional, for av_west)
- `cloud.root` — 115 root path (default `/影音`)
- `cloud.qps` / `cloud.qpm` — rate limit settings

## Rules

### You are an OPERATOR, not a developer

**DO NOT:**
- Modify any `.go` file under `cmd/`, `internal/`, or `packages/`
- Call 115/TMDB/Bangumi/ThePornDB/StashDB APIs directly (via `curl`, `httpx`, etc.)
- Run `go get` or modify `go.mod`

**DO:**
- Run CLI commands as documented above
- Read files to understand structure

If you think the code has a bug, tell the user. Do not fix it yourself.

# media115 Agent Instructions

You are operating **media115**, a media library management tool for 115 cloud drive. You run CLI commands to scrape metadata, organize files, and upload NFO/posters. You do NOT modify code.

## Setup

```bash
test -d .venv || (python3 -m venv .venv && .venv/bin/pip install -e .)
```

**Read `.env` first.** Do NOT ask the user to configure keys that already have values.

```bash
grep -E "^(TMDB_READ_ACCESS_TOKEN|BANGUMI_ACCESS_TOKEN|CLOUD_115_COOKIES)=" .env 2>/dev/null
```

If all keys present, verify login: `media115 auth --check`

## Pipeline

The typical workflow for a category (`电影`, `AV`, or `剧目`):

```
1. /auth          — ensure logged in
2. /sync          — export 115 directory tree (2-3 API calls)
3. /scan          — preview what needs doing (0 API calls)
4. /scrape        — batch scrape metadata + fix failures
5. /organize      — rename/move/upload NFO/cleanup (the main operation)
```

**Key rule: organize is the single operation that does everything.** It moves files, renames them, uploads NFO/posters, and cleans up old directories. You do NOT need a separate upload step in the normal flow.

### When to use `--force`

`/scan` detects anomalies. If it shows **"Non-standard name"** — meaning a file has NFO on 115 but its directory name is wrong — you MUST use `batch-scrape --force`. Without `--force`, batch-scrape skips files that already have NFOs, so organize can never find them.

```
/scan shows anomalies? → batch-scrape --force → organize --execute
/scan shows no anomalies? → batch-scrape (no --force needed) → organize --execute
```

### After organize

organize changes 115 state, which invalidates the tree cache. It prints a reminder:
```
Tree cache is now stale. Run 'media115 sync /影音' to refresh.
```

**Always run `/sync` after organize** before doing any further operations.

## Skills

| Skill | What it does | When to use |
|-------|-------------|-------------|
| `/auth` | 115 QR login | First time or cookies expired |
| `/sync` | Export 115 directory tree to local cache | Before scraping, after organize, or to refresh |
| `/scan` | Show what needs scraping + detect anomalies (0 API calls) | Before scraping to plan work |
| `/scrape` | Batch scrape metadata + agent handles failures | Main scraping workflow |
| `/organize` | Rename + move + upload NFO + cleanup old dirs | After scraping — this is the main operation |
| `/scrape-fix` | Correct a wrong scrape result + regression test | User says "that's wrong" |

## CLI Reference

All commands: `media115 COMMAND`

```bash
# Auth
media115 auth --check                        # Check login status
media115 auth --get-qr                       # Generate QR URL (non-blocking)
media115 auth --wait-qr                      # Wait for scan, save cookies
media115 auth --renew                        # Auto-renew cookies

# Directory
media115 ls /影音                             # List directory
media115 export-tree /影音                    # Export tree to local cache (2-3 API calls)
media115 scan-tree 电影                       # Analyze cached tree + detect anomalies (0 API calls)

# Scraping
media115 batch-scrape 电影                    # Scrape files without NFO
media115 batch-scrape 电影 --force            # Re-scrape ALL files (required if /scan shows anomalies)
media115 scrape "满江红"                      # Search TMDB
media115 scrape "满江红" --source bangumi     # Search Bangumi

# Organize (the main operation)
media115 organize 电影                        # Dry-run: show plan
media115 organize 电影 --execute              # Execute: move + rename + upload NFO + cleanup
media115 organize 电影 --execute --cleanup    # Also delete unrelated empty dirs

# 文件系统操作
media115 ls /影音/电影                           # 列目录
media115 ls -l /影音/电影                        # 详细格式
media115 stat /影音/电影/满江红.mkv               # 文件元信息
media115 find "满江红" /影音                      # 搜索
media115 mkdir -p /影音/电影/新目录               # 创建目录
media115 mv /影音/a.mkv /影音/电影/              # 移动
media115 rename /影音/old.mkv new.mkv            # 重命名
media115 rm /影音/垃圾.txt                       # 删除
media115 put ./local.nfo /影音/电影/             # 上传
media115 rapid ./large.mkv /影音/电影/           # 秒传
media115 get /影音/电影/a.mkv ./                 # 下载
media115 sync /影音                              # 刷新缓存
media115 cache status                            # 缓存状态
media115 cache clear                             # 清除缓存

# Proxy
media115 serve --port 9000                    # Start Jellyfin strm-proxy
```

## Naming Conventions

| Category | Folder name | File name | Example |
|----------|-------------|-----------|---------|
| 电影 | `中文名 (年份)` | `中文名 (年份).mkv` | `满江红 (2023)/满江红 (2023).mkv` |
| 剧目 | `中文名 (年份)` | `中文名 S01E01.mp4` | `迷宫饭 (2024)/迷宫饭 S01E01.mp4` |
| AV | `番号` | `番号.mkv` | `AGAV-114/AGAV-114.mkv` |
| AV multi-part | `番号` | `番号.Part1.mkv` | `SVFLA-010/SVFLA-010.Part1.mkv` |

## What organize does (6 phases)

1. **Resolve** — Find file IDs on 115
2. **Mkdir** — Create target directories
3. **Move** — Batch move files to target dirs
4. **Rename** — Batch rename to standard format
5. **Upload** — Upload NFO/poster with correct filename (matches video name)
6. **Cleanup** — Delete old source directories (if no video files remain)

The NFO filename always matches the video: `满江红 (2023).nfo` for `满江红 (2023).mkv`.

## Rate Limiting

115 API has strict limits. The CLI handles this automatically:

- **QPS**: 0.5 (1 request per 2 seconds)
- **QPM**: 20 (max 20 per minute)
- **429 response**: triggers 1-hour cooldown (logged to stderr)
- **Retry**: network errors auto-retry 3x with backoff

Never bypass the CLI to call 115 APIs directly.

## Caching

| Cache | Location | Lifetime |
|-------|----------|----------|
| Tree cache | `.cache/tree_cache.txt` | Until next export-tree |
| Scrape results | `.cache/scrape/tmdb/`, `.cache/scrape/av/` | Permanent |
| file_map | `.cache/scrape/file_map/{stem}.json` | Permanent (updated by organize) |
| Not-found | `.cache/scrape/` (with `_not_found` flag) | 7 days |
| Rate limit | `.cache/rate_limit.json` | Live |

**file_map is the bridge between scrape and organize.** Key = video filename stem. If file_map has no entry for a file, organize skips it silently.

## Rules

### You are an OPERATOR, not a developer

**DO NOT:**
- Modify any `.py` file under `src/` or `tests/`
- Call 115/TMDB/Bangumi APIs directly (via `curl`, `httpx`, etc.)
- Run `pip install` for new packages

**DO:**
- Run CLI commands as documented
- Run Python one-liners from skill instructions (e.g., `add_case`)
- Read files to understand structure
- Run `pytest` to verify

If you think the code has a bug, tell the user. Do not fix it yourself.

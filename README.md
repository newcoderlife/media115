# media115

[![CI](https://github.com/newcoderlife/media115/actions/workflows/ci.yml/badge.svg)](https://github.com/newcoderlife/media115/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Media library management tool for 115 cloud drive. Scrapes metadata from TMDB, Bangumi, jav321, javfree, ThePornDB, and StashDB; organizes files into standard naming; uploads NFO and posters.

Two binaries:

- **`cloud115`** — 115 file system operations, cache management, auth
- **`media115`** — metadata scraping and library organization

## Installation

```bash
go install github.com/newcoderlife/media115/cmd/cloud115@latest
go install github.com/newcoderlife/media115/cmd/media115@latest
```

## Quick Start

```bash
# 1. Authenticate with 115
cloud115 auth

# 2. Sync the local SQLite cache from 115
cloud115 sync /影音

# 3. Preview what needs scraping
media115 scan 电影

# 4. Scrape metadata
media115 scrape 电影

# 5. Rename, move, and upload NFO/posters
media115 organize 电影 --execute
```

## Configuration

The config file lives at `~/.config/media115/config.toml`.

```toml
[auth]
cookies = ""

[auth.tmdb]
token = ""

[auth.bangumi]
token = ""            # optional — only needed for anime

[auth.theporndb]
token = ""

[auth.stashdb]
api_key = ""

[cloud]
root = "/影音"
qps = 0.5
qpm = 20
cooldown_seconds = 3600

[cache]
listing_ttl = 3600
path_ttl = 86400

[categories.电影]
type = "movie"
naming = "{title} ({year})"
sources = ["tmdb"]

[categories.剧目]
type = "tv"
naming = "{title} ({year})"
sources = ["tmdb", "bangumi"]

[categories.AV]
type = "av"
naming = "{number}"
sources = ["jav321", "javfree"]

[categories.写真]
type = "gravure"
naming = "{number}"
sources = ["jav321", "javfree"]

[proxy]
host = "127.0.0.1"
port = 9000
jellyfin_url = "http://localhost:8096"
```

## Supported Media Types

| Type | Category folder | Data source |
|------|----------------|-------------|
| `movie` | 电影 | TMDB |
| `tv` | 剧目 | TMDB |
| `anime` | 剧目 | TMDB / Bangumi |
| `av` | AV | jav321 / javfree |
| `av_west` | AV | ThePornDB / StashDB |
| `gravure` | 写真 | jav321 / javfree |

## CLI Reference

### cloud115

```bash
# Auth
cloud115 auth                        # QR login (scan with 115 app)
cloud115 auth --check                # Check login status
cloud115 auth --get-qr               # Print QR URL and exit
cloud115 auth --wait-qr              # Block until QR scanned, save cookies
cloud115 auth --renew                # Auto-renew cookies

# File system
cloud115 ls /影音                    # List directory
cloud115 ls -l /影音/电影            # Long format (size + type)
cloud115 ls -R --depth 3 /影音       # Recursive (default depth 2)
cloud115 stat /影音/电影/满江红.mkv  # File metadata (JSON)
cloud115 find "满江红" /影音         # Search by keyword
cloud115 mkdir -p /影音/电影/新目录  # Create directory (recursive)
cloud115 mv /影音/a.mkv /影音/电影/  # Move file or directory
cloud115 rename /影音/old.mkv new    # Rename in place
cloud115 rm /影音/垃圾.txt           # Delete file
cloud115 rm -r /影音/空目录          # Delete directory recursively
cloud115 put ./local.nfo /影音/电影/ # Upload (tries rapid upload first)
cloud115 rapid ./large.mkv /影音/电影/ # Rapid upload (SHA1 match, instant)
cloud115 get /影音/电影/a.mkv ./     # Download

# Cache and sync
cloud115 sync /影音                  # Refresh SQLite cache (2-3 API calls)
cloud115 sync /影音 --deep --depth 2 # Also pre-warm dir listing cache
cloud115 cache status                # Show cache stats
cloud115 cache clear                 # Clear path/dir cache
cloud115 cache clear --tree          # Also clear tree_cache.txt
cloud115 cache clear --scrape        # Also clear scrape cache + scrape_output
cloud115 cache clear --all           # Clear everything

# Utilities
cloud115 dedup "/影音/电影"          # Dry-run: show duplicate files
cloud115 dedup "/影音/电影" --execute # Delete duplicates (keeps first copy)
cloud115 doctor                      # Check environment, credentials, cache
```

### media115

```bash
# Scan (zero API calls — reads SQLite cache)
media115 scan 电影                   # Analyze cache, show scraping plan + anomalies
media115 scan AV
media115 scan 剧目

# Scrape
media115 scrape 电影                 # Scrape files without NFO
media115 scrape 电影 --force         # Re-scrape all files (including those with NFO)
media115 scrape "满江红"             # Search TMDB for a single title
media115 scrape "满江红" --source bangumi   # Search Bangumi
media115 scrape "SONE-001" --source jav321  # Search jav321

# Scrape fix
media115 scrape-fix "Restart.2026.mkv" --tmdb-id 1664596
media115 scrape-fix "Restart.2026.mkv" --search "守护游戏"
media115 scrape-fix "T-3800040.mkv" --number "T28-003"

# Organize
media115 organize 电影               # Dry-run: show plan
media115 organize 电影 --execute     # Execute: move + rename + upload NFO
media115 organize 电影 --execute --cleanup  # Also delete unrelated empty dirs

# Doctor
media115 doctor                      # Check environment, credentials, cache
```

## License

<!-- TODO: add license -->

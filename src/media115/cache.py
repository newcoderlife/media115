"""Simple file-based cache for scraped metadata and rate limit state.

All cache files live in .cache/ directory (gitignored).

Structure:
  .cache/
  ├── rate_limit.json          # QPS/QPM/cooldown state (cross-process)
  ├── tree_cache.txt           # 115 directory tree export
  └── scrape/                  # Scraped metadata cache
      ├── tmdb/
      │   ├── movie_603.json   # TMDB movie detail by ID
      │   └── tv_1396.json    # TMDB TV detail by ID
      ├── jav321/
      │   └── DANDY-992.json  # AV metadata by number
      └── javfree/
          └── DANDY-992.json
"""

import json
import time
from pathlib import Path

CACHE_DIR = ".cache"


def _cache_dir(subdir: str = "") -> Path:
    d = Path.cwd() / CACHE_DIR
    if subdir:
        d = d / subdir
    d.mkdir(parents=True, exist_ok=True)
    return d


def get(source: str, key: str) -> dict | None:
    """Get cached metadata. Returns None if not cached."""
    path = _cache_dir(f"scrape/{source}") / f"{key}.json"
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text())
    except (json.JSONDecodeError, ValueError):
        return None


def put(source: str, key: str, data: dict):
    """Cache metadata. Overwrites existing."""
    data["_cached_at"] = time.time()
    path = _cache_dir(f"scrape/{source}") / f"{key}.json"
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2))


def has(source: str, key: str) -> bool:
    """Check if a cache entry exists."""
    return (_cache_dir(f"scrape/{source}") / f"{key}.json").exists()


def rate_limit_path() -> Path:
    """Path to rate limit state file."""
    return _cache_dir() / "rate_limit.json"


def tree_cache_path() -> Path:
    """Path to 115 directory tree cache."""
    return _cache_dir() / "tree_cache.txt"

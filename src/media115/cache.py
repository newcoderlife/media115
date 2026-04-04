"""Simple file-based cache for scraped metadata and rate limit state.

All cache files live in .cache/ directory (gitignored).

Cache policy (follows MetaTube pattern):
- Successful scrape → cached forever (metadata is immutable)
- not_found → cached 7 days (new releases get added to databases)
- Rate limit state → cross-process persistence

Structure:
  .cache/
  ├── rate_limit.json          # QPS/QPM/cooldown state
  ├── tree_cache.txt           # 115 directory tree export
  └── scrape/                  # Scraped metadata cache
      ├── tmdb/
      │   └── movie_603.json   # Keyed by source + ID
      └── av/
          └── DANDY-992.json   # Keyed by number
"""

import json
import time
from pathlib import Path

CACHE_DIR = ".cache"
NOT_FOUND_TTL = 7 * 24 * 3600  # 7 days


def _cache_dir(subdir: str = "") -> Path:
    d = Path.cwd() / CACHE_DIR
    if subdir:
        d = d / subdir
    d.mkdir(parents=True, exist_ok=True)
    return d


def get(source: str, key: str, max_age: float = 0) -> dict | None:
    """Get cached metadata. Returns None if not cached or expired.

    max_age: max age in seconds. 0 = no expiry (default for successful results).
    """
    path = _cache_dir(f"scrape/{source}") / f"{key}.json"
    if not path.exists():
        return None
    try:
        data = json.loads(path.read_text())
    except (json.JSONDecodeError, ValueError):
        return None

    if max_age > 0:
        cached_at = data.get("_cached_at", 0)
        if time.time() - cached_at > max_age:
            return None  # Expired

    return data


def put(source: str, key: str, data: dict):
    """Cache metadata. Overwrites existing."""
    data["_cached_at"] = time.time()
    path = _cache_dir(f"scrape/{source}") / f"{key}.json"
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2))


def put_not_found(source: str, key: str):
    """Cache a not_found result. Expires after NOT_FOUND_TTL."""
    put(source, key, {"_not_found": True})


def is_not_found(source: str, key: str) -> bool:
    """Check if key was recently marked not_found (within TTL)."""
    data = get(source, key, max_age=NOT_FOUND_TTL)
    return data is not None and data.get("_not_found", False)


def has(source: str, key: str) -> bool:
    """Check if a cache entry exists (ignores TTL)."""
    return (_cache_dir(f"scrape/{source}") / f"{key}.json").exists()


def rate_limit_path() -> Path:
    """Path to rate limit state file."""
    return _cache_dir() / "rate_limit.json"


def tree_cache_path() -> Path:
    """Path to 115 directory tree cache."""
    return _cache_dir() / "tree_cache.txt"

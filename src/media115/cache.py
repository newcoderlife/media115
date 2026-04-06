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

from __future__ import annotations

import json
import os
import time
from pathlib import Path

NOT_FOUND_TTL = 7 * 24 * 3600  # 7 days


def _cache_root() -> Path:
    """~/.cache/media115/ 或 $XDG_CACHE_HOME/media115/"""
    base = os.environ.get("XDG_CACHE_HOME", "")
    if not base:
        base = str(Path.home() / ".cache")
    return Path(base) / "media115"


def _config_root() -> Path:
    """~/.config/media115/ 或 $XDG_CONFIG_HOME/media115/"""
    base = os.environ.get("XDG_CONFIG_HOME", "")
    if not base:
        base = str(Path.home() / ".config")
    return Path(base) / "media115"


def _cache_dir(subdir: str = "") -> Path:
    d = _cache_root()
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


def parse_tree_cache(video_exts: set[str], nfo_ext: str = ".nfo") -> list[dict]:
    """Parse tree_cache.txt into a list of file entries."""

    from media115.utils import split_ext

    path = tree_cache_path()
    if not path.exists():
        return []

    text = path.read_text()
    entries = []
    path_stack: list[str] = []

    for line in text.strip().split("\n"):
        stripped = line.rstrip()
        if "|-" not in stripped:
            continue

        depth = stripped.count("| ")
        name = stripped.split("|-", 1)[1].strip() if "|-" in stripped else ""
        if not name:
            continue

        while len(path_stack) >= depth:
            path_stack.pop() if path_stack else None
        path_stack.append(name)

        full_path = "/".join(path_stack)
        _, ext = split_ext(name)
        is_video = ext.lower() in video_exts
        is_nfo = ext.lower() == nfo_ext

        if is_video or is_nfo:
            parent = "/".join(path_stack[:-1]) if len(path_stack) > 1 else ""
            entries.append(
                {
                    "n": name,
                    "path": full_path,
                    "parent": parent,
                    "is_video": is_video,
                    "is_nfo": is_nfo,
                }
            )

    return entries

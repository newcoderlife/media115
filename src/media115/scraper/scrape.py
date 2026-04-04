"""Unified scraping interface with automatic caching.

All scrapers go through this module. Cache is checked first;
on miss, the appropriate provider is called and result is cached.
"""

import os
import re
from pathlib import Path

from media115 import cache as media_cache
from media115.scraper.nfo import generate_movie_nfo, generate_episode_nfo
from media115.scraper.artwork import save_poster, download_image


def scrape_movie(title: str, year: int | None, filename: str, out_dir: Path) -> dict:
    """Scrape movie metadata from TMDB, generate NFO + poster."""
    from media115.scraper.tmdb import TMDBClient

    token = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
    if not token:
        return {"status": "error", "error": "TMDB_READ_ACCESS_TOKEN not set"}

    client = TMDBClient(read_access_token=token)
    try:
        results = client.search_movie(title)
        if not results and year:
            results = client.search_movie(title.split(".")[0])
        if not results:
            return {"status": "not_found", "query": title}

        match = _best_match(results, year)
        tmdb_id = match["id"]
        detail, images = _get_tmdb_detail(client, "movie", tmdb_id)

        metadata = {
            "title": detail.get("title", ""),
            "originaltitle": detail.get("original_title", ""),
            "year": int((detail.get("release_date", "") or "0000")[:4]),
            "plot": detail.get("overview", ""),
            "runtime": detail.get("runtime"),
            "rating": detail.get("vote_average"),
            "premiered": detail.get("release_date", ""),
            "genres": [g["name"] for g in detail.get("genres", [])],
            "directors": [],
            "actors": [],
            "uniqueids": {"tmdb": str(tmdb_id)},
        }
        imdb_id = detail.get("imdb_id")
        if imdb_id:
            metadata["uniqueids"]["imdb"] = imdb_id

        stem = _stem(filename)
        generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")
        _save_tmdb_poster(images, out_dir)

        return {"status": "ok", "match": detail.get("title", ""), "tmdb_id": tmdb_id}
    finally:
        client.close()


def scrape_tv(
    title: str, season: int | None, episode: int | None, filename: str, out_dir: Path
) -> dict:
    """Scrape TV episode metadata from TMDB, generate NFO."""
    from media115.scraper.tmdb import TMDBClient

    token = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
    if not token:
        return {"status": "error", "error": "TMDB_READ_ACCESS_TOKEN not set"}

    client = TMDBClient(read_access_token=token)
    try:
        clean = re.sub(r"^\[.*?\]\s*", "", title).replace(".", " ").strip()
        results = client.search_tv(clean)
        if not results:
            results = client.search_tv(clean.split()[0] if clean else title)
        if not results:
            return {"status": "not_found", "query": clean}

        match = results[0]
        tmdb_id = match["id"]
        detail, _ = _get_tmdb_detail(client, "tv", tmdb_id)

        stem = _stem(filename)
        metadata = {
            "title": detail.get("name", match.get("name", "")),
            "season": season,
            "episode": episode,
            "aired": detail.get("first_air_date", match.get("first_air_date", "")),
            "uniqueids": {"tmdb": str(tmdb_id)},
        }
        generate_episode_nfo(metadata, out_dir / f"{stem}.nfo")

        return {"status": "ok", "match": metadata["title"], "tmdb_id": tmdb_id}
    finally:
        client.close()


def scrape_av(number: str, filename: str, out_dir: Path) -> dict:
    """Scrape AV metadata. Fallback: jav321 → javfree."""
    # Check cache
    cached = media_cache.get("av", number)
    if cached:
        _write_av_nfo(cached, filename, out_dir, cached.get("_source", "cache"))
        return {"status": "ok", "match": cached.get("title", ""), "number": number}

    from media115.scraper.jav321 import fetch_metadata as jav321_fetch
    from media115.scraper.javfree import fetch_metadata as javfree_fetch

    for fetch, source in [(jav321_fetch, "jav321"), (javfree_fetch, "javfree")]:
        meta = fetch(number)
        if meta and meta.get("title"):
            meta["_source"] = source
            media_cache.put("av", number, meta)
            _write_av_nfo(meta, filename, out_dir, source)
            return {"status": "ok", "match": meta["title"], "number": number}

    return {"status": "not_found", "number": number}


# ── Internal helpers ─────────────────────────────────────────────────


def _get_tmdb_detail(client, media_type: str, tmdb_id: int) -> tuple[dict, dict]:
    """Get TMDB detail + images with cache."""
    cache_key = f"{media_type}_{tmdb_id}"
    cached = media_cache.get("tmdb", cache_key)
    if cached:
        return cached.get("detail", {}), cached.get("images", {})

    if media_type == "movie":
        detail = client.movie_detail(tmdb_id)
        images = client.movie_images(tmdb_id)
    else:
        detail = client.tv_detail(tmdb_id)
        images = {}  # TV doesn't need images per episode

    media_cache.put("tmdb", cache_key, {"detail": detail, "images": images})
    return detail, images


def _best_match(results: list[dict], year: int | None) -> dict:
    """Pick best TMDB search result by year match."""
    if not year:
        return results[0]
    for r in results:
        r_year = (r.get("release_date", r.get("first_air_date", "")) or "")[:4]
        if r_year == str(year):
            return r
    return results[0]


def _save_tmdb_poster(images: dict, out_dir: Path):
    """Download first poster from TMDB images."""
    if images.get("posters"):
        try:
            save_poster(
                images["posters"][0]["file_path"], out_dir, filename="poster.jpg"
            )
        except Exception:
            pass


def _write_av_nfo(meta: dict, filename: str, out_dir: Path, source: str):
    """Generate NFO + download cover for AV."""
    stem = _stem(filename)
    metadata = {
        "title": meta.get("title", ""),
        "originaltitle": meta.get("title", ""),
        "year": int(meta["release_date"][:4]) if meta.get("release_date") else None,
        "plot": "",
        "runtime": int(meta["runtime"]) if meta.get("runtime") else None,
        "genres": meta.get("genres", []),
        "directors": [meta["director"]] if meta.get("director") else [],
        "actors": [{"name": a} for a in meta.get("actors", [])],
        "uniqueids": {source: meta.get("number", "")},
        "studio": meta.get("studio", ""),
    }
    generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")

    if meta.get("cover_url"):
        try:
            download_image(meta["cover_url"], out_dir / "poster.jpg")
        except Exception:
            pass


def _stem(filename: str) -> str:
    dot = filename.rfind(".")
    return filename[:dot] if dot != -1 else filename

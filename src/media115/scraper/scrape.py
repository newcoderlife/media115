"""Unified scraping interface with automatic caching.

All scrapers go through this module. Cache is checked first;
on miss, the appropriate provider is called and result is cached.
"""

import contextlib
import os
import re
from pathlib import Path

from media115 import cache as media_cache
from media115.scraper.artwork import download_image, save_poster
from media115.scraper.nfo import generate_episode_nfo, generate_movie_nfo, generate_tvshow_nfo
from media115.utils import stem as _stem


def scrape_movie(title: str, year: int | None, filename: str, out_dir: Path) -> dict:
    """Scrape movie metadata from TMDB, generate NFO + poster."""
    from media115.scraper.tmdb import TMDBClient

    token = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
    if not token:
        return {"status": "error", "error": "TMDB_READ_ACCESS_TOKEN not set"}

    # Check not_found cache
    cache_key = f"search_{title}_{year}"
    if media_cache.is_not_found("tmdb", cache_key):
        return {"status": "not_found", "query": title, "cached": True}

    client = TMDBClient(read_access_token=token)
    try:
        results = client.search_movie(title)
        if not results and year:
            results = client.search_movie(title.split(".")[0])
        if not results:
            media_cache.put_not_found("tmdb", cache_key)
            return {"status": "not_found", "query": title}

        match = _best_match(results, year)
        tmdb_id = match["id"]
        detail, images, credits = _get_tmdb_movie_full(client, tmdb_id)

        # Build Kodi-standard metadata
        collection = detail.get("belongs_to_collection")
        metadata = {
            "title": detail.get("title", ""),
            "originaltitle": detail.get("original_title", ""),
            "year": int((detail.get("release_date", "") or "0000")[:4]) or year or 0,
            "plot": detail.get("overview", ""),
            "tagline": detail.get("tagline", ""),
            "runtime": detail.get("runtime"),
            "rating": detail.get("vote_average"),
            "votes": detail.get("vote_count"),
            "premiered": detail.get("release_date", ""),
            "genres": [g["name"] for g in detail.get("genres", [])],
            "studios": [c["name"] for c in detail.get("production_companies", [])],
            "countries": [c["name"] for c in detail.get("production_countries", [])],
            "set": collection["name"] if collection else None,
            "directors": [
                p["name"] for p in credits.get("crew", []) if p.get("job") == "Director"
            ],
            "credits": [
                p["name"]
                for p in credits.get("crew", [])
                if p.get("job") in ("Screenplay", "Writer")
            ],
            "actors": [
                {
                    "name": a["name"],
                    "role": a.get("character", ""),
                    "thumb": f"https://image.tmdb.org/t/p/w185{a['profile_path']}"
                    if a.get("profile_path")
                    else "",
                }
                for a in credits.get("cast", [])[:15]
            ],
            "uniqueids": {"tmdb": str(tmdb_id)},
            "thumb": f"https://image.tmdb.org/t/p/original{detail['poster_path']}"
            if detail.get("poster_path")
            else None,
            "fanart": f"https://image.tmdb.org/t/p/original{detail['backdrop_path']}"
            if detail.get("backdrop_path")
            else None,
        }
        imdb_id = detail.get("imdb_id")
        if imdb_id:
            metadata["uniqueids"]["imdb"] = imdb_id

        stem = _stem(filename)
        generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")
        _save_tmdb_poster(images, out_dir)

        # Save filename → scrape result mapping for organize
        media_cache.put(
            "file_map",
            _stem(filename),
            {
                "type": "movie",
                "title": detail.get("title", ""),
                "originaltitle": detail.get("original_title", ""),
                "year": metadata["year"],
                "tmdb_id": tmdb_id,
            },
        )

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

    cache_key = f"search_{title}_{season}_{episode}"
    if media_cache.is_not_found("tmdb", cache_key):
        return {"status": "not_found", "query": title, "cached": True}

    client = TMDBClient(read_access_token=token)
    try:
        clean = re.sub(r"^\[.*?\]\s*", "", title).replace(".", " ").strip()
        results = client.search_tv(clean)
        if not results:
            results = client.search_tv(clean.split()[0] if clean else title)
        if not results:
            media_cache.put_not_found("tmdb", cache_key)
            return {"status": "not_found", "query": clean}

        match = results[0]
        tmdb_id = match["id"]
        detail, _, credits = _get_tmdb_tv_full(client, tmdb_id)

        # Try to get episode-level metadata from season detail
        ep_plot = detail.get("overview", "")
        ep_aired = detail.get("first_air_date", match.get("first_air_date", ""))
        ep_title = detail.get("name", match.get("name", ""))
        if season and episode:
            try:
                season_data = client.season_detail(tmdb_id, season)
                for ep in season_data.get("episodes", []):
                    if ep.get("episode_number") == episode:
                        ep_plot = ep.get("overview") or ep_plot
                        ep_aired = ep.get("air_date") or ep_aired
                        ep_name = ep.get("name", "")
                        if ep_name:
                            ep_title = ep_name
                        break
            except Exception:
                pass  # Fall back to series-level data

        stem = _stem(filename)
        metadata = {
            "title": ep_title,
            "showtitle": detail.get("name", ""),
            "season": season,
            "episode": episode,
            "plot": ep_plot,
            "aired": ep_aired,
            "rating": detail.get("vote_average"),
            "votes": detail.get("vote_count"),
            "directors": [
                p["name"] for p in credits.get("crew", []) if p.get("job") == "Director"
            ][:3],
            "actors": [
                {"name": a["name"], "role": a.get("character", "")}
                for a in credits.get("cast", [])[:10]
            ],
            "uniqueids": {"tmdb": str(tmdb_id)},
        }
        generate_episode_nfo(metadata, out_dir / f"{stem}.nfo")

        # Generate tvshow.nfo once per show (skip if already exists)
        tvshow_nfo_path = out_dir / "tvshow.nfo"
        if not tvshow_nfo_path.exists():
            tvshow_metadata = {
                "title": detail.get("name", ""),
                "originaltitle": detail.get("original_name", ""),
                "showtitle": detail.get("name", ""),
                "year": int((detail.get("first_air_date", "") or "0000")[:4]) or None,
                "plot": detail.get("overview", ""),
                "premiered": detail.get("first_air_date", ""),
                "rating": detail.get("vote_average"),
                "votes": detail.get("vote_count"),
                "status": detail.get("status"),
                "genres": [g["name"] for g in detail.get("genres", [])],
                "studios": [
                    c["name"] for c in detail.get("production_companies", [])
                    if c.get("name")
                ],
                "tags": [
                    k["name"] for k in detail.get("keywords", {}).get("results", [])
                ] if isinstance(detail.get("keywords"), dict) else [],
                "actors": [
                    {
                        "name": a["name"],
                        "role": a.get("character", ""),
                        "thumb": f"https://image.tmdb.org/t/p/w185{a['profile_path']}"
                        if a.get("profile_path")
                        else "",
                    }
                    for a in credits.get("cast", [])[:15]
                ],
                "uniqueids": {"tmdb": str(tmdb_id)},
                "thumb": f"https://image.tmdb.org/t/p/original{detail['poster_path']}"
                if detail.get("poster_path")
                else None,
                "fanart": f"https://image.tmdb.org/t/p/original{detail['backdrop_path']}"
                if detail.get("backdrop_path")
                else None,
            }
            generate_tvshow_nfo(tvshow_metadata, tvshow_nfo_path)

        media_cache.put(
            "file_map",
            _stem(filename),
            {
                "type": "tv",
                "title": metadata["title"],
                "showtitle": metadata["showtitle"],
                "year": (detail.get("first_air_date", "") or "")[:4],
                "season": season,
                "episode": episode,
                "tmdb_id": tmdb_id,
            },
        )

        return {"status": "ok", "match": metadata["title"], "tmdb_id": tmdb_id}
    finally:
        client.close()


def scrape_av(number: str, filename: str, out_dir: Path) -> dict:
    """Scrape AV metadata. Fallback: jav321 → javfree. Caches not_found for 7 days."""
    # Check success cache (permanent)
    cached = media_cache.get("av", number)
    if cached and not cached.get("_not_found"):
        _write_av_nfo(cached, filename, out_dir, cached.get("_source", "cache"))
        media_cache.put(
            "file_map",
            _stem(filename),
            {
                "type": "av",
                "number": number,
                "title": cached.get("title", ""),
            },
        )
        return {"status": "ok", "match": cached.get("title", ""), "number": number}

    # Check not_found cache (7-day TTL)
    if media_cache.is_not_found("av", number):
        return {"status": "not_found", "number": number, "cached": True}

    from media115.scraper.jav321 import fetch_metadata as jav321_fetch
    from media115.scraper.javfree import fetch_metadata as javfree_fetch

    for fetch, source in [(jav321_fetch, "jav321"), (javfree_fetch, "javfree")]:
        meta = fetch(number)
        if meta and meta.get("title"):
            meta["_source"] = source
            media_cache.put("av", number, meta)
            _write_av_nfo(meta, filename, out_dir, source)
            media_cache.put(
                "file_map",
                _stem(filename),
                {"type": "av", "number": number, "title": meta["title"]},
            )
            return {"status": "ok", "match": meta["title"], "number": number}

    # All sources exhausted — cache not_found for 7 days
    media_cache.put_not_found("av", number)
    return {"status": "not_found", "number": number}


# ── Internal helpers ─────────────────────────────────────────────────


def _get_tmdb_movie_full(client, tmdb_id: int) -> tuple[dict, dict, dict]:
    """Get TMDB movie detail + images + credits with cache."""
    cache_key = f"movie_{tmdb_id}"
    cached = media_cache.get("tmdb", cache_key)
    if cached and "credits" in cached:
        return cached["detail"], cached.get("images", {}), cached["credits"]

    detail = client.movie_detail(tmdb_id)
    images = client.movie_images(tmdb_id)
    credits = client.movie_credits(tmdb_id)
    media_cache.put(
        "tmdb", cache_key, {"detail": detail, "images": images, "credits": credits}
    )
    return detail, images, credits


def _get_tmdb_tv_full(client, tmdb_id: int) -> tuple[dict, dict, dict]:
    """Get TMDB TV detail + credits with cache."""
    cache_key = f"tv_{tmdb_id}"
    cached = media_cache.get("tmdb", cache_key)
    if cached and "credits" in cached:
        return cached["detail"], {}, cached["credits"]

    detail = client.tv_detail(tmdb_id)
    credits = client.tv_credits(tmdb_id)
    media_cache.put("tmdb", cache_key, {"detail": detail, "credits": credits})
    return detail, {}, credits


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
        with contextlib.suppress(Exception):
            save_poster(
                images["posters"][0]["file_path"], out_dir, filename="poster.jpg"
            )


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
        with contextlib.suppress(Exception):
            download_image(meta["cover_url"], out_dir / "poster.jpg")

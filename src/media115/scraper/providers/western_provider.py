"""Western adult content providers using ThePornDB and StashDB."""
from __future__ import annotations

import contextlib
from pathlib import Path

from media115.scraper.provider import BaseProvider


class ThePornDBProvider(BaseProvider):
    name = "theporndb"
    supported_types = {"av_west"}
    priority = 0

    def search(self, query, **kwargs):
        from media115.scraper.theporndb import ThePornDBClient
        client = ThePornDBClient()
        return client.search_scene(query)

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.theporndb import ThePornDBClient
        client = ThePornDBClient()
        return client.scene_detail(provider_id)

    def scrape(self, query, filename, out_dir, **kwargs):
        from media115.scraper.theporndb import ThePornDBClient
        from media115.scraper.nfo import generate_movie_nfo
        from media115.scraper.artwork import download_image
        from media115.utils import stem as _stem
        from media115 import cache as media_cache

        client = ThePornDBClient()
        results = client.search_scene(query)
        if not results:
            return {"status": "not_found", "query": query}

        match = results[0]
        scene_id = match.get("id", "")
        title = match.get("title", "")
        date = match.get("date", "")
        year = int(date[:4]) if date and len(date) >= 4 else None

        # Get performers
        performers = match.get("performers", [])
        actors = []
        for p in performers:
            parent = p.get("parent", p)  # TPDB nesting varies
            if isinstance(parent, dict):
                actors.append({"name": parent.get("name", ""), "role": ""})
            elif isinstance(p, dict) and "name" in p:
                actors.append({"name": p["name"], "role": ""})

        studio = ""
        site = match.get("site", {})
        if isinstance(site, dict):
            studio = site.get("name", "")

        metadata = {
            "title": title,
            "year": year,
            "plot": match.get("description", ""),
            "premiered": date,
            "studios": [studio] if studio else [],
            "genres": [t.get("name", "") for t in match.get("tags", []) if isinstance(t, dict)],
            "actors": actors,
            "uniqueids": {"theporndb": str(scene_id)},
        }

        # Generate NFO
        stem = _stem(filename)
        generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")

        # Download poster
        posters = match.get("posters", match.get("background", {}))
        poster_url = ""
        if isinstance(posters, dict):
            poster_url = posters.get("large", posters.get("medium", posters.get("small", "")))
        elif isinstance(posters, list) and posters:
            poster_url = posters[0] if isinstance(posters[0], str) else posters[0].get("url", "")
        if poster_url:
            with contextlib.suppress(Exception):
                download_image(poster_url, out_dir / "poster.jpg")

        # Update file_map
        media_cache.put("file_map", stem, {
            "type": "av",
            "title": title,
            "number": query,
        })

        return {"status": "ok", "match": title, "theporndb_id": scene_id}


class StashDBProvider(BaseProvider):
    name = "stashdb"
    supported_types = {"av_west"}
    priority = 1

    def search(self, query, **kwargs):
        from media115.scraper.stashdb import StashDBClient
        client = StashDBClient()
        return client.search_scenes(query)

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.stashdb import StashDBClient
        client = StashDBClient()
        return client.scene_detail(provider_id)

    def scrape(self, query, filename, out_dir, **kwargs):
        from media115.scraper.stashdb import StashDBClient
        from media115.scraper.nfo import generate_movie_nfo
        from media115.scraper.artwork import download_image
        from media115.utils import stem as _stem
        from media115 import cache as media_cache

        client = StashDBClient()
        results = client.search_scenes(query)
        if not results:
            return {"status": "not_found", "query": query}

        match = results[0]
        scene_id = match.get("id", "")
        title = match.get("title", "")
        date = match.get("date", "")
        year = int(date[:4]) if date and len(date) >= 4 else None

        performers = []
        for p in match.get("performers", []):
            perf = p.get("performer", p)
            if isinstance(perf, dict):
                performers.append({"name": perf.get("name", ""), "role": p.get("as_role", "")})

        studio = ""
        studio_data = match.get("studio")
        if isinstance(studio_data, dict):
            studio = studio_data.get("name", "")

        tags = [t.get("name", "") for t in match.get("tags", []) if isinstance(t, dict)]

        metadata = {
            "title": title,
            "year": year,
            "plot": match.get("details", ""),
            "premiered": date,
            "studios": [studio] if studio else [],
            "genres": tags,
            "actors": performers,
            "uniqueids": {"stashdb": str(scene_id)},
        }

        stem = _stem(filename)
        generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")

        # Download poster from images
        images = match.get("images", [])
        if images:
            poster_url = images[0].get("url", "") if isinstance(images[0], dict) else images[0]
            if poster_url:
                with contextlib.suppress(Exception):
                    download_image(poster_url, out_dir / "poster.jpg")

        media_cache.put("file_map", stem, {
            "type": "av",
            "title": title,
            "number": query,
        })

        return {"status": "ok", "match": title, "stashdb_id": scene_id}

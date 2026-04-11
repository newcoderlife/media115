"""Bangumi provider for anime metadata."""
from __future__ import annotations

import os
from pathlib import Path

from media115.scraper.provider import BaseProvider


class BangumiProvider(BaseProvider):
    name = "bangumi"
    supported_types = {"anime"}
    priority = 0  # Primary for anime

    def search(self, query, **kwargs):
        from media115.scraper.bangumi import BangumiClient
        token = os.environ.get("BANGUMI_ACCESS_TOKEN")
        client = BangumiClient(access_token=token)
        try:
            return client.search(query)
        finally:
            client.close()

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.bangumi import BangumiClient
        token = os.environ.get("BANGUMI_ACCESS_TOKEN")
        client = BangumiClient(access_token=token)
        try:
            return client.subject(int(provider_id))
        finally:
            client.close()

    def scrape(self, query, filename, out_dir, **kwargs):
        # Bangumi doesn't have a dedicated scrape function yet
        # For now, search and generate basic NFO
        # TODO: implement full Bangumi scrape with episode matching
        results = self.search(query)
        if not results:
            return {"status": "not_found", "query": query}

        match = results[0]
        title = match.get("name_cn") or match.get("name", "")
        bgm_id = match.get("id")

        # Generate basic NFO
        from media115.scraper.nfo import generate_episode_nfo
        from media115.utils import stem

        season = kwargs.get("season", 1)
        episode = kwargs.get("episode")

        metadata = {
            "title": title,
            "showtitle": title,
            "season": season,
            "episode": episode,
            "plot": match.get("summary", ""),
            "uniqueids": {"bangumi": str(bgm_id)},
        }

        nfo_path = out_dir / f"{stem(filename)}.nfo"
        generate_episode_nfo(metadata, nfo_path)

        from media115 import cache as media_cache
        media_cache.put("file_map", stem(filename), {
            "type": "tv",
            "title": title,
            "showtitle": title,
            "season": season,
            "episode": episode,
            "bangumi_id": bgm_id,
        })

        return {"status": "ok", "match": title, "bangumi_id": bgm_id}

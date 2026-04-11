"""AV providers wrapping existing jav321/javfree scrapers."""
from __future__ import annotations

from pathlib import Path

from media115.scraper.provider import BaseProvider


class Jav321Provider(BaseProvider):
    name = "jav321"
    supported_types = {"av", "gravure"}
    priority = 0

    def search(self, query, **kwargs):
        from media115.scraper.jav321 import fetch_metadata
        result = fetch_metadata(query)
        return [result] if result else []

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.jav321 import fetch_metadata
        return fetch_metadata(provider_id) or {}

    def scrape(self, query, filename, out_dir, **kwargs):
        from media115.scraper.scrape import scrape_av
        return scrape_av(query, filename, out_dir)


class JavFreeProvider(BaseProvider):
    name = "javfree"
    supported_types = {"av", "gravure"}
    priority = 1

    def search(self, query, **kwargs):
        from media115.scraper.javfree import fetch_metadata
        result = fetch_metadata(query)
        return [result] if result else []

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.javfree import fetch_metadata
        return fetch_metadata(provider_id) or {}

    def scrape(self, query, filename, out_dir, **kwargs):
        from media115.scraper.scrape import scrape_av
        # scrape_av already does jav321 → javfree fallback internally
        # For the registry, we call it only for this specific source
        from media115.scraper.javfree import fetch_metadata
        from media115 import cache as media_cache
        from media115.scraper.scrape import _write_av_nfo
        from media115.utils import stem as _stem

        if media_cache.is_not_found("av", query):
            return {"status": "not_found", "number": query, "cached": True}

        meta = fetch_metadata(query)
        if meta and meta.get("title"):
            meta["_source"] = "javfree"
            media_cache.put("av", query, meta)
            _write_av_nfo(meta, filename, out_dir, "javfree")
            media_cache.put("file_map", _stem(filename),
                            {"type": "av", "number": query, "title": meta["title"]})
            return {"status": "ok", "match": meta["title"], "number": query}

        return {"status": "not_found", "number": query}

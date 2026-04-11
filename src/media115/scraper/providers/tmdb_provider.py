"""TMDB provider for movie and TV metadata."""
from __future__ import annotations

import os
from pathlib import Path

from media115.scraper.provider import BaseProvider


class TMDBMovieProvider(BaseProvider):
    name = "tmdb"
    supported_types = {"movie"}
    priority = 0

    def search(self, query, **kwargs):
        from media115.scraper.tmdb import TMDBClient
        client = TMDBClient(read_access_token=os.environ.get("TMDB_READ_ACCESS_TOKEN", ""))
        try:
            return client.search_movie(query)
        finally:
            client.close()

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.tmdb import TMDBClient
        client = TMDBClient(read_access_token=os.environ.get("TMDB_READ_ACCESS_TOKEN", ""))
        try:
            return client.movie_detail(int(provider_id))
        finally:
            client.close()

    def scrape(self, query, filename, out_dir, **kwargs):
        from media115.scraper.scrape import scrape_movie
        year = kwargs.get("year")
        return scrape_movie(query, year, filename, out_dir)


class TMDBTVProvider(BaseProvider):
    name = "tmdb_tv"
    supported_types = {"tv", "anime"}
    priority = 0  # priority 0 for tv, will be 1 for anime (set in registry)

    def search(self, query, **kwargs):
        from media115.scraper.tmdb import TMDBClient
        client = TMDBClient(read_access_token=os.environ.get("TMDB_READ_ACCESS_TOKEN", ""))
        try:
            return client.search_tv(query)
        finally:
            client.close()

    def get_detail(self, provider_id, **kwargs):
        from media115.scraper.tmdb import TMDBClient
        client = TMDBClient(read_access_token=os.environ.get("TMDB_READ_ACCESS_TOKEN", ""))
        try:
            return client.tv_detail(int(provider_id))
        finally:
            client.close()

    def scrape(self, query, filename, out_dir, **kwargs):
        from media115.scraper.scrape import scrape_tv
        season = kwargs.get("season", 1)
        episode = kwargs.get("episode")
        return scrape_tv(query, season, episode, filename, out_dir)

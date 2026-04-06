"""TMDB API client for movie/TV metadata."""

from __future__ import annotations

import time

import httpx

from media115.cache import _cache_dir
from media115.rate_limit import RateLimiter

BASE_URL = "https://api.themoviedb.org/3"
IMAGE_BASE = "https://image.tmdb.org/t/p"


class TMDBClient:
    def __init__(self, read_access_token: str, language: str = "zh-CN"):
        self._language = language
        self._limiter = RateLimiter(
            qps=0.8, qpm=40,
            state_path=_cache_dir() / "rate_limit_tmdb.json",
        )
        self._http = httpx.Client(
            base_url=BASE_URL,
            headers={"Authorization": f"Bearer {read_access_token}"},
            timeout=15,
        )

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _get(self, path: str, params: dict | None = None) -> dict:
        self._limiter.acquire()
        resp = self._http.get(path, params=params)
        if resp.status_code == 429:
            retry_after = int(resp.headers.get("Retry-After", "5"))
            time.sleep(retry_after)
            self._limiter.acquire()
            resp = self._http.get(path, params=params)
        resp.raise_for_status()
        return resp.json()

    def search_movie(self, query: str, language: str | None = None) -> list[dict]:
        return self._get(
            "/search/movie",
            params={"query": query, "language": language or self._language},
        ).get("results", [])

    def movie_detail(self, movie_id: int, language: str | None = None) -> dict:
        return self._get(
            f"/movie/{movie_id}",
            params={"language": language or self._language},
        )

    def movie_images(self, movie_id: int) -> dict:
        return self._get(
            f"/movie/{movie_id}/images",
            params={"include_image_language": "en,zh,null"},
        )

    def search_tv(self, query: str, language: str | None = None) -> list[dict]:
        return self._get(
            "/search/tv",
            params={"query": query, "language": language or self._language},
        ).get("results", [])

    def tv_detail(self, tv_id: int, language: str | None = None) -> dict:
        return self._get(
            f"/tv/{tv_id}",
            params={"language": language or self._language},
        )

    def season_detail(self, tv_id: int, season_number: int, language: str | None = None) -> dict:
        return self._get(
            f"/tv/{tv_id}/season/{season_number}",
            params={"language": language or self._language},
        )

    def movie_credits(self, movie_id: int, language: str | None = None) -> dict:
        return self._get(
            f"/movie/{movie_id}/credits",
            params={"language": language or self._language},
        )

    def tv_credits(self, tv_id: int, language: str | None = None) -> dict:
        return self._get(
            f"/tv/{tv_id}/credits",
            params={"language": language or self._language},
        )

    @staticmethod
    def image_url(file_path: str, size: str = "original") -> str:
        return f"{IMAGE_BASE}/{size}{file_path}"

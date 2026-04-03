"""TMDB API client for movie/TV metadata."""

import time

import httpx

BASE_URL = "https://api.themoviedb.org/3"
IMAGE_BASE = "https://image.tmdb.org/t/p"
_MIN_INTERVAL = 0.05  # ~20 QPS max (TMDB allows ~40, we stay conservative)


class TMDBClient:
    def __init__(self, read_access_token: str, language: str = "zh-CN"):
        self._language = language
        self._http = httpx.Client(
            base_url=BASE_URL,
            headers={"Authorization": f"Bearer {read_access_token}"},
            timeout=10,
        )
        self._last_request: float = 0

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _get(self, path: str, params: dict | None = None) -> dict:
        elapsed = time.monotonic() - self._last_request
        if elapsed < _MIN_INTERVAL:
            time.sleep(_MIN_INTERVAL - elapsed)
        resp = self._http.get(path, params=params)
        self._last_request = time.monotonic()
        if resp.status_code == 429:
            retry_after = int(resp.headers.get("Retry-After", "2"))
            time.sleep(retry_after)
            resp = self._http.get(path, params=params)
            self._last_request = time.monotonic()
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

    def season_detail(
        self, tv_id: int, season_number: int, language: str | None = None
    ) -> dict:
        return self._get(
            f"/tv/{tv_id}/season/{season_number}",
            params={"language": language or self._language},
        )

    @staticmethod
    def image_url(file_path: str, size: str = "original") -> str:
        return f"{IMAGE_BASE}/{size}{file_path}"

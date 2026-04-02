"""TMDB API client for movie/TV metadata."""

import httpx

BASE_URL = "https://api.themoviedb.org/3"
IMAGE_BASE = "https://image.tmdb.org/t/p"


class TMDBClient:
    def __init__(self, read_access_token: str, language: str = "zh-CN"):
        self._language = language
        self._http = httpx.Client(
            base_url=BASE_URL,
            headers={"Authorization": f"Bearer {read_access_token}"},
            timeout=10,
        )

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def search_movie(self, query: str, language: str | None = None) -> list[dict]:
        resp = self._http.get(
            "/search/movie",
            params={"query": query, "language": language or self._language},
        )
        resp.raise_for_status()
        return resp.json().get("results", [])

    def movie_detail(self, movie_id: int, language: str | None = None) -> dict:
        resp = self._http.get(
            f"/movie/{movie_id}",
            params={"language": language or self._language},
        )
        resp.raise_for_status()
        return resp.json()

    def movie_images(self, movie_id: int) -> dict:
        resp = self._http.get(
            f"/movie/{movie_id}/images",
            params={"include_image_language": "en,zh,null"},
        )
        resp.raise_for_status()
        return resp.json()

    def search_tv(self, query: str, language: str | None = None) -> list[dict]:
        resp = self._http.get(
            "/search/tv",
            params={"query": query, "language": language or self._language},
        )
        resp.raise_for_status()
        return resp.json().get("results", [])

    def tv_detail(self, tv_id: int, language: str | None = None) -> dict:
        resp = self._http.get(
            f"/tv/{tv_id}",
            params={"language": language or self._language},
        )
        resp.raise_for_status()
        return resp.json()

    def season_detail(
        self, tv_id: int, season_number: int, language: str | None = None
    ) -> dict:
        resp = self._http.get(
            f"/tv/{tv_id}/season/{season_number}",
            params={"language": language or self._language},
        )
        resp.raise_for_status()
        return resp.json()

    @staticmethod
    def image_url(file_path: str, size: str = "original") -> str:
        return f"{IMAGE_BASE}/{size}{file_path}"

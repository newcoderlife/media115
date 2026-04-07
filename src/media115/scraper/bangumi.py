"""Bangumi API client for anime metadata."""

from __future__ import annotations

import httpx

from cloud115.rate_limit import RateLimiter

BASE_URL = "https://api.bgm.tv"


class BangumiClient:
    def __init__(self, access_token: str | None = None):
        headers = {"User-Agent": "media115/0.1"}
        if access_token:
            headers["Authorization"] = f"Bearer {access_token}"
        self._limiter = RateLimiter(
            "bangumi", qps=0.8, qpm=40
        )
        self._http = httpx.Client(
            base_url=BASE_URL,
            headers=headers,
            timeout=10,
        )

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def search(self, keyword: str, subject_type: int = 2, limit: int = 10) -> list[dict]:
        from media115.log import get_logger
        get_logger().debug("Bangumi GET /v0/search/subjects")
        self._limiter.acquire()
        resp = self._http.post(
            "/v0/search/subjects",
            json={
                "keyword": keyword,
                "filter": {"type": [subject_type]},
            },
            params={"limit": limit},
        )
        resp.raise_for_status()
        data = resp.json()
        return data.get("data", [])

    def subject(self, subject_id: int) -> dict:
        from media115.log import get_logger
        get_logger().debug("Bangumi GET /v0/subjects/%s", subject_id)
        self._limiter.acquire()
        resp = self._http.get(f"/v0/subjects/{subject_id}")
        resp.raise_for_status()
        return resp.json()

    def episodes(
        self,
        subject_id: int,
        episode_type: int | None = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[dict]:
        from media115.log import get_logger
        get_logger().debug("Bangumi GET /v0/episodes?subject_id=%s", subject_id)
        self._limiter.acquire()
        params: dict = {
            "subject_id": subject_id,
            "limit": limit,
            "offset": offset,
        }
        if episode_type is not None:
            params["type"] = episode_type
        resp = self._http.get("/v0/episodes", params=params)
        resp.raise_for_status()
        return resp.json().get("data", [])

    def subject_persons(self, subject_id: int) -> list[dict]:
        from media115.log import get_logger
        get_logger().debug("Bangumi GET /v0/subjects/%s/persons", subject_id)
        self._limiter.acquire()
        resp = self._http.get(f"/v0/subjects/{subject_id}/persons")
        resp.raise_for_status()
        return resp.json()

"""Bangumi API client for anime metadata."""

import time

import httpx

BASE_URL = "https://api.bgm.tv"
_MIN_INTERVAL = 0.2  # 5 QPS max (conservative, Bangumi has no published limit)


class BangumiClient:
    def __init__(self, access_token: str | None = None):
        headers = {"User-Agent": "media115/0.1"}
        if access_token:
            headers["Authorization"] = f"Bearer {access_token}"
        self._http = httpx.Client(
            base_url=BASE_URL,
            headers=headers,
            timeout=10,
        )
        self._last_request: float = 0

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _throttle(self):
        elapsed = time.monotonic() - self._last_request
        if elapsed < _MIN_INTERVAL:
            time.sleep(_MIN_INTERVAL - elapsed)
        self._last_request = time.monotonic()

    def search(
        self, keyword: str, subject_type: int = 2, limit: int = 10
    ) -> list[dict]:
        self._throttle()
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
        self._throttle()
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
        self._throttle()
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
        self._throttle()
        resp = self._http.get(f"/v0/subjects/{subject_id}/persons")
        resp.raise_for_status()
        return resp.json()

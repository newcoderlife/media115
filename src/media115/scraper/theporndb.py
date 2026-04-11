"""ThePornDB API client for western adult content metadata."""
from __future__ import annotations

import os

from media115.scraper.base import ThrottledClient

BASE_URL = "https://api.theporndb.net"


class ThePornDBClient:
    def __init__(self, token: str | None = None):
        self._token = token or os.environ.get("THEPORNDB_TOKEN", "")
        self._http = ThrottledClient(name="theporndb", qps=1.5, qpm=100)

    def _get(self, path, params=None):
        from media115.log import get_logger
        get_logger().debug("TPDB GET %s", path)
        resp = self._http.get(
            f"{BASE_URL}{path}",
            params=params,
            headers={
                "Authorization": f"Bearer {self._token}",
                "Accept": "application/json",
            },
        )
        resp.raise_for_status()
        return resp.json()

    def search_scene(self, query: str) -> list[dict]:
        """Search scenes by title/filename."""
        data = self._get("/scenes", params={"parse": query})
        return data.get("data", [])

    def scene_detail(self, scene_id: str) -> dict:
        """Get scene details by ID."""
        data = self._get(f"/scenes/{scene_id}")
        return data.get("data", {})

    def search_jav(self, query: str) -> list[dict]:
        """Search JAV by number."""
        data = self._get("/jav", params={"parse": query})
        return data.get("data", [])

    def search_performer(self, name: str) -> list[dict]:
        """Search performers by name."""
        data = self._get("/performers", params={"q": name})
        return data.get("data", [])

    def close(self):
        pass  # ThrottledClient doesn't need explicit close

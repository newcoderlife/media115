"""StashDB GraphQL API client for western adult content metadata."""
from __future__ import annotations

import json
import os

from media115.scraper.base import ThrottledClient

ENDPOINT = "https://stashdb.org/graphql"


class StashDBClient:
    def __init__(self, api_key: str | None = None):
        self._api_key = api_key or os.environ.get("STASHDB_API_KEY", "")
        self._http = ThrottledClient(name="stashdb", qps=1.0, qpm=60)

    def _query(self, query: str, variables: dict | None = None) -> dict:
        from media115.log import get_logger
        get_logger().debug("StashDB GraphQL query")
        resp = self._http.post(
            ENDPOINT,
            headers={
                "ApiKey": self._api_key,
                "Content-Type": "application/json",
                "Accept": "application/json",
            },
            content=json.dumps({"query": query, "variables": variables or {}}).encode(),
        )
        resp.raise_for_status()
        return resp.json().get("data", {})

    def search_scenes(self, term: str, per_page: int = 10) -> list[dict]:
        """Search scenes by title."""
        data = self._query("""
            query ($term: String!, $per_page: Int!) {
                searchScene(term: $term, limit: $per_page) {
                    id
                    title
                    date
                    studio { id name }
                    performers { performer { id name } }
                    urls { url type }
                    images { url }
                }
            }
        """, {"term": term, "per_page": per_page})
        return data.get("searchScene", [])

    def scene_detail(self, scene_id: str) -> dict:
        """Get scene by ID."""
        data = self._query("""
            query ($id: ID!) {
                findScene(id: $id) {
                    id
                    title
                    date
                    details
                    duration
                    studio { id name }
                    performers { performer { id name } as_role }
                    urls { url type }
                    images { url }
                    tags { id name }
                }
            }
        """, {"id": scene_id})
        return data.get("findScene", {})

    def search_performers(self, term: str, per_page: int = 10) -> list[dict]:
        """Search performers by name."""
        data = self._query("""
            query ($term: String!, $per_page: Int!) {
                searchPerformer(term: $term, limit: $per_page) {
                    id
                    name
                    birth_date
                    images { url }
                }
            }
        """, {"term": term, "per_page": per_page})
        return data.get("searchPerformer", [])

    def close(self):
        pass

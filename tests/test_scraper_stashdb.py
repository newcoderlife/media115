"""StashDB GraphQL API client tests. Mocked HTTP, no network."""
from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from media115.scraper.stashdb import StashDBClient, ENDPOINT


@pytest.fixture
def client():
    with patch("media115.scraper.base.httpx.Client"):
        with patch("media115.scraper.base.RateLimiter"):
            c = StashDBClient(api_key="test_api_key")
            yield c


@pytest.fixture
def mock_http(client):
    return client._http._http


class TestStashDBInit:
    def test_init_with_api_key(self):
        with patch("media115.scraper.base.httpx.Client"):
            with patch("media115.scraper.base.RateLimiter"):
                c = StashDBClient(api_key="my_key")
                assert c._api_key == "my_key"

    def test_init_from_env(self, monkeypatch):
        monkeypatch.setenv("STASHDB_API_KEY", "env_key")
        with patch("media115.scraper.base.httpx.Client"):
            with patch("media115.scraper.base.RateLimiter"):
                c = StashDBClient()
                assert c._api_key == "env_key"

    def test_init_empty_key_fallback(self, monkeypatch):
        monkeypatch.delenv("STASHDB_API_KEY", raising=False)
        with patch("media115.scraper.base.httpx.Client"):
            with patch("media115.scraper.base.RateLimiter"):
                c = StashDBClient()
                assert c._api_key == ""


class TestSearchScenes:
    def test_search_scenes_returns_data(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {
                "searchScene": [{"id": "1", "title": "Test Scene"}]
            }
            results = client.search_scenes("test")
            assert len(results) == 1
            assert results[0]["title"] == "Test Scene"

    def test_search_scenes_empty(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {"searchScene": []}
            results = client.search_scenes("nonexistent")
            assert results == []

    def test_search_scenes_missing_key(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {}
            results = client.search_scenes("test")
            assert results == []

    def test_search_scenes_default_per_page(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {"searchScene": []}
            client.search_scenes("test")
            call_variables = mock_query.call_args[0][1]
            assert call_variables["per_page"] == 10

    def test_search_scenes_custom_per_page(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {"searchScene": []}
            client.search_scenes("test", per_page=5)
            call_variables = mock_query.call_args[0][1]
            assert call_variables["per_page"] == 5

    def test_search_scenes_passes_term(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {"searchScene": []}
            client.search_scenes("my search term")
            call_variables = mock_query.call_args[0][1]
            assert call_variables["term"] == "my search term"


class TestSceneDetail:
    def test_scene_detail(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {
                "findScene": {
                    "id": "abc123",
                    "title": "Full Scene",
                    "date": "2024-03-20",
                    "details": "A great scene",
                }
            }
            detail = client.scene_detail("abc123")
            assert detail["id"] == "abc123"
            assert detail["title"] == "Full Scene"
            call_variables = mock_query.call_args[0][1]
            assert call_variables["id"] == "abc123"

    def test_scene_detail_not_found(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {}
            detail = client.scene_detail("bad_id")
            assert detail == {}


class TestSearchPerformers:
    def test_search_performers(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {
                "searchPerformer": [
                    {"id": "p1", "name": "Jane Doe", "birth_date": "1990-01-01"}
                ]
            }
            results = client.search_performers("Jane")
            assert len(results) == 1
            assert results[0]["name"] == "Jane Doe"

    def test_search_performers_empty(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {"searchPerformer": []}
            results = client.search_performers("Unknown")
            assert results == []

    def test_search_performers_passes_term(self, client):
        with patch.object(client, "_query") as mock_query:
            mock_query.return_value = {"searchPerformer": []}
            client.search_performers("Jane Doe", per_page=3)
            call_variables = mock_query.call_args[0][1]
            assert call_variables["term"] == "Jane Doe"
            assert call_variables["per_page"] == 3


class TestQueryMethod:
    def test_query_posts_to_endpoint(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": {"searchScene": []}}
        mock_http.post.return_value = mock_resp

        client._query("query { test }", {"var": "val"})

        url = mock_http.post.call_args.args[0]
        assert url == ENDPOINT

    def test_query_sets_api_key_header(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": {}}
        mock_http.post.return_value = mock_resp

        client._query("query { test }")

        headers = mock_http.post.call_args.kwargs.get("headers", {})
        assert headers["ApiKey"] == "test_api_key"
        assert headers["Content-Type"] == "application/json"
        assert headers["Accept"] == "application/json"

    def test_query_sends_json_body(self, client, mock_http):
        import json as _json
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": {}}
        mock_http.post.return_value = mock_resp

        client._query("query { test }", {"key": "value"})

        content = mock_http.post.call_args.kwargs.get("content", b"")
        body = _json.loads(content)
        assert body["query"] == "query { test }"
        assert body["variables"] == {"key": "value"}

    def test_query_raises_on_http_error(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status.side_effect = Exception("401 Unauthorized")
        mock_http.post.return_value = mock_resp

        with pytest.raises(Exception, match="401 Unauthorized"):
            client._query("query { test }")

    def test_query_returns_data_key(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": {"findScene": {"id": "1"}}}
        mock_http.post.return_value = mock_resp

        result = client._query("query { findScene }")
        assert result == {"findScene": {"id": "1"}}

    def test_query_empty_variables_default(self, client, mock_http):
        import json as _json
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": {}}
        mock_http.post.return_value = mock_resp

        client._query("query { test }")

        content = mock_http.post.call_args.kwargs.get("content", b"")
        body = _json.loads(content)
        assert body["variables"] == {}


class TestEndpointConstant:
    def test_endpoint_value(self):
        assert ENDPOINT == "https://stashdb.org/graphql"


class TestClose:
    def test_close_is_noop(self, client):
        client.close()

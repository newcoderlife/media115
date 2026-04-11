"""ThePornDB API client tests. Mocked HTTP, no network."""
from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from media115.scraper.theporndb import ThePornDBClient


@pytest.fixture
def client():
    with patch("media115.scraper.base.httpx.Client"):
        with patch("media115.scraper.base.RateLimiter"):
            c = ThePornDBClient(token="test_token")
            yield c


@pytest.fixture
def mock_http(client):
    return client._http._http


class TestThePornDBInit:
    def test_init_with_token(self):
        with patch("media115.scraper.base.httpx.Client"):
            with patch("media115.scraper.base.RateLimiter"):
                c = ThePornDBClient(token="my_token")
                assert c._token == "my_token"

    def test_init_from_env(self, monkeypatch):
        monkeypatch.setenv("THEPORNDB_TOKEN", "env_token")
        with patch("media115.scraper.base.httpx.Client"):
            with patch("media115.scraper.base.RateLimiter"):
                c = ThePornDBClient()
                assert c._token == "env_token"

    def test_init_empty_token_fallback(self, monkeypatch):
        monkeypatch.delenv("THEPORNDB_TOKEN", raising=False)
        with patch("media115.scraper.base.httpx.Client"):
            with patch("media115.scraper.base.RateLimiter"):
                c = ThePornDBClient()
                assert c._token == ""


class TestSearchScene:
    def test_search_scene_returns_data(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {"data": [{"id": "1", "title": "Test Scene"}]}
            results = client.search_scene("test")
            assert len(results) == 1
            assert results[0]["title"] == "Test Scene"
            mock_get.assert_called_once_with("/scenes", params={"parse": "test"})

    def test_search_scene_empty(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {"data": []}
            results = client.search_scene("nonexistent")
            assert results == []

    def test_search_scene_missing_data_key(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {}
            results = client.search_scene("test")
            assert results == []

    def test_search_scene_multiple_results(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {
                "data": [
                    {"id": "1", "title": "Scene One"},
                    {"id": "2", "title": "Scene Two"},
                ]
            }
            results = client.search_scene("scene")
            assert len(results) == 2
            assert results[1]["id"] == "2"


class TestSceneDetail:
    def test_scene_detail(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {
                "data": {"id": "42", "title": "Detailed Scene", "date": "2024-01-15"}
            }
            detail = client.scene_detail("42")
            assert detail["id"] == "42"
            assert detail["title"] == "Detailed Scene"
            mock_get.assert_called_once_with("/scenes/42")

    def test_scene_detail_not_found(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {}
            detail = client.scene_detail("999")
            assert detail == {}


class TestSearchJav:
    def test_search_jav(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {"data": [{"id": "abc", "title": "JAV-001"}]}
            results = client.search_jav("JAV-001")
            assert len(results) == 1
            mock_get.assert_called_once_with("/jav", params={"parse": "JAV-001"})


class TestSearchPerformer:
    def test_search_performer(self, client):
        with patch.object(client, "_get") as mock_get:
            mock_get.return_value = {"data": [{"id": "p1", "name": "Jane Doe"}]}
            results = client.search_performer("Jane")
            assert len(results) == 1
            assert results[0]["name"] == "Jane Doe"
            mock_get.assert_called_once_with("/performers", params={"q": "Jane"})


class TestGetMethod:
    def test_get_sets_auth_header(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": []}
        mock_http.get.return_value = mock_resp

        client._get("/scenes", params={"parse": "test"})

        call_kwargs = mock_http.get.call_args
        headers = call_kwargs.kwargs.get("headers", {})
        assert headers["Authorization"] == "Bearer test_token"
        assert headers["Accept"] == "application/json"

    def test_get_constructs_url(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json.return_value = {"data": []}
        mock_http.get.return_value = mock_resp

        client._get("/performers")

        url = mock_http.get.call_args.args[0]
        assert url == "https://api.theporndb.net/performers"

    def test_get_raises_on_http_error(self, client, mock_http):
        mock_resp = MagicMock()
        mock_resp.raise_for_status.side_effect = Exception("404 Not Found")
        mock_http.get.return_value = mock_resp

        with pytest.raises(Exception, match="404 Not Found"):
            client._get("/scenes/bad_id")


class TestClose:
    def test_close_is_noop(self, client):
        # close() should not raise
        client.close()

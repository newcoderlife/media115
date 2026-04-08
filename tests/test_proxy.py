"""strm-proxy tests. Mock CachedClient and Jellyfin."""

from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest
from fastapi.testclient import TestClient

from media115.proxy import create_app


@pytest.fixture
def mock_client():
    return MagicMock()


@pytest.fixture
def app(mock_client):
    return create_app(
        jellyfin_url="http://jellyfin:8096",
        client=mock_client,
    )


@pytest.fixture
def client(app):
    return TestClient(app)


def _mock_async_response(json_data=None, status_code=200, content=b"", headers=None):
    resp = MagicMock(spec=httpx.Response)
    resp.status_code = status_code
    resp.headers = headers or {}
    resp.content = content
    resp.raise_for_status = MagicMock()
    if json_data is not None:
        resp.json.return_value = json_data
    return resp


class TestPlayEndpoint:
    def test_302_redirect(self, client, app):
        app.state.client.download_url.return_value = "https://cdn.115.com/video.mkv"
        resp = client.get("/play/abc123", follow_redirects=False)
        assert resp.status_code == 302
        assert resp.headers["location"] == "https://cdn.115.com/video.mkv"

    def test_no_caching(self, client, app):
        """download_url is called every time — no in-memory cache."""
        app.state.client.download_url.return_value = "https://cdn.115.com/video.mkv"
        client.get("/play/abc123", follow_redirects=False)
        client.get("/play/abc123", follow_redirects=False)
        assert app.state.client.download_url.call_count == 2

    def test_different_pickcode(self, client, app):
        app.state.client.download_url.return_value = "https://cdn.115.com/a.mkv"
        client.get("/play/aaa", follow_redirects=False)
        app.state.client.download_url.return_value = "https://cdn.115.com/b.mkv"
        client.get("/play/bbb", follow_redirects=False)
        assert app.state.client.download_url.call_count == 2

    def test_ua_forwarded(self, client, app):
        """User-Agent from client request is forwarded to download_url."""
        app.state.client.download_url.return_value = "https://cdn.115.com/video.mkv"
        client.get(
            "/play/abc123",
            follow_redirects=False,
            headers={"User-Agent": "VLC/3.0.20"},
        )
        call_kwargs = app.state.client.download_url.call_args
        assert call_kwargs.kwargs.get("user_agent") == "VLC/3.0.20"


class TestVideoStreamIntercept:
    def test_strm_item_redirects(self, client, app):
        """When Jellyfin item is .strm, should 302 to 115 CDN."""
        jellyfin_item = {
            "Items": [
                {
                    "Path": "/media/movies/test.strm",
                    "MediaSources": [{"Id": "src1", "Path": "http://proxy:9000/play/xyz789"}],
                }
            ]
        }
        app.state.client.download_url.return_value = "https://cdn.115.com/real.mkv"
        app.state.http = AsyncMock(spec=httpx.AsyncClient)
        app.state.http.get.return_value = _mock_async_response(json_data=jellyfin_item)

        resp = client.get(
            "/Videos/abc/stream?mediasourceid=src1&api_key=test",
            follow_redirects=False,
        )
        assert resp.status_code == 302

    def test_strm_item_ua_forwarded(self, client, app):
        """User-Agent is forwarded to download_url for strm items."""
        jellyfin_item = {
            "Items": [
                {
                    "Path": "/media/movies/test.strm",
                    "MediaSources": [{"Id": "src1", "Path": "http://proxy:9000/play/xyz789"}],
                }
            ]
        }
        app.state.client.download_url.return_value = "https://cdn.115.com/real.mkv"
        app.state.http = AsyncMock(spec=httpx.AsyncClient)
        app.state.http.get.return_value = _mock_async_response(json_data=jellyfin_item)

        client.get(
            "/Videos/abc/stream?mediasourceid=src1&api_key=test",
            follow_redirects=False,
            headers={"User-Agent": "Jellyfin/10.8"},
        )
        call_kwargs = app.state.client.download_url.call_args
        assert call_kwargs.kwargs.get("user_agent") == "Jellyfin/10.8"

    def test_non_strm_item_proxies(self, client, app):
        """When Jellyfin item is local file, should proxy to Jellyfin."""
        jellyfin_item = {
            "Items": [
                {
                    "Path": "/media/movies/local.mkv",
                    "MediaSources": [{"Id": "src1", "Path": "/media/movies/local.mkv"}],
                }
            ]
        }
        app.state.http = AsyncMock(spec=httpx.AsyncClient)
        app.state.http.get.return_value = _mock_async_response(json_data=jellyfin_item)
        app.state.http.request.return_value = _mock_async_response(
            status_code=200,
            content=b"fake video bytes",
            headers={"content-type": "video/mp4"},
        )

        resp = client.get(
            "/Videos/abc/stream?mediasourceid=src1&api_key=test",
            follow_redirects=False,
        )
        assert resp.status_code == 200


class TestPassthrough:
    def test_other_requests_proxy_to_jellyfin(self, client, app):
        app.state.http = AsyncMock(spec=httpx.AsyncClient)
        app.state.http.request.return_value = _mock_async_response(
            status_code=200,
            content=b'{"Items": []}',
            headers={"content-type": "application/json"},
        )

        resp = client.get("/Items?api_key=test")
        assert resp.status_code == 200


class TestRedirectByPath:
    """Tests for /redirect/{file_path} endpoint."""

    def test_redirect_success(self):
        """stream_url resolves path and returns CDN URL, verify 302."""
        mock = MagicMock()
        mock.stream_url.return_value = "https://cdn.115.com/test.mkv"

        app = create_app(jellyfin_url="http://jellyfin:8096", client=mock)
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/Test Movie (2024)/Test Movie (2024).mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 302
        assert resp.headers["location"] == "https://cdn.115.com/test.mkv"
        mock.stream_url.assert_called_once()

    def test_redirect_not_found(self):
        """When stream_url raises FileNotFoundError, should return 404."""
        mock = MagicMock()
        mock.stream_url.side_effect = FileNotFoundError("File not found: /影音/电影/Missing/missing.mkv")

        app = create_app(jellyfin_url="http://jellyfin:8096", client=mock)
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/Missing/missing.mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 404

    def test_redirect_exception(self):
        """When stream_url raises, should return 500."""
        mock = MagicMock()
        mock.stream_url.side_effect = Exception("connection refused")

        app = create_app(jellyfin_url="http://jellyfin:8096", client=mock)
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/SomeDir/file.mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 500
        assert "connection refused" in resp.text

    def test_redirect_ua_forwarded(self):
        """User-Agent is forwarded to stream_url."""
        mock = MagicMock()
        mock.stream_url.return_value = "https://cdn.115.com/test.mkv"

        app = create_app(jellyfin_url="http://jellyfin:8096", client=mock)
        tc = TestClient(app)

        tc.get(
            "/redirect/影音/电影/movie.mkv",
            follow_redirects=False,
            headers={"User-Agent": "Infuse/7.0"},
        )

        call_kwargs = mock.stream_url.call_args
        assert call_kwargs.kwargs.get("user_agent") == "Infuse/7.0"

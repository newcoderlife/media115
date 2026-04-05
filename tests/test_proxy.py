"""strm-proxy tests. Mock 115 API and Jellyfin."""

from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest
from fastapi.testclient import TestClient

from media115.proxy import create_app


@pytest.fixture
def app():
    return create_app(
        jellyfin_url="http://jellyfin:8096",
        cloud115_client=MagicMock(),
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
        app.state.cloud115.download_url.return_value = "https://cdn.115.com/video.mkv"
        resp = client.get("/play/abc123", follow_redirects=False)
        assert resp.status_code == 302
        assert resp.headers["location"] == "https://cdn.115.com/video.mkv"

    def test_caches_url(self, client, app):
        app.state.cloud115.download_url.return_value = "https://cdn.115.com/video.mkv"
        client.get("/play/abc123", follow_redirects=False)
        client.get("/play/abc123", follow_redirects=False)
        assert app.state.cloud115.download_url.call_count == 1

    def test_different_pickcode_not_cached(self, client, app):
        app.state.cloud115.download_url.return_value = "https://cdn.115.com/a.mkv"
        client.get("/play/aaa", follow_redirects=False)
        app.state.cloud115.download_url.return_value = "https://cdn.115.com/b.mkv"
        client.get("/play/bbb", follow_redirects=False)
        assert app.state.cloud115.download_url.call_count == 2


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
        app.state.cloud115.download_url.return_value = "https://cdn.115.com/real.mkv"
        app.state.http = AsyncMock(spec=httpx.AsyncClient)
        app.state.http.get.return_value = _mock_async_response(json_data=jellyfin_item)

        resp = client.get(
            "/Videos/abc/stream?mediasourceid=src1&api_key=test",
            follow_redirects=False,
        )
        assert resp.status_code == 302

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
        """Mock client resolves path -> pick_code -> download URL, verify 302."""
        cloud_client = MagicMock()
        cloud_client.get_dir_id.return_value = "dir_123"
        cloud_client.list_files_all.return_value = [
            {"n": "Test Movie (2024).mkv", "fid": "fid_1", "pc": "pick_abc"},
        ]
        cloud_client.download_url.return_value = "https://cdn.115.com/test.mkv"

        app = create_app(
            jellyfin_url="http://jellyfin:8096",
            cloud115_client=cloud_client,
        )
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/Test Movie (2024)/Test Movie (2024).mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 302
        assert resp.headers["location"] == "https://cdn.115.com/test.mkv"
        cloud_client.get_dir_id.assert_called_once()
        cloud_client.download_url.assert_called_once_with("pick_abc")

    def test_redirect_not_found(self):
        """When get_dir_id returns None, should return 404."""
        cloud_client = MagicMock()
        cloud_client.get_dir_id.return_value = None

        app = create_app(
            jellyfin_url="http://jellyfin:8096",
            cloud115_client=cloud_client,
        )
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/Missing/missing.mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 404

    def test_redirect_file_not_in_listing(self):
        """Directory exists but file not found in listing, should 404."""
        cloud_client = MagicMock()
        cloud_client.get_dir_id.return_value = "dir_123"
        cloud_client.list_files_all.return_value = [
            {"n": "other_file.mkv", "fid": "fid_other", "pc": "pick_other"},
        ]

        app = create_app(
            jellyfin_url="http://jellyfin:8096",
            cloud115_client=cloud_client,
        )
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/SomeDir/missing.mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 404

    def test_redirect_caches(self):
        """Call twice with same path, verify client only called once (caching)."""
        cloud_client = MagicMock()
        cloud_client.get_dir_id.return_value = "dir_123"
        cloud_client.list_files_all.return_value = [
            {"n": "cached.mkv", "fid": "fid_1", "pc": "pick_cached"},
        ]
        cloud_client.download_url.return_value = "https://cdn.115.com/cached.mkv"

        app = create_app(
            jellyfin_url="http://jellyfin:8096",
            cloud115_client=cloud_client,
        )
        tc = TestClient(app)

        # First request -- resolves path, populates cache
        resp1 = tc.get(
            "/redirect/影音/电影/CacheDir/cached.mkv",
            follow_redirects=False,
        )
        assert resp1.status_code == 302

        # Second request -- should use cached pick_code
        resp2 = tc.get(
            "/redirect/影音/电影/CacheDir/cached.mkv",
            follow_redirects=False,
        )
        assert resp2.status_code == 302

        # get_dir_id and list_files_all should only be called once
        assert cloud_client.get_dir_id.call_count == 1
        assert cloud_client.list_files_all.call_count == 1
        # download_url may be called once (URL also cached by _get_cached_url)
        assert cloud_client.download_url.call_count == 1

    def test_redirect_no_pick_code(self):
        """File found but has no pick_code, should 404."""
        cloud_client = MagicMock()
        cloud_client.get_dir_id.return_value = "dir_123"
        cloud_client.list_files_all.return_value = [
            {"n": "no_pc.mkv", "fid": "fid_1", "pc": ""},
        ]

        app = create_app(
            jellyfin_url="http://jellyfin:8096",
            cloud115_client=cloud_client,
        )
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/SomeDir/no_pc.mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 404
        assert "No pick_code" in resp.text

    def test_redirect_exception(self):
        """When client raises, should return 500."""
        cloud_client = MagicMock()
        cloud_client.get_dir_id.side_effect = Exception("connection refused")

        app = create_app(
            jellyfin_url="http://jellyfin:8096",
            cloud115_client=cloud_client,
        )
        tc = TestClient(app)

        resp = tc.get(
            "/redirect/影音/电影/SomeDir/file.mkv",
            follow_redirects=False,
        )

        assert resp.status_code == 500
        assert "connection refused" in resp.text

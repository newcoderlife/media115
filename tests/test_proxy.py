"""strm-proxy tests. Mock 115 API and Jellyfin."""

import pytest
from unittest.mock import patch, MagicMock
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
        # Should only call download_url once (cached)
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
        # Mock Jellyfin API response
        jellyfin_item = {
            "Items": [
                {
                    "Path": "/media/movies/test.strm",
                    "MediaSources": [
                        {"Id": "src1", "Path": "http://proxy:9000/play/xyz789"}
                    ],
                }
            ]
        }
        app.state.cloud115.download_url.return_value = "https://cdn.115.com/real.mkv"

        with patch("media115.proxy.httpx") as mock_httpx:
            mock_resp = MagicMock()
            mock_resp.json.return_value = jellyfin_item
            mock_resp.raise_for_status = MagicMock()
            mock_httpx.get.return_value = mock_resp

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
        with patch("media115.proxy.httpx") as mock_httpx:
            # Jellyfin API call for item query
            mock_item_resp = MagicMock()
            mock_item_resp.json.return_value = jellyfin_item
            mock_item_resp.raise_for_status = MagicMock()

            # Jellyfin proxy response for the actual video
            mock_proxy_resp = MagicMock()
            mock_proxy_resp.status_code = 200
            mock_proxy_resp.headers = {"content-type": "video/mp4"}
            mock_proxy_resp.content = b"fake video bytes"
            mock_proxy_resp.raise_for_status = MagicMock()

            mock_httpx.get.side_effect = [mock_item_resp]
            mock_httpx.request.return_value = mock_proxy_resp

            resp = client.get(
                "/Videos/abc/stream?mediasourceid=src1&api_key=test",
                follow_redirects=False,
            )
            # Should proxy (200), not redirect (302)
            assert resp.status_code == 200


class TestPassthrough:
    def test_other_requests_proxy_to_jellyfin(self, client):
        with patch("media115.proxy.httpx") as mock_httpx:
            mock_resp = MagicMock()
            mock_resp.status_code = 200
            mock_resp.headers = {"content-type": "application/json"}
            mock_resp.content = b'{"Items": []}'
            mock_resp.raise_for_status = MagicMock()
            mock_httpx.request.return_value = mock_resp

            resp = client.get("/Items?api_key=test")
            assert resp.status_code == 200

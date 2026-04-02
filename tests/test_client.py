"""115 OpenAPI client tests. All mocked (API not yet approved)."""

import pytest
from unittest.mock import patch, MagicMock

from media115.client import Cloud115Client, RateLimiter


class TestRateLimiter:
    def test_allows_within_qps(self):
        limiter = RateLimiter(qps=10, qpm=600, qph=36000)
        for _ in range(10):
            limiter.acquire()  # should not raise or block significantly

    def test_tracks_request_count(self):
        limiter = RateLimiter(qps=100, qpm=600, qph=36000)
        for _ in range(5):
            limiter.acquire()
        assert limiter.request_count >= 5


class TestCloud115Client:
    @pytest.fixture
    def mock_client(self):
        return Cloud115Client(
            app_id="test_app",
            app_secret="test_secret",
            access_token="test_token",
            refresh_token="test_refresh",
        )

    def test_init(self, mock_client):
        assert mock_client._access_token == "test_token"

    def test_list_files(self, mock_client):
        mock_response = {
            "state": True,
            "data": [
                {
                    "fid": "1",
                    "fn": "movie.mkv",
                    "pc": "abc123",
                    "sha": "deadbeef",
                    "s": 1024,
                },
                {"fid": "2", "fn": "sub", "pc": "", "sha": "", "s": 0, "fc": "1"},
            ],
        }
        with patch.object(mock_client, "_request", return_value=mock_response):
            files = mock_client.list_files(dir_id="0")
            assert len(files) == 2
            assert files[0]["fn"] == "movie.mkv"

    def test_download_url(self, mock_client):
        mock_response = {
            "state": True,
            "data": {
                "abc123": {"url": {"url": "https://cdn.115.com/download/abc123.mkv"}},
            },
        }
        with patch.object(mock_client, "_request", return_value=mock_response):
            url = mock_client.download_url("abc123")
            assert "cdn.115.com" in url

    def test_mkdir(self, mock_client):
        mock_response = {"state": True, "data": {"cid": "12345", "cname": "test_dir"}}
        with patch.object(mock_client, "_request", return_value=mock_response):
            result = mock_client.mkdir(parent_id="0", name="test_dir")
            assert result["cid"] == "12345"

    def test_rapid_upload_success(self, mock_client):
        mock_response = {"state": True, "status": 2, "data": {"pick_code": "xyz789"}}
        with patch.object(mock_client, "_request", return_value=mock_response):
            result = mock_client.rapid_upload(
                dir_id="0",
                filename="movie.mkv",
                file_size=1024,
                sha1="aabbccdd",
                pre_sha1="11223344",
            )
            assert result["status"] == 2
            assert result["data"]["pick_code"] == "xyz789"

    def test_rapid_upload_not_found(self, mock_client):
        mock_response = {"state": True, "status": 1}
        with patch.object(mock_client, "_request", return_value=mock_response):
            result = mock_client.rapid_upload(
                dir_id="0",
                filename="new.mkv",
                file_size=1024,
                sha1="aabbccdd",
                pre_sha1="11223344",
            )
            assert result["status"] == 1  # not a rapid upload

    def test_token_refresh(self, mock_client):
        refresh_response = {
            "state": True,
            "data": {
                "access_token": "new_token",
                "refresh_token": "new_refresh",
                "expires_in": 7200,
            },
        }
        with patch.object(mock_client._http, "post") as mock_post:
            mock_resp = MagicMock()
            mock_resp.json.return_value = refresh_response
            mock_resp.raise_for_status = MagicMock()
            mock_post.return_value = mock_resp

            mock_client.refresh_access_token()
            assert mock_client._access_token == "new_token"
            assert mock_client._refresh_token == "new_refresh"

    def test_request_adds_auth_header(self, mock_client):
        with patch.object(mock_client._http, "request") as mock_req:
            mock_resp = MagicMock()
            mock_resp.json.return_value = {"state": True}
            mock_resp.raise_for_status = MagicMock()
            mock_req.return_value = mock_resp

            mock_client._request("GET", "/open/ufile/files", params={"cid": "0"})
            call_kwargs = mock_req.call_args
            assert "Authorization" in call_kwargs.kwargs.get("headers", {})

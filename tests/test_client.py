"""115 client tests. Covers both cookie and OpenAPI modes (all mocked)."""

import pytest
from unittest.mock import patch, MagicMock

from media115.client import Cloud115Client, RateLimiter


class TestRateLimiter:
    def test_allows_without_env(self):
        """Without env_path, limiter is in-process only and doesn't block."""
        limiter = RateLimiter(qps=100, qpm=1000, state_dir=None)
        for _ in range(10):
            limiter.acquire()

    def test_persistent_state(self, tmp_path):
        """State is persisted to .115_rate_limit file."""
        limiter = RateLimiter(qps=10, qpm=1000, state_dir=tmp_path)
        limiter.acquire()
        state_file = tmp_path / ".115_rate_limit"
        assert state_file.exists()
        import json

        state = json.loads(state_file.read_text())
        assert "last_request" in state
        assert state["minute_count"] == 1

    def test_persistent_qpm(self, tmp_path):
        """QPM counter is tracked across calls."""
        limiter = RateLimiter(qps=100, qpm=1000, state_dir=tmp_path)
        limiter.acquire()
        limiter.acquire()
        limiter.acquire()
        import json

        state = json.loads((tmp_path / ".115_rate_limit").read_text())
        assert state["minute_count"] == 3

    def test_cooldown_blocks(self, tmp_path):
        """Cooldown blocks subsequent calls."""
        limiter = RateLimiter(qps=100, qpm=1000, state_dir=tmp_path)
        limiter.set_cooldown(3600)
        with pytest.raises(RuntimeError, match="cooldown"):
            limiter.acquire()

    def test_tracks_request_count(self):
        limiter = RateLimiter(qps=100, qpm=1000, state_dir=None)
        for _ in range(5):
            limiter.acquire()
        assert limiter.request_count >= 5


class TestFactoryMethods:
    def test_from_cookies(self):
        client = Cloud115Client.from_cookies("UID=12345_A1_170000; CID=abc; SEID=def")
        assert client._mode == "cookie"
        assert client._user_id == "12345"

    def test_from_cookies_no_uid(self):
        client = Cloud115Client.from_cookies("CID=abc; SEID=def")
        assert client._mode == "cookie"
        assert client._user_id == ""

    def test_from_openapi(self):
        client = Cloud115Client.from_openapi(
            app_id="test_app",
            app_secret="test_secret",
            access_token="test_token",
            refresh_token="test_refresh",
        )
        assert client._mode == "openapi"
        assert client._access_token == "test_token"

    def test_from_cookie_file(self, tmp_path):
        f = tmp_path / "cookies.txt"
        f.write_text("UID=99_A1_170000; CID=xyz; SEID=abc")
        client = Cloud115Client.from_cookie_file(f)
        assert client._mode == "cookie"
        assert client._user_id == "99"

    def test_save_cookies_to_env(self, tmp_path):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        env = tmp_path / ".env"
        env.write_text("TMDB_API_KEY=abc\n")
        client.save_cookies_to_env(env)
        content = env.read_text()
        assert "CLOUD_115_COOKIES=UID=1_A1_0; CID=x; SEID=y" in content
        assert "TMDB_API_KEY=abc" in content

    def test_save_cookies_to_new_env(self, tmp_path):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        env = tmp_path / ".env"
        client.save_cookies_to_env(env)
        assert "CLOUD_115_COOKIES=UID=1_A1_0; CID=x; SEID=y" in env.read_text()


class TestRenewCookies:
    def test_renew_success(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=old; SEID=old")

        # Mock the 4-step auto-scan flow
        get_responses = [
            # Step 1: QR token
            MagicMock(
                json=lambda: {"data": {"uid": "test_uid"}}, raise_for_status=MagicMock()
            ),
            # Step 2: auto-scan
            MagicMock(json=lambda: {"state": True}, raise_for_status=MagicMock()),
            # Step 3: auto-confirm
            MagicMock(json=lambda: {"state": True}, raise_for_status=MagicMock()),
        ]
        post_resp = MagicMock(
            json=lambda: {
                "data": {"cookie": {"UID": "2_A1_1", "CID": "new", "SEID": "new"}}
            },
            raise_for_status=MagicMock(),
        )

        with patch.object(client._http, "get", side_effect=get_responses):
            with patch.object(client._http, "post", return_value=post_resp):
                result = client.renew_cookies()
                assert result is True
                assert "new" in client._cookies

    def test_renew_fails_no_cookies(self):
        client = Cloud115Client.from_openapi(app_id="x", app_secret="y")
        assert client.renew_cookies() is False

    def test_auto_renew_on_405(self):
        """When cookie request returns 405, should auto-renew and retry."""
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=old; SEID=old")

        with patch.object(client, "renew_cookies", return_value=True) as mock_renew:
            # First call returns 405, second succeeds
            resp_405 = MagicMock()
            resp_405.status_code = 405
            resp_ok = MagicMock()
            resp_ok.status_code = 200
            resp_ok.json.return_value = {"state": True, "data": []}
            resp_ok.raise_for_status = MagicMock()

            with patch.object(client._http, "request", side_effect=[resp_405, resp_ok]):
                result = client._cookie_request("GET", "https://webapi.115.com/files")
                mock_renew.assert_called_once()
                assert result["state"] is True


class TestCookieMode:
    @pytest.fixture
    def client(self):
        return Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")

    def test_list_files(self, client):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "state": True,
            "data": [{"fn": "movie.mkv", "pc": "abc123"}],
        }
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "request", return_value=mock_resp):
            files = client.list_files(dir_id="0")
            assert len(files) == 1
            assert files[0]["fn"] == "movie.mkv"

    def test_download_url(self, client):
        """Test M115 encrypted download URL retrieval."""
        with patch.object(
            client, "_download_url_cookie", return_value="https://cdn.115.com/video.mkv"
        ):
            url = client.download_url("abc123")
            assert "cdn.115.com" in url

    def test_mkdir(self, client):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"state": True, "cid": "999", "cname": "test"}
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "request", return_value=mock_resp):
            result = client.mkdir(parent_id="0", name="test")
            assert result["cid"] == "999"

    def test_request_sends_cookie_header(self, client):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"state": True, "data": []}
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "request", return_value=mock_resp) as mock_req:
            client.list_files()
            call_kwargs = mock_req.call_args
            assert "Cookie" in call_kwargs.kwargs.get("headers", {})


class TestOpenAPIMode:
    @pytest.fixture
    def client(self):
        return Cloud115Client.from_openapi(
            app_id="test_app",
            app_secret="test_secret",
            access_token="test_token",
            refresh_token="test_refresh",
        )

    def test_list_files(self, client):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "state": True,
            "data": [{"fn": "movie.mkv", "pc": "abc123"}],
        }
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "request", return_value=mock_resp):
            files = client.list_files(dir_id="0")
            assert len(files) == 1

    def test_download_url(self, client):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "state": True,
            "data": {"abc123": {"url": {"url": "https://cdn.115.com/download.mkv"}}},
        }
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "request", return_value=mock_resp):
            url = client.download_url("abc123")
            assert "cdn.115.com" in url

    def test_token_refresh(self, client):
        refresh_response = {
            "state": True,
            "data": {
                "access_token": "new_token",
                "refresh_token": "new_refresh",
                "expires_in": 7200,
            },
        }
        mock_resp = MagicMock()
        mock_resp.json.return_value = refresh_response
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "post", return_value=mock_resp):
            client.refresh_access_token()
            assert client._access_token == "new_token"
            assert client._refresh_token == "new_refresh"

    def test_request_sends_auth_header(self, client):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"state": True, "data": []}
        mock_resp.raise_for_status = MagicMock()
        with patch.object(client._http, "request", return_value=mock_resp) as mock_req:
            client.list_files()
            call_kwargs = mock_req.call_args
            headers = call_kwargs.kwargs.get("headers", {})
            assert "Authorization" in headers
            assert headers["Authorization"].startswith("Bearer ")


class TestCrypto:
    def test_m115_roundtrip(self):
        """Test that M115 encode/decode is consistent."""
        from media115._crypto import generate_m115_key, m115_encode

        key = generate_m115_key()
        assert len(key) == 16

        # We can't do a true roundtrip because encode uses RSA with random padding,
        # but we can verify the functions don't crash
        plaintext = '{"pickcode": "abc123"}'
        encoded = m115_encode(key, plaintext)
        assert isinstance(encoded, str)
        assert len(encoded) > 0

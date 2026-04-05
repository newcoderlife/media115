"""115 client tests. Covers both cookie and OpenAPI modes (all mocked)."""

from unittest.mock import MagicMock, patch

import pytest

from media115.client import Cloud115Client, RateLimiter


class TestRateLimiter:
    def test_allows_without_env(self):
        """Without env_path, limiter is in-process only and doesn't block."""
        limiter = RateLimiter(qps=100, qpm=1000, use_state=False)
        for _ in range(10):
            limiter.acquire()

    def test_persistent_state(self, tmp_path):
        """State is persisted to state file."""
        import json

        state_file = tmp_path / "rate_limit.json"
        limiter = RateLimiter(qps=10, qpm=1000, use_state=False)
        limiter._state_path = state_file  # Override for test
        limiter.acquire()
        assert state_file.exists()
        state = json.loads(state_file.read_text())
        assert "last_request" in state
        assert state["minute_count"] == 1

    def test_persistent_qpm(self, tmp_path):
        """QPM counter is tracked across calls."""
        import json

        state_file = tmp_path / "rate_limit.json"
        limiter = RateLimiter(qps=100, qpm=1000, use_state=False)
        limiter._state_path = state_file
        limiter.acquire()
        limiter.acquire()
        limiter.acquire()
        state = json.loads(state_file.read_text())
        assert state["minute_count"] == 3

    def test_cooldown_blocks(self, tmp_path):
        """Cooldown blocks subsequent calls."""
        state_file = tmp_path / "rate_limit.json"
        limiter = RateLimiter(qps=100, qpm=1000, use_state=False)
        limiter._state_path = state_file
        limiter.set_cooldown(3600)
        with pytest.raises(RuntimeError, match="cooldown"):
            limiter.acquire()

    def test_tracks_request_count(self):
        limiter = RateLimiter(qps=100, qpm=1000, use_state=False)
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


class TestCookieModeAPIs:
    """Test individual API methods in cookie mode with mocked HTTP."""

    @pytest.fixture
    def client(self):
        c = Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        c._limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        c._download_limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        return c

    def _mock_resp(self, json_data, status_code=200):
        resp = MagicMock()
        resp.status_code = status_code
        resp.json.return_value = json_data
        resp.raise_for_status = MagicMock()
        return resp

    def test_list_files(self, client):
        data = {
            "state": True,
            "data": [
                {"n": "movie.mkv", "fid": "100", "pc": "abc"},
                {"n": "show.mp4", "fid": "101", "pc": "def"},
            ],
        }
        with patch.object(client._http, "request", return_value=self._mock_resp(data)):
            files = client.list_files(dir_id="42", limit=50, offset=0)
            assert len(files) == 2
            assert files[0]["n"] == "movie.mkv"
            assert files[1]["fid"] == "101"

    def test_list_files_params(self, client):
        """Verify correct parameters are sent to the API."""
        with patch.object(
            client._http,
            "request",
            return_value=self._mock_resp({"state": True, "data": []}),
        ) as mock_req:
            client.list_files(dir_id="55", limit=200, offset=10)
            call_args = mock_req.call_args
            params = call_args.kwargs.get("params", {})
            assert params["cid"] == "55"
            assert params["limit"] == 200
            assert params["offset"] == 10

    def test_get_dir_id(self, client):
        resp_data = {"state": True, "id": 12345}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            dir_id = client.get_dir_id("/movies/action")
            assert dir_id == "12345"

    def test_get_dir_id_not_found(self, client):
        resp_data = {"state": False}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            dir_id = client.get_dir_id("/nonexistent")
            assert dir_id is None

    def test_mkdir(self, client):
        resp_data = {"state": True, "cid": "777", "cname": "NewFolder"}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.mkdir(parent_id="0", name="NewFolder")
            assert result["cid"] == "777"
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["pid"] == "0"
            assert data["cname"] == "NewFolder"

    def test_batch_rename(self, client):
        renames = {"100": "new_movie.mkv", "101": "new_show.mp4"}
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.batch_rename(renames)
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["files_new_name[100]"] == "new_movie.mkv"
            assert data["files_new_name[101]"] == "new_show.mp4"

    def test_delete(self, client):
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.delete(["200", "201"])
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["fid[0]"] == "200"
            assert data["fid[1]"] == "201"

    def test_move(self, client):
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.move(["300", "301"], target_dir_id="500")
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["pid"] == "500"
            assert data["fid[0]"] == "300"
            assert data["fid[1]"] == "301"

    def test_upload_file(self, client, tmp_path):
        nfo_file = tmp_path / "movie.nfo"
        nfo_file.write_text("<movie><title>Test</title></movie>")

        init_resp = self._mock_resp({
            "host": "https://oss.example.com/upload",
            "object": "obj_key",
            "accessid": "ak123",
            "policy": "pol",
            "signature": "sig",
            "callback": "cb",
        })
        oss_resp = self._mock_resp({"data": {"file_id": "999", "file_name": "movie.nfo"}})

        with patch.object(client._http, "post", return_value=init_resp):
            with patch("media115.client.httpx.post", return_value=oss_resp) as mock_oss:
                result = client.upload_file(nfo_file, target_dir_id="42")
                assert result["file_id"] == "999"
                # Verify OSS upload was called with correct host
                mock_oss.assert_called_once()
                oss_call_args = mock_oss.call_args
                assert oss_call_args.args[0] == "https://oss.example.com/upload"

    def test_download_url(self, client):
        """Test download_url by mocking _download_url_cookie."""
        with patch.object(
            client,
            "_download_url_cookie",
            return_value="https://cdn.115.com/files/video.mkv",
        ):
            url = client.download_url("pc123")
            assert url == "https://cdn.115.com/files/video.mkv"

    def test_download_url_cookie_internal(self, client):
        """Test _download_url_cookie with mocked M115 crypto."""
        import json as _json

        fake_key = b"\x00" * 16
        fake_decrypted = _json.dumps(
            {"abc": {"url": {"url": "https://cdn.115.com/real_video.mkv"}}}
        )

        mock_post_resp = self._mock_resp({"data": "encrypted_blob"})

        with patch("media115.client.generate_m115_key", return_value=fake_key):
            with patch("media115.client.m115_encode", return_value="encoded_data"):
                with patch("media115.client.m115_decode", return_value=fake_decrypted):
                    with patch.object(client._http, "post", return_value=mock_post_resp):
                        url = client._download_url_cookie("pc_test")
                        assert url == "https://cdn.115.com/real_video.mkv"

    def test_search(self, client):
        resp_data = {
            "state": True,
            "data": [{"n": "found.mkv", "fid": "400"}],
        }
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            results = client.search("found", dir_id="0")
            assert len(results) == 1
            assert results[0]["n"] == "found.mkv"

    def test_rename(self, client):
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.rename("100", "renamed.mkv")
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["fid"] == "100"
            assert data["file_name"] == "renamed.mkv"


class TestListFilesAllAndRecursive:
    """Test list_files_all and list_files_recursive with mocked list_files."""

    @pytest.fixture
    def client(self):
        c = Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        c._limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        c._download_limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        return c

    def test_list_files_all_single_page(self, client):
        items = [{"n": f"file{i}.mkv", "fid": str(i)} for i in range(5)]
        with patch.object(client, "list_files", return_value=items):
            result = client.list_files_all(dir_id="10")
            assert len(result) == 5

    def test_list_files_all_empty(self, client):
        with patch.object(client, "list_files", return_value=[]):
            result = client.list_files_all(dir_id="10")
            assert result == []

    def test_list_files_recursive_flat(self, client):
        """Flat directory with only files (no subdirs)."""
        items = [
            {"n": "a.mkv", "fid": "1"},
            {"n": "b.mkv", "fid": "2"},
        ]
        with patch.object(client, "list_files_all", return_value=items):
            result = client.list_files_recursive(dir_id="0", max_depth=2)
            assert len(result) == 2
            assert all(item["_is_dir"] is False for item in result)
            assert all(item["_parent_id"] == "0" for item in result)

    def test_list_files_recursive_with_subdir(self, client):
        """Directory with a subdirectory containing files."""
        root_items = [
            {"n": "SubDir", "cid": "55"},  # dir (no "fid")
            {"n": "root.mkv", "fid": "1"},
        ]
        sub_items = [
            {"n": "child.mkv", "fid": "2"},
        ]
        with patch.object(
            client, "list_files_all", side_effect=[root_items, sub_items]
        ):
            result = client.list_files_recursive(dir_id="0", max_depth=2)
            assert len(result) == 3
            dir_entry = [r for r in result if r["n"] == "SubDir"][0]
            assert dir_entry["_is_dir"] is True

    def test_list_files_recursive_max_depth(self, client):
        """Stops recursing when max_depth is exceeded."""
        result = client.list_files_recursive(dir_id="0", max_depth=0, _depth=1)
        assert result == []


class TestResolvePath:
    @pytest.fixture
    def client(self):
        c = Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        c._limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        c._download_limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        return c

    def test_resolve_empty_path(self, client):
        assert client.resolve_path("") == "0"
        assert client.resolve_path("/") == "0"

    def test_resolve_single_level(self, client):
        items = [{"fn": "movies", "cid": "42"}]  # dir, no "fid"
        with patch.object(client, "list_files_all", return_value=items):
            result = client.resolve_path("/movies")
            assert result == "42"

    def test_resolve_not_found(self, client):
        items = [{"fn": "other", "cid": "99"}]
        with patch.object(client, "list_files_all", return_value=items):
            with pytest.raises(FileNotFoundError, match="movies"):
                client.resolve_path("/movies")


class TestOpenAPIModeAPIs:
    """Test OpenAPI-mode API methods with mocked HTTP."""

    @pytest.fixture
    def client(self):
        c = Cloud115Client.from_openapi(
            app_id="test_app",
            app_secret="test_secret",
            access_token="test_token",
            refresh_token="test_refresh",
        )
        c._limiter = RateLimiter(qps=100, qpm=10000, use_state=False)
        return c

    def _mock_resp(self, json_data, status_code=200):
        resp = MagicMock()
        resp.status_code = status_code
        resp.json.return_value = json_data
        resp.raise_for_status = MagicMock()
        return resp

    def test_mkdir(self, client):
        resp_data = {"state": True, "cid": "888", "cname": "NewDir"}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.mkdir(parent_id="0", name="NewDir")
            assert result["cid"] == "888"
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["pid"] == "0"
            assert data["cname"] == "NewDir"

    def test_move(self, client):
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.move(["10", "20"], target_dir_id="100")
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["fid"] == "10,20"
            assert data["pid"] == "100"

    def test_delete(self, client):
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.delete(["30", "40"])
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["fid"] == "30,40"

    def test_rename(self, client):
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ) as mock_req:
            result = client.rename("50", "new_name.mkv")
            assert result["state"] is True
            call_args = mock_req.call_args
            data = call_args.kwargs.get("data", {})
            assert data["fid"] == "50"
            assert data["file_name"] == "new_name.mkv"

    def test_search(self, client):
        resp_data = {"state": True, "data": [{"n": "hit.mp4", "fid": "60"}]}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            results = client.search("hit")
            assert len(results) == 1
            assert results[0]["n"] == "hit.mp4"

    def test_download_url_openapi(self, client):
        resp_data = {
            "state": True,
            "data": {"pc1": {"url": {"url": "https://cdn.115.com/dl.mkv"}}},
        }
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            url = client.download_url("pc1")
            assert url == "https://cdn.115.com/dl.mkv"

    def test_download_url_string_format(self, client):
        """download_url when url value is a plain string, not a dict."""
        resp_data = {
            "state": True,
            "data": {"pc2": {"url": "https://cdn.115.com/direct.mkv"}},
        }
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            url = client.download_url("pc2")
            assert url == "https://cdn.115.com/direct.mkv"

    def test_download_url_no_url_raises(self, client):
        resp_data = {"state": True, "data": {"pc3": {"url": {}}}}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            with pytest.raises(ValueError, match="No download URL"):
                client.download_url("pc3")

    def test_batch_rename_falls_back_to_individual(self, client):
        """OpenAPI batch_rename falls back to individual rename calls."""
        resp_data = {"state": True}
        with patch.object(
            client._http, "request", return_value=self._mock_resp(resp_data)
        ):
            result = client.batch_rename({"1": "a.mkv", "2": "b.mkv"})
            assert result["state"] is True

    def test_get_dir_id_delegates_to_resolve_path(self, client):
        """OpenAPI get_dir_id delegates to resolve_path."""
        with patch.object(client, "resolve_path", return_value="42") as mock_rp:
            result = client.get_dir_id("/movies")
            assert result == "42"
            mock_rp.assert_called_once_with("/movies")

    def test_auto_refresh_expired_token(self, client):
        """When token is expired, _openapi_request auto-refreshes."""
        import time as _time

        client._token_expires_at = _time.time() - 10  # expired

        refresh_resp = MagicMock()
        refresh_resp.json.return_value = {
            "data": {
                "access_token": "refreshed",
                "refresh_token": "new_r",
                "expires_in": 7200,
            }
        }
        refresh_resp.raise_for_status = MagicMock()

        api_resp = MagicMock()
        api_resp.json.return_value = {"state": True, "data": []}
        api_resp.raise_for_status = MagicMock()

        with patch.object(
            client._http, "post", return_value=refresh_resp
        ):
            with patch.object(
                client._http, "request", return_value=api_resp
            ):
                client.list_files(dir_id="0")
                assert client._access_token == "refreshed"


class TestCheckLogin:
    def test_check_login_cookie_success(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"state": True}
        with patch.object(client._http, "get", return_value=mock_resp):
            assert client.check_login() is True

    def test_check_login_cookie_failure(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"state": False}
        with patch.object(client._http, "get", return_value=mock_resp):
            assert client.check_login() is False

    def test_check_login_exception(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        with patch.object(client._http, "get", side_effect=Exception("network")):
            assert client.check_login() is False


class TestContextManager:
    def test_context_manager(self):
        with Cloud115Client.from_cookies("UID=1_A1_0; CID=abc") as client:
            assert client._mode == "cookie"
        # after __exit__, close has been called (no crash)


class TestReadStateCorrupt:
    def test_corrupt_json(self, tmp_path):
        """_read_state returns {} for corrupt JSON."""
        from media115.client import _read_state

        state_file = tmp_path / "bad.json"
        state_file.write_text("{invalid json")
        result = _read_state(state_file)
        assert result == {}

    def test_missing_file(self, tmp_path):
        from media115.client import _read_state

        result = _read_state(tmp_path / "nonexistent.json")
        assert result == {}


class TestRateLimiterSetCooldown:
    def test_set_cooldown_writes_state(self, tmp_path):
        """Verify set_cooldown writes cooldown_until to state file."""
        import json

        state_file = tmp_path / "rate_limit.json"
        limiter = RateLimiter(qps=100, qpm=1000, use_state=False)
        limiter._state_path = state_file

        limiter.set_cooldown(1800)

        assert state_file.exists()
        state = json.loads(state_file.read_text())
        assert "cooldown_until" in state
        import time
        assert state["cooldown_until"] > time.time()
        assert state["cooldown_until"] <= time.time() + 1801


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

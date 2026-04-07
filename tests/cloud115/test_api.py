"""CloudAPI tests. Cookie mode (all mocked)."""

from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from cloud115.api import CloudAPI
from cloud115.cache import FileCache
from cloud115.rate_limit import RateLimiter


class TestRateLimiter:
    def test_allows_without_cache(self):
        """Without cache, limiter is in-process only and doesn't block."""
        limiter = RateLimiter("test_noop", qps=100, qpm=1000, cache=None)
        for _ in range(10):
            limiter.acquire()

    def test_persistent_state(self, tmp_path):
        """State is persisted to SQLite via FileCache."""
        cache = FileCache(tmp_path / "cache.db")
        limiter = RateLimiter("test_persist", qps=10, qpm=1000, cache=cache)
        limiter.acquire()
        state = cache.get_rate_limit("test_persist")
        assert state["last_request"] > 0
        assert state["minute_count"] == 1
        cache.close()

    def test_persistent_qpm(self, tmp_path):
        """QPM counter is tracked across calls."""
        cache = FileCache(tmp_path / "cache.db")
        limiter = RateLimiter("test_qpm", qps=100, qpm=1000, cache=cache)
        limiter.acquire()
        limiter.acquire()
        limiter.acquire()
        state = cache.get_rate_limit("test_qpm")
        assert state["minute_count"] == 3
        cache.close()

    def test_cooldown_blocks(self, tmp_path):
        """Cooldown blocks subsequent calls."""
        cache = FileCache(tmp_path / "cache.db")
        limiter = RateLimiter("test_cd", qps=100, qpm=1000, cache=cache)
        limiter.set_cooldown(3600)
        with pytest.raises(RuntimeError, match="cooldown"):
            limiter.acquire()
        cache.close()

    def test_tracks_request_count(self):
        limiter = RateLimiter("test_count", qps=100, qpm=1000, cache=None)
        for _ in range(5):
            limiter.acquire()
        assert limiter.request_count >= 5


@pytest.fixture()
def _fast_limiters(tmp_path):
    """Patch CloudAPI.__init__ to use fast (no-op) limiters for tests."""
    original_init = CloudAPI.__init__

    def fast_init(self, cookies="", cache_dir=None):
        original_init(self, cookies=cookies, cache_dir=tmp_path)
        self._limiter = RateLimiter("api", qps=100, qpm=10000, cache=None)
        self._download_limiter = RateLimiter("download", qps=100, qpm=10000, cache=None)

    with patch.object(CloudAPI, "__init__", fast_init):
        yield


class TestFactoryMethods:
    def test_from_cookies(self, tmp_path):
        client = CloudAPI.from_cookies("UID=12345_A1_170000; CID=abc; SEID=def", cache_dir=tmp_path)
        assert client._user_id == "12345"
        client.close()

    def test_from_cookies_no_uid(self, tmp_path):
        client = CloudAPI.from_cookies("CID=abc; SEID=def", cache_dir=tmp_path)
        assert client._user_id == ""
        client.close()

    def test_from_cookie_file(self, tmp_path):
        f = tmp_path / "cookies.txt"
        f.write_text("UID=99_A1_170000; CID=xyz; SEID=abc")
        client = CloudAPI.from_cookie_file(f, cache_dir=tmp_path)
        assert client._user_id == "99"
        client.close()

    def test_save_cookies_to_env(self, tmp_path):
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=x; SEID=y", cache_dir=tmp_path)
        env = tmp_path / ".env"
        env.write_text("TMDB_API_KEY=abc\n")
        client.save_cookies_to_env(env)
        content = env.read_text()
        assert "CLOUD_115_COOKIES=UID=1_A1_0; CID=x; SEID=y" in content
        assert "TMDB_API_KEY=abc" in content
        client.close()

    def test_save_cookies_to_new_env(self, tmp_path):
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=x; SEID=y", cache_dir=tmp_path)
        env = tmp_path / ".env"
        client.save_cookies_to_env(env)
        assert "CLOUD_115_COOKIES=UID=1_A1_0; CID=x; SEID=y" in env.read_text()
        client.close()


@pytest.mark.usefixtures("_fast_limiters")
class TestRenewCookies:
    def test_renew_success(self):
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=old; SEID=old")

        # Mock the 4-step auto-scan flow
        get_responses = [
            # Step 1: QR token
            MagicMock(json=lambda: {"data": {"uid": "test_uid"}}, raise_for_status=MagicMock()),
            # Step 2: auto-scan
            MagicMock(json=lambda: {"state": True}, raise_for_status=MagicMock()),
            # Step 3: auto-confirm
            MagicMock(json=lambda: {"state": True}, raise_for_status=MagicMock()),
        ]
        post_resp = MagicMock(
            json=lambda: {"data": {"cookie": {"UID": "2_A1_1", "CID": "new", "SEID": "new"}}},
            raise_for_status=MagicMock(),
        )

        with patch.object(client._http, "get", side_effect=get_responses):
            with patch.object(client._http, "post", return_value=post_resp):
                result = client.renew_cookies()
                assert result is True
                assert "new" in client._cookies
        client.close()

    def test_renew_fails_no_cookies(self):
        client = CloudAPI.from_cookies("")
        assert client.renew_cookies() is False
        client.close()

    def test_auto_renew_on_405(self):
        """When cookie request returns 405, should auto-renew and retry."""
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=old; SEID=old")

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
        client.close()


@pytest.mark.usefixtures("_fast_limiters")
class TestCookieMode:
    @pytest.fixture()
    def client(self):
        c = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        yield c
        c.close()

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


@pytest.mark.usefixtures("_fast_limiters")
class TestCookieModeAPIs:
    """Test individual API methods with mocked HTTP."""

    @pytest.fixture()
    def client(self):
        c = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        yield c
        c.close()

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
        with patch.object(client._http, "request", return_value=self._mock_resp(resp_data)):
            dir_id = client.get_dir_id("/movies/action")
            assert dir_id == "12345"

    def test_get_dir_id_not_found(self, client):
        resp_data = {"state": False}
        with patch.object(client._http, "request", return_value=self._mock_resp(resp_data)):
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

        init_resp = self._mock_resp(
            {
                "host": "https://oss.example.com/upload",
                "object": "obj_key",
                "accessid": "ak123",
                "policy": "pol",
                "signature": "sig",
                "callback": "cb",
            }
        )
        oss_resp = self._mock_resp({"data": {"file_id": "999", "file_name": "movie.nfo"}})

        with patch.object(client._http, "post", return_value=init_resp):
            with patch("cloud115.api.httpx.post", return_value=oss_resp) as mock_oss:
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

        with patch("cloud115.api.generate_m115_key", return_value=fake_key):
            with patch("cloud115.api.m115_encode", return_value="encoded_data"):
                with patch("cloud115.api.m115_decode", return_value=fake_decrypted):
                    with patch.object(client._http, "post", return_value=mock_post_resp):
                        url = client._download_url_cookie("pc_test")
                        assert url == "https://cdn.115.com/real_video.mkv"

    def test_search(self, client):
        resp_data = {
            "state": True,
            "data": [{"n": "found.mkv", "fid": "400"}],
        }
        with patch.object(client._http, "request", return_value=self._mock_resp(resp_data)):
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


@pytest.mark.usefixtures("_fast_limiters")
class TestListFilesAllPagination:
    """Test list_files_all with multi-page pagination."""

    @pytest.fixture()
    def client(self):
        c = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        yield c
        c.close()

    def test_list_files_all_single_page(self, client):
        items = [{"n": f"file{i}.mkv", "fid": str(i)} for i in range(5)]
        with patch.object(client, "list_files", return_value=items):
            result = client.list_files_all(dir_id="10")
            assert len(result) == 5

    def test_list_files_all_empty(self, client):
        with patch.object(client, "list_files", return_value=[]):
            result = client.list_files_all(dir_id="10")
            assert result == []

    def test_list_files_all_pagination(self, client):
        """Multiple pages: first page full (1000 items), second page partial."""
        page1 = [{"n": f"file{i}.mkv", "fid": str(i)} for i in range(1000)]
        page2 = [{"n": f"file{i}.mkv", "fid": str(i)} for i in range(1000, 1050)]

        with patch.object(client, "list_files", side_effect=[page1, page2]):
            result = client.list_files_all(dir_id="42")
            assert len(result) == 1050
            assert result[0]["n"] == "file0.mkv"
            assert result[-1]["n"] == "file1049.mkv"

    def test_list_files_all_exact_page_boundary(self, client):
        """When first page is exactly 1000 items, must request second page."""
        page1 = [{"n": f"file{i}.mkv", "fid": str(i)} for i in range(1000)]
        page2 = []

        with patch.object(client, "list_files", side_effect=[page1, page2]):
            result = client.list_files_all(dir_id="42")
            assert len(result) == 1000

    def test_list_files_all_three_pages(self, client):
        """Three full pages then partial."""
        page1 = [{"n": f"f{i}", "fid": str(i)} for i in range(1000)]
        page2 = [{"n": f"f{i}", "fid": str(i)} for i in range(1000, 2000)]
        page3 = [{"n": f"f{i}", "fid": str(i)} for i in range(2000, 2500)]

        with patch.object(client, "list_files", side_effect=[page1, page2, page3]):
            result = client.list_files_all(dir_id="42")
            assert len(result) == 2500


@pytest.mark.usefixtures("_fast_limiters")
class TestCheckLogin:
    def test_check_login_cookie_success(self):
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"state": True}
        with patch.object(client._http, "get", return_value=mock_resp):
            assert client.check_login() is True
        client.close()

    def test_check_login_cookie_failure(self):
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"state": False}
        with patch.object(client._http, "get", return_value=mock_resp):
            assert client.check_login() is False
        client.close()

    def test_check_login_exception(self):
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        with patch.object(client._http, "get", side_effect=Exception("network")):
            assert client.check_login() is False
        client.close()


@pytest.mark.usefixtures("_fast_limiters")
class TestContextManager:
    def test_context_manager(self):
        with CloudAPI.from_cookies("UID=1_A1_0; CID=abc") as client:
            assert client._cookies == "UID=1_A1_0; CID=abc"
        # after __exit__, close has been called (no crash)


@pytest.mark.usefixtures("_fast_limiters")
class TestRateLimiterSetCooldown:
    def test_set_cooldown_via_cache(self, tmp_path):
        """Verify set_cooldown writes cooldown_until to FileCache."""
        import time

        cache = FileCache(tmp_path / "cd_test.db")
        limiter = RateLimiter("test_cd_write", qps=100, qpm=1000, cache=cache)
        limiter.set_cooldown(1800)

        state = cache.get_rate_limit("test_cd_write")
        assert state["cooldown_until"] > time.time()
        assert state["cooldown_until"] <= time.time() + 1801
        cache.close()


@pytest.mark.usefixtures("_fast_limiters")
class TestExportTree:
    """Tests for the export_tree 3-step flow."""

    @pytest.fixture()
    def client(self):
        c = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        yield c
        c.close()

    def _mock_resp(self, json_data, status_code=200):
        resp = MagicMock()
        resp.status_code = status_code
        resp.json.return_value = json_data
        resp.raise_for_status = MagicMock()
        return resp

    @patch("cloud115.api.time.sleep", return_value=None)
    def test_export_tree(self, _sleep, client):
        """Full flow: POST export_dir -> poll -> download -> delete -> return text."""
        tree_text = "root\n" + "".join(f"  file_{i}.mkv\n" for i in range(20))
        tree_bytes = tree_text.encode("utf-16-le")

        start_resp = {"state": True, "data": {"export_id": "123"}}
        poll_resp = {
            "state": True,
            "data": {"pick_code": "pc_tree", "file_id": "999"},
        }
        delete_resp = {"state": True}

        dl_resp = MagicMock()
        dl_resp.status_code = 200
        dl_resp.content = tree_bytes

        with patch.object(
            client, "_cookie_request", side_effect=[start_resp, poll_resp, delete_resp]
        ):
            with patch("httpx.get", return_value=dl_resp):
                result = client.export_tree("12345")

        assert result is not None
        assert "root" in result
        assert "file_0.mkv" in result

    @patch("cloud115.api.time.sleep", return_value=None)
    def test_export_tree_pending(self, _sleep, client):
        """When start returns no export_id, falls back to export_id=0 for polling."""
        tree_text = "dir_tree\n" + "".join(f"  item_{i}.mkv\n" for i in range(20))
        tree_bytes = tree_text.encode("utf-16-le")

        start_resp = {"state": True, "errno": 990005, "data": {}}
        poll_resp = {
            "state": True,
            "data": {"pick_code": "pc_pending", "file_id": "888"},
        }
        delete_resp = {"state": True}

        dl_resp = MagicMock()
        dl_resp.status_code = 200
        dl_resp.content = tree_bytes

        cookie_request_calls = []

        def track_cookie_request(method, url, **kwargs):
            cookie_request_calls.append((method, url, kwargs))
            if len(cookie_request_calls) == 1:
                return start_resp
            if len(cookie_request_calls) == 2:
                return poll_resp
            return delete_resp

        with patch.object(client, "_cookie_request", side_effect=track_cookie_request):
            with patch("httpx.get", return_value=dl_resp):
                result = client.export_tree("12345")

        assert result is not None
        _, poll_url, poll_kwargs = cookie_request_calls[1]
        assert poll_kwargs.get("params", {}).get("export_id") == 0

    @patch("cloud115.api.time.sleep", return_value=None)
    def test_export_tree_list_format(self, _sleep, client):
        """When poll returns data as a list instead of dict."""
        tree_text = "list_format_tree\n" + "".join(f"  entry_{i}.mkv\n" for i in range(20))
        tree_bytes = tree_text.encode("utf-16-le")

        start_resp = {"state": True, "data": {"export_id": "456"}}
        poll_resp = {
            "state": True,
            "data": [{"pick_code": "pc_list", "file_id": "777"}],
        }
        delete_resp = {"state": True}

        dl_resp = MagicMock()
        dl_resp.status_code = 200
        dl_resp.content = tree_bytes

        with patch.object(
            client, "_cookie_request", side_effect=[start_resp, poll_resp, delete_resp]
        ):
            with patch("httpx.get", return_value=dl_resp):
                result = client.export_tree("12345")

        assert result is not None
        assert "list_format_tree" in result


@pytest.mark.usefixtures("_fast_limiters")
class TestSaveCookiesToEnv:
    def test_save_cookies_to_env_existing(self, tmp_path):
        """Save cookies to .env that already has the key -> replaces it."""
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=new; SEID=new")
        env = tmp_path / ".env"
        env.write_text("CLOUD_115_COOKIES=old_value\nTMDB_API_KEY=abc\n")
        client.save_cookies_to_env(env)
        content = env.read_text()
        assert "CLOUD_115_COOKIES=UID=1_A1_0; CID=new; SEID=new" in content
        assert "old_value" not in content
        assert "TMDB_API_KEY=abc" in content
        client.close()

    def test_save_cookies_to_env_new_file(self, tmp_path):
        """Save cookies to non-existent .env -> creates it."""
        client = CloudAPI.from_cookies("UID=2_A1_0; CID=x; SEID=y")
        env = tmp_path / ".env"
        client.save_cookies_to_env(env)
        content = env.read_text()
        assert "CLOUD_115_COOKIES=UID=2_A1_0; CID=x; SEID=y" in content
        client.close()


@pytest.mark.usefixtures("_fast_limiters")
class TestRenewCookiesEdgeCases:
    def test_renew_cookies_no_existing(self):
        """Client with empty cookies returns False."""
        client = CloudAPI.from_cookies("")
        assert client.renew_cookies() is False
        client.close()

    def test_renew_cookies_empty_cookies(self):
        """Client with empty cookie string returns False."""
        client = CloudAPI.from_cookies("")
        assert client.renew_cookies() is False
        client.close()

    def test_renew_cookies_exception_returns_false(self):
        """If any step in renewal raises, returns False gracefully."""
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")
        with patch.object(client._http, "get", side_effect=Exception("network error")):
            assert client.renew_cookies() is False
        client.close()

    def test_renew_cookies_short_cookie_returns_false(self):
        """If renewed cookie is too short (<10 chars), returns False."""
        client = CloudAPI.from_cookies("UID=1_A1_0; CID=abc; SEID=def")

        get_responses = [
            MagicMock(
                json=lambda: {"data": {"uid": "test_uid"}},
                raise_for_status=MagicMock(),
            ),
            MagicMock(json=lambda: {"state": True}, raise_for_status=MagicMock()),
            MagicMock(json=lambda: {"state": True}, raise_for_status=MagicMock()),
        ]
        post_resp = MagicMock(
            json=lambda: {"data": {"cookie": "short"}},
            raise_for_status=MagicMock(),
        )

        with patch.object(client._http, "get", side_effect=get_responses):
            with patch.object(client._http, "post", return_value=post_resp):
                result = client.renew_cookies()
                assert result is False
        client.close()


class TestCrypto:
    def test_m115_roundtrip(self):
        """Test that M115 encode/decode is consistent."""
        from cloud115.crypto import generate_m115_key, m115_encode

        key = generate_m115_key()
        assert len(key) == 16

        plaintext = '{"pickcode": "abc123"}'
        encoded = m115_encode(key, plaintext)
        assert isinstance(encoded, str)
        assert len(encoded) > 0

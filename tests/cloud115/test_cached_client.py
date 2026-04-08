"""CachedClient tests — cache-first reads + write-through writes."""
from __future__ import annotations

import time
from unittest.mock import MagicMock

import pytest

from cloud115.cached import (
    CachedClient,
    _entry_to_public,
    _normalize,
    _normalize_item,
)
from cloud115.cache import FileCache


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def _raw_file(name="movie.mkv", fid="f1", size=1024, pc="pk1"):
    """Raw API item for a file."""
    return {"n": name, "fid": fid, "s": size, "pc": pc}


def _raw_dir(name="subdir", cid="200"):
    """Raw API item for a directory."""
    return {"n": name, "cid": cid}


# ---------------------------------------------------------------------------
# fixtures
# ---------------------------------------------------------------------------

@pytest.fixture()
def mock_api():
    api = MagicMock()
    api.get_dir_id.return_value = "100"
    api.list_files_all.return_value = [
        _raw_file("movie.mkv", "f1", 1024, "pk1"),
        _raw_dir("subdir", "200"),
    ]
    api.close.return_value = None
    return api


@pytest.fixture()
def client(tmp_path, mock_api):
    c = CachedClient.__new__(CachedClient)
    c._api = mock_api
    c._cache = FileCache(tmp_path / "test.db")
    c._listing_ttl = 3600
    c._path_ttl = 86400
    return c


# ---------------------------------------------------------------------------
# _normalize
# ---------------------------------------------------------------------------

class TestNormalize:
    def test_empty(self):
        assert _normalize("") == "/"

    def test_root(self):
        assert _normalize("/") == "/"

    def test_strip_trailing_slash(self):
        assert _normalize("/movies/") == "/movies"

    def test_adds_leading_slash(self):
        assert _normalize("movies") == "/movies"

    def test_nested(self):
        assert _normalize("/a/b/c/") == "/a/b/c"


# ---------------------------------------------------------------------------
# _normalize_item
# ---------------------------------------------------------------------------

class TestNormalizeItem:
    def test_file(self):
        result = _normalize_item({"n": "test.mkv", "fid": "f1", "s": 999, "pc": "pk"})
        assert result == {
            "name": "test.mkv",
            "type": "file",
            "node_id": "f1",
            "size": 999,
            "pick_code": "pk",
        }

    def test_dir(self):
        result = _normalize_item({"n": "folder", "cid": "c1"})
        assert result == {
            "name": "folder",
            "type": "dir",
            "node_id": "c1",
            "size": 0,
            "pick_code": "",
        }

    def test_fn_key(self):
        result = _normalize_item({"fn": "alt.txt", "fid": "f2", "s": 10, "pc": "p"})
        assert result["name"] == "alt.txt"
        assert result["type"] == "file"


# ---------------------------------------------------------------------------
# _entry_to_public
# ---------------------------------------------------------------------------

class TestEntryToPublic:
    def test_file(self):
        entry = {
            "name": "a.mkv", "type": "file", "node_id": "f1",
            "size": 100, "pick_code": "pk",
        }
        pub = _entry_to_public(entry)
        assert pub == {
            "name": "a.mkv", "type": "file", "fid": "f1",
            "size": 100, "pick_code": "pk",
        }

    def test_dir(self):
        entry = {
            "name": "sub", "type": "dir", "node_id": "c1",
            "size": 0, "pick_code": "",
        }
        pub = _entry_to_public(entry)
        assert pub == {"name": "sub", "type": "dir", "cid": "c1"}


# ---------------------------------------------------------------------------
# resolve_path
# ---------------------------------------------------------------------------

class TestResolvePath:
    def test_root(self, client):
        assert client.resolve_path("/") == "0"
        client._api.get_dir_id.assert_not_called()

    def test_cache_miss_calls_api(self, client):
        cid = client.resolve_path("/movies")
        assert cid == "100"
        client._api.get_dir_id.assert_called_once_with("/movies")

    def test_cache_hit_skips_api(self, client):
        client.resolve_path("/movies")
        client._api.get_dir_id.reset_mock()

        cid = client.resolve_path("/movies")
        assert cid == "100"
        client._api.get_dir_id.assert_not_called()

    def test_not_found_raises(self, client):
        client._api.get_dir_id.return_value = None
        with pytest.raises(FileNotFoundError, match="Path not found"):
            client.resolve_path("/nonexistent")

    def test_expired_ttl_calls_api_again(self, client):
        client._path_ttl = 0  # instant expiry
        client.resolve_path("/movies")
        client._api.get_dir_id.reset_mock()

        client.resolve_path("/movies")
        client._api.get_dir_id.assert_called_once()


# ---------------------------------------------------------------------------
# list_dir
# ---------------------------------------------------------------------------

class TestListDir:
    def test_cache_miss_calls_api(self, client):
        entries = client.list_dir("/movies")
        assert len(entries) == 2
        client._api.list_files_all.assert_called_once_with("100")

        # Check public format
        names = {e["name"] for e in entries}
        assert names == {"movie.mkv", "subdir"}

        file_entry = next(e for e in entries if e["type"] == "file")
        assert file_entry["fid"] == "f1"
        assert file_entry["size"] == 1024
        assert file_entry["pick_code"] == "pk1"

        dir_entry = next(e for e in entries if e["type"] == "dir")
        assert dir_entry["cid"] == "200"

    def test_cache_hit_skips_api(self, client):
        client.list_dir("/movies")
        client._api.list_files_all.reset_mock()

        entries = client.list_dir("/movies")
        assert len(entries) == 2
        client._api.list_files_all.assert_not_called()

    def test_stale_ttl_refetches(self, client):
        client._listing_ttl = 0  # instant expiry
        client.list_dir("/movies")
        client._api.list_files_all.reset_mock()

        client.list_dir("/movies")
        client._api.list_files_all.assert_called_once()


# ---------------------------------------------------------------------------
# find_file
# ---------------------------------------------------------------------------

class TestFindFile:
    def test_returns_metadata(self, client):
        result = client.find_file("/movies/movie.mkv")
        assert result["name"] == "movie.mkv"
        assert result["fid"] == "f1"
        assert result["size"] == 1024
        assert result["pick_code"] == "pk1"
        assert result["parent_cid"] == "100"

    def test_not_found_raises(self, client):
        with pytest.raises(FileNotFoundError, match="File not found"):
            client.find_file("/movies/nonexistent.txt")

    def test_root_raises(self, client):
        with pytest.raises(FileNotFoundError, match="Root is not a file"):
            client.find_file("/")

    def test_refreshes_stale_listing(self, client):
        """If dir_meta is stale/missing, find_file refreshes before lookup."""
        # First call populates cache
        client.find_file("/movies/movie.mkv")
        client._api.list_files_all.reset_mock()

        # Mark listing as stale
        client._cache.delete_dir_meta("100")

        # Should refetch
        client.find_file("/movies/movie.mkv")
        client._api.list_files_all.assert_called_once_with("100")


# ---------------------------------------------------------------------------
# mkdir
# ---------------------------------------------------------------------------

class TestMkdir:
    def test_creates_and_caches(self, client):
        client._api.mkdir.return_value = {"cid": "300", "aid": ""}

        new_cid = client.mkdir("/movies/new_folder")

        assert new_cid == "300"
        client._api.mkdir.assert_called_once_with("100", "new_folder")

        # Cache updated
        entry = client._cache.find_entry("100", "new_folder")
        assert entry is not None
        assert entry["type"] == "dir"
        assert entry["node_id"] == "300"

        # Path index updated
        cached = client._cache.get_path("/movies/new_folder")
        assert cached is not None
        assert cached[0] == "300"

    def test_mkdir_parents(self, client):
        call_count = [0]
        cids = {"a": "10", "b": "20", "c": "30"}

        def mock_get_dir_id(path):
            return None  # all segments missing

        def mock_mkdir(parent_id, name):
            call_count[0] += 1
            return {"cid": cids[name]}

        client._api.get_dir_id.side_effect = mock_get_dir_id
        client._api.mkdir.side_effect = mock_mkdir

        result = client.mkdir("/a/b/c", parents=True)
        assert result == "30"
        assert call_count[0] == 3


# ---------------------------------------------------------------------------
# rename
# ---------------------------------------------------------------------------

class TestRename:
    def test_rename_file(self, client):
        # Populate cache
        client.list_dir("/movies")
        client._api.rename.return_value = {"state": True}

        client.rename("/movies/movie.mkv", "renamed.mkv")

        client._api.rename.assert_called_once_with("f1", "renamed.mkv")

        # Old name gone, new name present
        assert client._cache.find_entry("100", "movie.mkv") is None
        assert client._cache.find_entry("100", "renamed.mkv") is not None

    def test_rename_dir_updates_path_index(self, client):
        client.list_dir("/movies")
        client._api.rename.return_value = {"state": True}
        # Pre-cache a sub-path
        client._cache.set_path("/movies/subdir", "200")
        client._cache.set_path("/movies/subdir/child", "201")

        client.rename("/movies/subdir", "newname")

        # Old paths deleted
        assert client._cache.get_path("/movies/subdir") is None
        assert client._cache.get_path("/movies/subdir/child") is None
        # New path set
        cached = client._cache.get_path("/movies/newname")
        assert cached is not None
        assert cached[0] == "200"


# ---------------------------------------------------------------------------
# delete
# ---------------------------------------------------------------------------

class TestDelete:
    def test_delete_file(self, client):
        client.list_dir("/movies")
        client._api.delete.return_value = {"state": True}

        client.delete(["/movies/movie.mkv"])

        client._api.delete.assert_called_once_with(["f1"])
        assert client._cache.find_entry("100", "movie.mkv") is None

    def test_delete_multiple(self, client):
        client.list_dir("/movies")
        client._api.delete.return_value = {"state": True}

        client.delete(["/movies/movie.mkv", "/movies/subdir"])

        args = client._api.delete.call_args[0][0]
        assert set(args) == {"f1", "200"}

    def test_delete_not_found_raises(self, client):
        # Parent resolves fine, but the child is not in the listing
        client._api.list_files_all.return_value = []
        # get_dir_id returns None for the child path (not a dir either)
        original_get = client._api.get_dir_id.side_effect

        def _get_dir_id(path):
            if path == "/movies":
                return "100"
            return None

        client._api.get_dir_id.side_effect = _get_dir_id
        with pytest.raises(FileNotFoundError, match="Not found"):
            client.delete(["/movies/ghost.txt"])


# ---------------------------------------------------------------------------
# move
# ---------------------------------------------------------------------------

class TestMove:
    def test_move_file(self, client):
        client.list_dir("/movies")
        client._api.move.return_value = {"state": True}
        client._api.get_dir_id.return_value = "500"

        client.move(["/movies/movie.mkv"], "/archive")

        client._api.move.assert_called_once_with(["f1"], "500")

        # Source removed
        assert client._cache.find_entry("100", "movie.mkv") is None
        # Target has entry
        target_entry = client._cache.find_entry("500", "movie.mkv")
        assert target_entry is not None
        assert target_entry["node_id"] == "f1"


# ---------------------------------------------------------------------------
# batch_rename
# ---------------------------------------------------------------------------

class TestBatchRename:
    def test_batch_rename(self, client):
        client.list_dir("/movies")
        client._api.batch_rename.return_value = {"state": True}

        client.batch_rename([("/movies/movie.mkv", "film.mkv")])

        client._api.batch_rename.assert_called_once_with({"f1": "film.mkv"})
        assert client._cache.find_entry("100", "movie.mkv") is None
        assert client._cache.find_entry("100", "film.mkv") is not None


# ---------------------------------------------------------------------------
# upload
# ---------------------------------------------------------------------------

class TestUpload:
    def test_upload_with_fid(self, client, tmp_path):
        f = tmp_path / "test.nfo"
        f.write_text("data")

        client._api.upload_file.return_value = {
            "fid": "f99",
            "file_size": 4,
            "pick_code": "pk99",
        }

        result = client.upload(str(f), "/movies")

        assert result["fid"] == "f99"
        entry = client._cache.find_entry("100", "test.nfo")
        assert entry is not None
        assert entry["node_id"] == "f99"

    def test_upload_without_fid_marks_stale(self, client, tmp_path):
        f = tmp_path / "test.nfo"
        f.write_text("data")

        client._api.upload_file.return_value = {"status": "ok"}

        # Pre-populate dir_meta
        client.list_dir("/movies")
        assert client._cache.get_dir_ts("100") is not None

        client.upload(str(f), "/movies")

        # dir_meta deleted (stale)
        assert client._cache.get_dir_ts("100") is None

    def test_upload_none_response_marks_stale(self, client, tmp_path):
        f = tmp_path / "test.nfo"
        f.write_text("data")

        client._api.upload_file.return_value = None

        client.list_dir("/movies")
        client.upload(str(f), "/movies")

        assert client._cache.get_dir_ts("100") is None


# ---------------------------------------------------------------------------
# cache management
# ---------------------------------------------------------------------------

class TestCacheManagement:
    def test_invalidate(self, client):
        client.list_dir("/movies")
        # Path and dir_meta should exist
        assert client._cache.get_path("/movies") is not None

        client.invalidate("/movies")

        assert client._cache.get_path("/movies") is None

    def test_invalidate_clears_dir_listing(self, client):
        """invalidate() must also remove the dir listing (dir_meta + entries)."""
        client.list_dir("/movies")
        # Verify listing is cached
        assert client._cache.get_dir_ts("100") is not None
        assert len(client._cache.get_dir_entries("100")) == 2

        client.invalidate("/movies")

        # Dir listing must be gone
        assert client._cache.get_dir_ts("100") is None
        assert client._cache.get_dir_entries("100") == []

    def test_cache_status(self, client):
        s = client.cache_status()
        assert "path_count" in s
        assert "dir_count" in s
        assert "entry_count" in s

    def test_cache_clear(self, client):
        client.list_dir("/movies")
        client.cache_clear()
        s = client.cache_status()
        assert s["path_count"] == 0
        assert s["dir_count"] == 0
        assert s["entry_count"] == 0


# ---------------------------------------------------------------------------
# download_url / stream_url
# ---------------------------------------------------------------------------

class TestDownloadUrl:
    def test_download_url_no_ua(self, client, mock_api):
        mock_api.download_url.return_value = "https://cdn.115.com/file"
        url = client.download_url("pk1")
        assert url == "https://cdn.115.com/file"
        mock_api.download_url.assert_called_once_with("pk1", None)

    def test_download_url_with_ua(self, client, mock_api):
        mock_api.download_url.return_value = "https://cdn.115.com/file"
        url = client.download_url("pk1", user_agent="VLC/3.0")
        assert url == "https://cdn.115.com/file"
        mock_api.download_url.assert_called_once_with("pk1", "VLC/3.0")


class TestStreamUrl:
    def test_stream_url(self, client, mock_api):
        mock_api.download_url.return_value = "https://cdn.115.com/file"
        url = client.stream_url("/影音/电影/movie.mkv")
        assert url == "https://cdn.115.com/file"
        mock_api.download_url.assert_called_once_with("pk1", None)

    def test_stream_url_with_ua(self, client, mock_api):
        mock_api.download_url.return_value = "https://cdn.115.com/file"
        url = client.stream_url("/影音/电影/movie.mkv", user_agent="VLC/3.0")
        mock_api.download_url.assert_called_once_with("pk1", "VLC/3.0")


# ---------------------------------------------------------------------------
# auth delegation
# ---------------------------------------------------------------------------

class TestAuth:
    def test_check_login(self, client):
        client._api.check_login.return_value = True
        assert client.check_login() is True

    def test_renew_cookies(self, client):
        client._api.renew_cookies.return_value = True
        assert client.renew_cookies("tv") is True

    def test_save_cookies(self, client, tmp_path):
        env_file = tmp_path / ".env"
        client.save_cookies(str(env_file))
        client._api.save_cookies_to_env.assert_called_once()


# ---------------------------------------------------------------------------
# walk
# ---------------------------------------------------------------------------

class TestWalk:
    def test_walk_depth(self, client):
        """walk collects entries recursively (capped by max_depth)."""
        # Root listing has a subdir; subdir has only a file (no further dirs)
        def _list_by_cid(cid):
            if cid == "100":
                return [_raw_file("a.mkv", "f1"), _raw_dir("sub", "200")]
            # cid == "200": leaf dir
            return [_raw_file("b.mkv", "f2")]

        client._api.list_files_all.side_effect = _list_by_cid
        # Subdir path resolution
        client._api.get_dir_id.side_effect = lambda p: {"/movies": "100", "/movies/sub": "200"}.get(p)

        result = client.walk("/movies", max_depth=5)
        assert len(result) == 2
        assert result[0]["path"] == "/movies"
        assert result[1]["path"] == "/movies/sub"

    def test_walk_respects_max_depth(self, client):
        client._api.list_files_all.return_value = [_raw_dir("deep", "300")]

        result = client.walk("/movies", max_depth=0)
        # depth 0 = root only
        assert len(result) == 1


# ---------------------------------------------------------------------------
# stat
# ---------------------------------------------------------------------------

class TestStat:
    def test_stat_file(self, client):
        result = client.stat("/movies/movie.mkv")
        assert result["type"] == "file"
        assert result["name"] == "movie.mkv"

    def test_stat_dir(self, client):
        # Resolve returns cid for dirs
        result = client.stat("/movies")
        assert result["type"] == "dir"
        assert result["cid"] == "100"

    def test_stat_root(self, client):
        result = client.stat("/")
        assert result == {"name": "/", "type": "dir", "cid": "0"}


# ---------------------------------------------------------------------------
# shared FileCache between CachedClient and CloudAPI
# ---------------------------------------------------------------------------

def test_shared_file_cache(tmp_path):
    """CachedClient and CloudAPI share the same FileCache instance."""
    from cloud115.api import CloudAPI
    from cloud115.cache import FileCache

    c = CachedClient.__new__(CachedClient)
    c._cache = FileCache(tmp_path / "test.db")

    # Verify that when we pass file_cache to CloudAPI, it uses it
    api = CloudAPI(cookies="test=1", file_cache=c._cache)
    assert api._file_cache is c._cache
    assert api._owns_cache is False
    api.close()  # should NOT close the shared cache
    # cache should still be usable
    c._cache.set_path("/test", "123")
    assert c._cache.get_path("/test") is not None
    c._cache.close()


# ---------------------------------------------------------------------------
# _normalize_item with both fid and cid
# ---------------------------------------------------------------------------

def test_normalize_item_fid_and_cid():
    """Items with both fid and cid should be classified as files."""
    item = {"n": "movie.mkv", "fid": "f1", "cid": "parent_cid", "s": 1024, "pc": "pk1"}
    result = _normalize_item(item)
    assert result["type"] == "file"
    assert result["node_id"] == "f1"


# ---------------------------------------------------------------------------
# refresh_paths
# ---------------------------------------------------------------------------

class TestRefreshPaths:
    def test_calls_refresh_dir_for_each_unique_path(self, client):
        """refresh_paths calls the API once per unique normalized path."""
        client._api.list_files_all.return_value = [_raw_file("a.mkv", "f1")]
        # Pre-resolve /movies so resolve_path hits cache
        client._api.get_dir_id.return_value = "100"

        client.refresh_paths(["/movies", "/movies/", "movies"])

        # All three are the same path after normalization — one API call
        client._api.list_files_all.assert_called_once_with("100")

    def test_deduplicates_paths(self, client):
        """Duplicate paths result in a single refresh each."""
        client._api.list_files_all.return_value = []
        client._api.get_dir_id.return_value = "100"

        client.refresh_paths(["/movies", "/movies", "/movies"])

        client._api.list_files_all.assert_called_once()

    def test_multiple_distinct_paths(self, client):
        """Each distinct path triggers its own refresh."""
        call_count = {"n": 0}

        def _list(cid):
            call_count["n"] += 1
            return []

        client._api.list_files_all.side_effect = _list
        client._api.get_dir_id.side_effect = lambda p: {"100": "100", "/movies": "100", "/archive": "200"}.get(p, "100")

        client.refresh_paths(["/movies", "/archive"])

        assert call_count["n"] == 2

    def test_empty_list_is_noop(self, client):
        """Empty paths list makes no API calls."""
        client.refresh_paths([])
        client._api.list_files_all.assert_not_called()


# ---------------------------------------------------------------------------
# tree cache: _parse_tree_text, save_tree, get_tree_entries, tree_stats
# ---------------------------------------------------------------------------

_SAMPLE_TREE = """\
\ufeff|——根目录
| |-影音
| | |-AV
| | | |-DANDY-001
| | | | |-DANDY-001.mkv
| | | | |-DANDY-001.nfo
| | | |-DANDY-002
| | | | |-DANDY-002.mp4
| | |-电影
| | | |-Dune (2021)
| | | | |-Dune (2021).mkv
"""

_VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v"}


class TestParseTreeText:
    def test_finds_videos_and_nfo(self):
        entries = CachedClient._parse_tree_text(_SAMPLE_TREE, _VIDEO_EXTS)
        paths = {e["path"] for e in entries}
        assert "影音/AV/DANDY-001/DANDY-001.mkv" in paths
        assert "影音/AV/DANDY-001/DANDY-001.nfo" in paths
        assert "影音/AV/DANDY-002/DANDY-002.mp4" in paths
        assert "影音/电影/Dune (2021)/Dune (2021).mkv" in paths

    def test_excludes_directories(self):
        entries = CachedClient._parse_tree_text(_SAMPLE_TREE, _VIDEO_EXTS)
        names = {e["n"] for e in entries}
        # Directory names should not appear as entries
        assert "AV" not in names
        assert "DANDY-001" not in names
        assert "电影" not in names

    def test_is_video_flag(self):
        entries = CachedClient._parse_tree_text(_SAMPLE_TREE, _VIDEO_EXTS)
        by_path = {e["path"]: e for e in entries}
        assert by_path["影音/AV/DANDY-001/DANDY-001.mkv"]["is_video"] is True
        assert by_path["影音/AV/DANDY-001/DANDY-001.nfo"]["is_video"] is False

    def test_is_nfo_flag(self):
        entries = CachedClient._parse_tree_text(_SAMPLE_TREE, _VIDEO_EXTS)
        by_path = {e["path"]: e for e in entries}
        assert by_path["影音/AV/DANDY-001/DANDY-001.nfo"]["is_nfo"] is True
        assert by_path["影音/AV/DANDY-001/DANDY-001.mkv"]["is_nfo"] is False

    def test_parent_field(self):
        entries = CachedClient._parse_tree_text(_SAMPLE_TREE, _VIDEO_EXTS)
        by_path = {e["path"]: e for e in entries}
        assert by_path["影音/AV/DANDY-001/DANDY-001.mkv"]["parent"] == "影音/AV/DANDY-001"
        assert by_path["影音/电影/Dune (2021)/Dune (2021).mkv"]["parent"] == "影音/电影/Dune (2021)"

    def test_empty_text(self):
        assert CachedClient._parse_tree_text("", _VIDEO_EXTS) == []

    def test_name_field(self):
        entries = CachedClient._parse_tree_text(_SAMPLE_TREE, _VIDEO_EXTS)
        by_path = {e["path"]: e for e in entries}
        assert by_path["影音/AV/DANDY-001/DANDY-001.mkv"]["n"] == "DANDY-001.mkv"


class TestSaveAndGetTree:
    def test_save_tree_returns_count(self, client):
        count = client.save_tree(_SAMPLE_TREE, _VIDEO_EXTS)
        assert count == 4  # 3 videos + 1 nfo

    def test_save_tree_stores_in_cache(self, client):
        client.save_tree(_SAMPLE_TREE, _VIDEO_EXTS)
        entries = client.get_tree_entries()
        assert len(entries) == 4

    def test_get_tree_entries_with_category(self, client):
        client.save_tree(_SAMPLE_TREE, _VIDEO_EXTS)
        av_entries = client.get_tree_entries("影音/AV")
        assert all("影音/AV" in e["path"] for e in av_entries)
        assert len(av_entries) == 3  # DANDY-001.mkv, DANDY-001.nfo, DANDY-002.mp4

    def test_tree_stats(self, client):
        client.save_tree(_SAMPLE_TREE, _VIDEO_EXTS)
        stats = client.tree_stats()
        assert stats["total"] == 4
        assert stats["videos"] == 3
        assert stats["nfos"] == 1

    def test_save_tree_replaces_previous(self, client):
        client.save_tree(_SAMPLE_TREE, _VIDEO_EXTS)
        client.save_tree("影音\n|- AV\n| |- X\n| | |- X.mkv\n", _VIDEO_EXTS)
        entries = client.get_tree_entries()
        assert len(entries) == 1
        assert entries[0]["n"] == "X.mkv"

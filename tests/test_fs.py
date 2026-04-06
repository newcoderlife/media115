import json
import time
from unittest.mock import MagicMock

import pytest

from media115.fs import PathResolver


@pytest.fixture
def resolver(tmp_path, monkeypatch):
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path))
    client = MagicMock()
    return PathResolver(client)


class TestResolveDir:
    def test_root(self, resolver):
        assert resolver.resolve_dir("/") == "0"
        resolver.client.get_dir_id.assert_not_called()

    def test_cached_path(self, resolver):
        resolver._write_path_index("/影音", "123")
        assert resolver.resolve_dir("/影音") == "123"
        resolver.client.get_dir_id.assert_not_called()

    def test_uncached_path_hits_api(self, resolver):
        resolver.client.get_dir_id.return_value = "456"
        assert resolver.resolve_dir("/影音/电影") == "456"
        resolver.client.get_dir_id.assert_called_once_with("/影音/电影")
        # 第二次走缓存
        resolver.client.get_dir_id.reset_mock()
        assert resolver.resolve_dir("/影音/电影") == "456"
        resolver.client.get_dir_id.assert_not_called()

    def test_nonexistent_path(self, resolver):
        resolver.client.get_dir_id.return_value = None
        with pytest.raises(FileNotFoundError):
            resolver.resolve_dir("/不存在")


class TestResolveFile:
    def test_resolve_file(self, resolver):
        resolver._write_path_index("/影音/电影", "100")
        resolver._write_dir_listing("100", [
            {"name": "test.mkv", "fid": "200", "size": 1000, "pick_code": "pc1"},
        ])
        fid, parent_cid, meta = resolver.resolve_file("/影音/电影/test.mkv")
        assert fid == "200"
        assert parent_cid == "100"
        assert meta["pick_code"] == "pc1"

    def test_file_not_found(self, resolver):
        resolver._write_path_index("/影音", "100")
        resolver._write_dir_listing("100", [])
        resolver.client.list_files_all.return_value = []
        with pytest.raises(FileNotFoundError, match="no_such.mkv"):
            resolver.resolve_file("/影音/no_such.mkv")


class TestCacheInvalidation:
    def test_invalidate_dir(self, resolver):
        resolver._write_path_index("/影音/电影", "100")
        resolver._write_dir_listing("100", [{"name": "a.mkv", "fid": "1"}])
        resolver.invalidate("/影音/电影")
        assert resolver._read_path_index().get("/影音/电影") is None

    def test_mark_stale(self, resolver):
        resolver._write_dir_listing("100", [{"name": "a.mkv", "fid": "1"}])
        resolver.mark_stale("100")
        listing = resolver._read_dir_listing("100")
        assert listing["stale"] is True

    def test_invalidate_recursive(self, resolver):
        """invalidate 应该递归清理所有子路径。"""
        resolver._write_path_index("/a", "100")
        resolver._write_path_index("/a/b", "200")
        resolver._write_path_index("/a/b/c", "300")
        resolver._write_path_index("/other", "400")
        resolver.invalidate("/a")
        index = resolver._read_path_index()
        assert "/a" not in index
        assert "/a/b" not in index
        assert "/a/b/c" not in index
        assert "/other" in index  # 不应该被删


class TestListing:
    def test_listing_from_cache(self, resolver):
        resolver._write_dir_listing("100", [{"name": "a.mkv", "fid": "1"}])
        items = resolver.listing("100")
        assert len(items) == 1
        resolver.client.list_files_all.assert_not_called()

    def test_listing_stale_refreshes(self, resolver):
        resolver._write_dir_listing("100", [{"name": "old.mkv", "fid": "1"}])
        resolver.mark_stale("100")
        resolver.client.list_files_all.return_value = [
            {"fn": "new.mkv", "fid": "2", "s": 500, "pc": "pc2"},
        ]
        items = resolver.listing("100")
        assert items[0]["name"] == "new.mkv"
        resolver.client.list_files_all.assert_called_once()


class TestUpdateAndRemove:
    def test_update_dir_entry(self, resolver):
        resolver._write_dir_listing("100", [])
        resolver.update_dir_entry("/影音", "100", "新目录", "200")
        index = resolver._read_path_index()
        assert index["/影音/新目录"]["cid"] == "200"
        listing = resolver._read_dir_listing("100")
        assert any(i["name"] == "新目录" for i in listing["items"])

    def test_remove_from_listing(self, resolver):
        resolver._write_dir_listing("100", [
            {"name": "a.mkv", "fid": "1"},
            {"name": "b.mkv", "fid": "2"},
        ])
        resolver.remove_from_listing("100", "a.mkv")
        listing = resolver._read_dir_listing("100")
        names = [i["name"] for i in listing["items"]]
        assert "a.mkv" not in names
        assert "b.mkv" in names

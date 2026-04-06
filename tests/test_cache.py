"""Cache module tests."""

import json
import os
import time

from media115 import cache


class TestGetPut:
    def test_put_and_get(self, tmp_path, monkeypatch):
        cache.put("tmdb", "movie_603", {"title": "The Matrix"})
        result = cache.get("tmdb", "movie_603")
        assert result["title"] == "The Matrix"
        assert "_cached_at" in result

    def test_get_missing(self, tmp_path, monkeypatch):
        assert cache.get("tmdb", "movie_999") is None

    def test_has(self, tmp_path, monkeypatch):
        assert not cache.has("tmdb", "movie_603")
        cache.put("tmdb", "movie_603", {"title": "x"})
        assert cache.has("tmdb", "movie_603")

    def test_overwrite(self, tmp_path, monkeypatch):
        cache.put("av", "ABC-123", {"title": "old"})
        cache.put("av", "ABC-123", {"title": "new"})
        assert cache.get("av", "ABC-123")["title"] == "new"


class TestMaxAge:
    def test_fresh_entry(self, tmp_path, monkeypatch):
        cache.put("test", "key1", {"v": 1})
        assert cache.get("test", "key1", max_age=60) is not None

    def test_expired_entry(self, tmp_path, monkeypatch):
        cache.put("test", "key2", {"v": 1})
        # Backdate the cached_at
        path = cache._cache_root() / "scrape" / "test" / "key2.json"
        data = json.loads(path.read_text())
        data["_cached_at"] = time.time() - 3700
        path.write_text(json.dumps(data))
        assert cache.get("test", "key2", max_age=3600) is None

    def test_no_max_age_ignores_time(self, tmp_path, monkeypatch):
        cache.put("test", "key3", {"v": 1})
        path = cache._cache_root() / "scrape" / "test" / "key3.json"
        data = json.loads(path.read_text())
        data["_cached_at"] = 0  # very old
        path.write_text(json.dumps(data))
        assert cache.get("test", "key3") is not None  # no max_age = never expires


class TestNotFound:
    def test_put_and_check(self, tmp_path, monkeypatch):
        cache.put_not_found("av", "XXX-999")
        assert cache.is_not_found("av", "XXX-999")

    def test_not_found_expires(self, tmp_path, monkeypatch):
        cache.put_not_found("av", "XXX-888")
        # Backdate past TTL
        path = cache._cache_root() / "scrape" / "av" / "XXX-888.json"
        data = json.loads(path.read_text())
        data["_cached_at"] = time.time() - cache.NOT_FOUND_TTL - 100
        path.write_text(json.dumps(data))
        assert not cache.is_not_found("av", "XXX-888")

    def test_not_found_distinct_from_success(self, tmp_path, monkeypatch):
        cache.put("av", "ABC-123", {"title": "found"})
        assert not cache.is_not_found("av", "ABC-123")


class TestTreeCachePath:
    def test_tree_cache_path(self, tmp_path, monkeypatch):
        p = cache.tree_cache_path()
        assert str(p).endswith("tree_cache.txt")
        assert "media115" in str(p)


class TestParseTreeCache:
    VIDEO_EXTS = {".mkv", ".mp4", ".avi"}

    def _write_tree(self, text: str):
        tree_path = cache.tree_cache_path()
        tree_path.write_text(text)

    def test_parse_tree_cache_basic(self, tmp_path, monkeypatch):
        # 115 tree format: root has no |- prefix, children use "| |-"
        tree_text = "root\n| |-movie.mkv\n| |-movie.nfo\n"
        self._write_tree(tree_text)
        entries = cache.parse_tree_cache(self.VIDEO_EXTS)
        assert len(entries) == 2

        video = [e for e in entries if e["is_video"]][0]
        assert video["n"] == "movie.mkv"
        assert video["is_video"] is True
        assert video["is_nfo"] is False
        assert "movie.mkv" in video["path"]

        nfo = [e for e in entries if e["is_nfo"]][0]
        assert nfo["n"] == "movie.nfo"
        assert nfo["is_nfo"] is True
        assert nfo["is_video"] is False
        assert "movie.nfo" in nfo["path"]

    def test_parse_tree_cache_nested(self, tmp_path, monkeypatch):
        # Matches SAMPLE_TREE format from test_cli.py
        tree_text = (
            "\u5f71\u97f3\n| |-\u7535\u5f71\n| | |-The Matrix (1999)\n| | | |-The Matrix.mkv\n"
        )
        self._write_tree(tree_text)
        entries = cache.parse_tree_cache(self.VIDEO_EXTS)
        assert len(entries) == 1
        e = entries[0]
        assert e["n"] == "The Matrix.mkv"
        assert e["parent"] == "\u7535\u5f71/The Matrix (1999)"
        assert e["path"] == "\u7535\u5f71/The Matrix (1999)/The Matrix.mkv"

    def test_parse_tree_cache_nfo_detection(self, tmp_path, monkeypatch):
        tree_text = (
            "dir\n"
            "| |-video.mkv\n"
            "| |-video.nfo\n"
            "| |-poster.jpg\n"
            "| |-fanart.png\n"
            "| |-subtitle.srt\n"
            "| |-another.mp4\n"
        )
        self._write_tree(tree_text)
        entries = cache.parse_tree_cache(self.VIDEO_EXTS)
        names = {e["n"] for e in entries}
        assert names == {"video.mkv", "video.nfo", "another.mp4"}
        assert all(e["is_video"] or e["is_nfo"] for e in entries)
        # jpg/png/srt should be excluded
        assert "poster.jpg" not in names
        assert "fanart.png" not in names
        assert "subtitle.srt" not in names

    def test_parse_tree_cache_empty(self, tmp_path, monkeypatch):
        self._write_tree("")
        entries = cache.parse_tree_cache(self.VIDEO_EXTS)
        assert entries == []

    def test_parse_tree_cache_no_file(self, tmp_path, monkeypatch):
        """When tree_cache.txt does not exist, returns empty list."""
        entries = cache.parse_tree_cache(self.VIDEO_EXTS)
        assert entries == []


class TestXDGPaths:
    def test_cache_dir_default(self, tmp_path, monkeypatch):
        """默认缓存目录是 ~/.cache/media115/。"""
        monkeypatch.setenv("HOME", str(tmp_path))
        monkeypatch.delenv("XDG_CACHE_HOME", raising=False)
        from media115.cache import _cache_root
        assert _cache_root() == tmp_path / ".cache" / "media115"

    def test_cache_dir_xdg_override(self, tmp_path, monkeypatch):
        """XDG_CACHE_HOME 覆盖默认路径。"""
        custom = tmp_path / "custom_cache"
        monkeypatch.setenv("XDG_CACHE_HOME", str(custom))
        from media115.cache import _cache_root
        assert _cache_root() == custom / "media115"

    def test_config_dir_default(self, tmp_path, monkeypatch):
        """默认配置目录是 ~/.config/media115/。"""
        monkeypatch.setenv("HOME", str(tmp_path))
        monkeypatch.delenv("XDG_CONFIG_HOME", raising=False)
        from media115.cache import _config_root
        assert _config_root() == tmp_path / ".config" / "media115"

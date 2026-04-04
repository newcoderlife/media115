"""Cache module tests."""

import json
import time

import pytest

from media115 import cache


class TestGetPut:
    def test_put_and_get(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put("tmdb", "movie_603", {"title": "The Matrix"})
        result = cache.get("tmdb", "movie_603")
        assert result["title"] == "The Matrix"
        assert "_cached_at" in result

    def test_get_missing(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        assert cache.get("tmdb", "movie_999") is None

    def test_has(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        assert not cache.has("tmdb", "movie_603")
        cache.put("tmdb", "movie_603", {"title": "x"})
        assert cache.has("tmdb", "movie_603")

    def test_overwrite(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put("av", "ABC-123", {"title": "old"})
        cache.put("av", "ABC-123", {"title": "new"})
        assert cache.get("av", "ABC-123")["title"] == "new"


class TestMaxAge:
    def test_fresh_entry(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put("test", "key1", {"v": 1})
        assert cache.get("test", "key1", max_age=60) is not None

    def test_expired_entry(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put("test", "key2", {"v": 1})
        # Backdate the cached_at
        path = tmp_path / ".cache" / "scrape" / "test" / "key2.json"
        data = json.loads(path.read_text())
        data["_cached_at"] = time.time() - 3700
        path.write_text(json.dumps(data))
        assert cache.get("test", "key2", max_age=3600) is None

    def test_no_max_age_ignores_time(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put("test", "key3", {"v": 1})
        path = tmp_path / ".cache" / "scrape" / "test" / "key3.json"
        data = json.loads(path.read_text())
        data["_cached_at"] = 0  # very old
        path.write_text(json.dumps(data))
        assert cache.get("test", "key3") is not None  # no max_age = never expires


class TestNotFound:
    def test_put_and_check(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put_not_found("av", "XXX-999")
        assert cache.is_not_found("av", "XXX-999")

    def test_not_found_expires(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put_not_found("av", "XXX-888")
        # Backdate past TTL
        path = tmp_path / ".cache" / "scrape" / "av" / "XXX-888.json"
        data = json.loads(path.read_text())
        data["_cached_at"] = time.time() - cache.NOT_FOUND_TTL - 100
        path.write_text(json.dumps(data))
        assert not cache.is_not_found("av", "XXX-888")

    def test_not_found_distinct_from_success(self, tmp_path, monkeypatch):
        monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))
        cache.put("av", "ABC-123", {"title": "found"})
        assert not cache.is_not_found("av", "ABC-123")

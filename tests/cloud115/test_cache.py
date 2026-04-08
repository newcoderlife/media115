"""FileCache tests — SQLite-backed path / directory / rate-limit cache."""
from __future__ import annotations

import time

import pytest

from cloud115.cache import FileCache, _default_db_path


# ---------------------------------------------------------------------------
# fixture
# ---------------------------------------------------------------------------

@pytest.fixture()
def cache(tmp_path):
    """Return a FileCache backed by an isolated database."""
    return FileCache(db_path=tmp_path / "test.db")


# ---------------------------------------------------------------------------
# _default_db_path
# ---------------------------------------------------------------------------

class TestDefaultDbPath:
    def test_returns_cache_db(self, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "xdg"))
        p = _default_db_path()
        assert p == tmp_path / "xdg" / "cloud115" / "cache.db"
        assert p.parent.is_dir()

    def test_falls_back_to_home(self, tmp_path, monkeypatch):
        monkeypatch.delenv("XDG_CACHE_HOME", raising=False)
        monkeypatch.setenv("HOME", str(tmp_path))
        p = _default_db_path()
        assert "cloud115" in str(p)
        assert str(p).endswith("cache.db")


# ---------------------------------------------------------------------------
# path_index
# ---------------------------------------------------------------------------

class TestPathIndex:
    def test_set_and_get(self, cache):
        cache.set_path("/movies/foo", "cid_1")
        result = cache.get_path("/movies/foo")
        assert result is not None
        cid, ts = result
        assert cid == "cid_1"
        assert ts <= int(time.time())

    def test_overwrite(self, cache):
        cache.set_path("/a", "old")
        cache.set_path("/a", "new")
        cid, _ = cache.get_path("/a")
        assert cid == "new"

    def test_get_missing_returns_none(self, cache):
        assert cache.get_path("/nonexistent") is None

    def test_delete_prefix_exact_and_children(self, cache):
        cache.set_path("/a/b", "1")
        cache.set_path("/a/b/c", "2")
        cache.set_path("/a/b/d", "3")
        cache.set_path("/a/x", "4")

        cache.delete_path_prefix("/a/b")

        # exact match and children gone
        assert cache.get_path("/a/b") is None
        assert cache.get_path("/a/b/c") is None
        assert cache.get_path("/a/b/d") is None
        # sibling untouched
        assert cache.get_path("/a/x") is not None

    def test_delete_prefix_does_not_hit_partial_match(self, cache):
        """Deleting prefix ``/a/b`` must NOT delete ``/ab``."""
        cache.set_path("/a/b", "1")
        cache.set_path("/ab", "2")

        cache.delete_path_prefix("/a/b")

        assert cache.get_path("/a/b") is None
        assert cache.get_path("/ab") is not None  # must survive


# ---------------------------------------------------------------------------
# dir listing
# ---------------------------------------------------------------------------

def _entry(name="file.txt", type_="file", node_id="n1", size=100, pick_code="pc1"):
    return {
        "name": name,
        "type": type_,
        "node_id": node_id,
        "size": size,
        "pick_code": pick_code,
    }


class TestDirListing:
    def test_set_and_get(self, cache):
        entries = [_entry("a.txt", node_id="1"), _entry("b.mp4", node_id="2")]
        cache.set_dir_listing("dir_cid", entries)

        result = cache.get_dir_entries("dir_cid")
        assert len(result) == 2
        names = [e["name"] for e in result]
        assert "a.txt" in names
        assert "b.mp4" in names

    def test_set_updates_dir_meta(self, cache):
        assert cache.get_dir_ts("d1") is None
        cache.set_dir_listing("d1", [_entry()])
        ts = cache.get_dir_ts("d1")
        assert ts is not None and ts <= int(time.time())

    def test_set_replaces_old_entries(self, cache):
        cache.set_dir_listing("d1", [_entry("old.txt", node_id="1")])
        cache.set_dir_listing("d1", [_entry("new.txt", node_id="2")])
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["name"] == "new.txt"

    def test_get_empty_returns_empty_list(self, cache):
        assert cache.get_dir_entries("nonexistent") == []

    def test_find_entry_found(self, cache):
        cache.set_dir_listing("d1", [_entry("target.txt", node_id="n9")])
        result = cache.find_entry("d1", "target.txt")
        assert result is not None
        assert result["node_id"] == "n9"

    def test_find_entry_missing(self, cache):
        cache.set_dir_listing("d1", [_entry()])
        assert cache.find_entry("d1", "nope.txt") is None

    def test_order_dirs_first_then_name(self, cache):
        entries = [
            _entry("beta.txt", type_="file", node_id="1"),
            _entry("alpha", type_="dir", node_id="2"),
            _entry("gamma", type_="dir", node_id="3"),
            _entry("aaa.txt", type_="file", node_id="4"),
        ]
        cache.set_dir_listing("d1", entries)
        result = cache.get_dir_entries("d1")
        types = [e["type"] for e in result]
        # dirs come first, then files
        assert types == ["dir", "dir", "file", "file"]
        # within each group, sorted by name
        assert result[0]["name"] == "alpha"
        assert result[1]["name"] == "gamma"
        assert result[2]["name"] == "aaa.txt"
        assert result[3]["name"] == "beta.txt"


# ---------------------------------------------------------------------------
# write-through
# ---------------------------------------------------------------------------

class TestWriteThrough:
    def test_add_entry(self, cache):
        cache.set_dir_listing("d1", [_entry("a.txt", node_id="1")])
        cache.add_entry("d1", _entry("b.txt", node_id="2"))
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 2

    def test_add_entry_upserts(self, cache):
        cache.set_dir_listing("d1", [_entry("a.txt", node_id="1", size=10)])
        cache.add_entry("d1", _entry("a.txt", node_id="1", size=99))
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["size"] == 99

    def test_remove_entry(self, cache):
        cache.set_dir_listing("d1", [
            _entry("keep.txt", node_id="1"),
            _entry("gone.txt", node_id="2"),
        ])
        cache.remove_entry("d1", "gone.txt")
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["name"] == "keep.txt"

    def test_rename_entry(self, cache):
        cache.set_dir_listing("d1", [_entry("old.txt", node_id="n1")])
        cache.rename_entry("d1", "n1", "new.txt")
        assert cache.find_entry("d1", "old.txt") is None
        assert cache.find_entry("d1", "new.txt") is not None

    def test_invalidate_dir(self, cache):
        cache.set_dir_listing("d1", [_entry()])
        assert cache.get_dir_ts("d1") is not None
        assert len(cache.get_dir_entries("d1")) == 1

        cache.invalidate_dir("d1")

        assert cache.get_dir_ts("d1") is None
        assert cache.get_dir_entries("d1") == []

    def test_delete_dir_meta_keeps_entries(self, cache):
        cache.set_dir_listing("d1", [_entry("f.txt", node_id="1")])
        assert cache.get_dir_ts("d1") is not None

        cache.delete_dir_meta("d1")

        # meta gone, entries remain
        assert cache.get_dir_ts("d1") is None
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["name"] == "f.txt"

    def test_add_entry_no_dir_meta(self, cache):
        """add_entry on a dir that was never listed — entry exists but dir_meta is None."""
        cache.add_entry("orphan_cid", {
            "name": "file.txt", "type": "file", "node_id": "f1",
            "size": 100, "pick_code": "pc1",
        })
        entry = cache.find_entry("orphan_cid", "file.txt")
        assert entry is not None
        assert entry["node_id"] == "f1"
        # dir_meta should be None (never listed from API)
        assert cache.get_dir_ts("orphan_cid") is None


# ---------------------------------------------------------------------------
# rate_limit
# ---------------------------------------------------------------------------

class TestRateLimit:
    def test_defaults(self, cache):
        rl = cache.get_rate_limit("api_list")
        assert rl == {
            "cooldown_until": 0.0,
            "last_request": 0.0,
            "minute_start": 0.0,
            "minute_count": 0,
        }

    def test_set_and_get(self, cache):
        cache.set_rate_limit("api_list", cooldown_until=100.0, last_request=99.0)
        rl = cache.get_rate_limit("api_list")
        assert rl["cooldown_until"] == 100.0
        assert rl["last_request"] == 99.0
        # untouched fields stay at 0
        assert rl["minute_count"] == 0

    def test_set_merges(self, cache):
        cache.set_rate_limit("x", cooldown_until=50.0)
        cache.set_rate_limit("x", last_request=70.0)
        rl = cache.get_rate_limit("x")
        assert rl["cooldown_until"] == 50.0
        assert rl["last_request"] == 70.0

    def test_cooldown(self, cache):
        future = time.time() + 3600
        cache.set_rate_limit("api_move", cooldown_until=future)
        rl = cache.get_rate_limit("api_move")
        assert rl["cooldown_until"] >= time.time()

    def test_increment_minute_count_creates_row(self, cache):
        cache.increment_minute_count("new_ep")
        rl = cache.get_rate_limit("new_ep")
        assert rl["minute_count"] == 1

    def test_increment_minute_count_bumps(self, cache):
        cache.set_rate_limit("ep", minute_count=5)
        cache.increment_minute_count("ep")
        assert cache.get_rate_limit("ep")["minute_count"] == 6

    def test_increment_preserves_other_fields(self, cache):
        cache.set_rate_limit("ep", cooldown_until=42.0, minute_count=3)
        cache.increment_minute_count("ep")
        rl = cache.get_rate_limit("ep")
        assert rl["minute_count"] == 4
        assert rl["cooldown_until"] == 42.0

    def test_unknown_kwarg_raises(self, cache):
        with pytest.raises(ValueError, match="Unknown rate_limit fields"):
            cache.set_rate_limit("api", cooldwon_until=5.0)


# ---------------------------------------------------------------------------
# management
# ---------------------------------------------------------------------------

class TestManagement:
    def test_clear(self, cache):
        cache.set_path("/a", "1")
        cache.set_dir_listing("d1", [_entry()])
        cache.set_rate_limit("x", cooldown_until=1.0)

        cache.clear()

        assert cache.get_path("/a") is None
        assert cache.get_dir_entries("d1") == []
        assert cache.get_dir_ts("d1") is None
        assert cache.get_rate_limit("x")["cooldown_until"] == 0.0

    def test_stats(self, cache):
        s = cache.stats()
        assert s["path_count"] == 0
        assert s["dir_count"] == 0
        assert s["entry_count"] == 0
        assert s["db_size_bytes"] >= 0

    def test_stats_after_inserts(self, cache):
        cache.set_path("/a", "1")
        cache.set_path("/b", "2")
        cache.set_dir_listing("d1", [_entry("x", node_id="1"), _entry("y", node_id="2")])

        s = cache.stats()
        assert s["path_count"] == 2
        assert s["dir_count"] == 1
        assert s["entry_count"] == 2
        assert s["db_size_bytes"] > 0

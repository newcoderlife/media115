"""FileCache tests — SQLite-backed path / directory / rate-limit cache."""
from __future__ import annotations

import sqlite3
import time

import pytest

from cloud115.cache import FileCache, _SCHEMA_VERSION, _default_db_path


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
    def test_double_close(self, cache):
        cache.close()
        cache.close()  # should not raise

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

    def test_stats_includes_rate_limit(self, cache):
        cache.set_rate_limit("api", cooldown_until=42.0, last_request=10.0, minute_count=3)
        s = cache.stats()
        assert "rate_limit" in s
        assert "api" in s["rate_limit"]
        assert s["rate_limit"]["api"]["cooldown_until"] == 42.0
        assert s["rate_limit"]["api"]["minute_count"] == 3


# ---------------------------------------------------------------------------
# clear_metadata
# ---------------------------------------------------------------------------

class TestClearMetadata:
    def test_keeps_rate_limit(self, cache):
        cache.set_path("/a", "1")
        cache.set_dir_listing("cid1", [
            {"name": "f", "type": "file", "node_id": "f1", "size": 1, "pick_code": "pc"},
        ])
        cache.set_rate_limit("api", minute_count=5)

        cache.clear_metadata()

        assert cache.get_path("/a") is None
        assert cache.get_dir_ts("cid1") is None
        state = cache.get_rate_limit("api")
        assert state["minute_count"] == 5


# ---------------------------------------------------------------------------
# try_acquire_slot
# ---------------------------------------------------------------------------

class TestTryAcquireSlot:
    def test_succeeds(self, cache):
        now = time.time()
        assert cache.try_acquire_slot("test", now, 1.0, 20) is True
        state = cache.get_rate_limit("test")
        assert state["minute_count"] == 1

    def test_fails_qpm(self, cache):
        now = time.time()
        cache.set_rate_limit("test", minute_start=now, minute_count=20, last_request=now - 5)
        assert cache.try_acquire_slot("test", now, 1.0, 20) is False

    def test_fails_cooldown(self, cache):
        now = time.time()
        cache.set_rate_limit("test", cooldown_until=now + 3600)
        assert cache.try_acquire_slot("test", now, 1.0, 20) is False

    def test_fails_qps(self, cache):
        now = time.time()
        cache.set_rate_limit("test", last_request=now - 0.5, minute_start=now - 10, minute_count=5)
        assert cache.try_acquire_slot("test", now, 2.0, 20) is False

    def test_resets_minute_window(self, cache):
        now = time.time()
        cache.set_rate_limit("test", minute_start=now - 120, minute_count=999)
        assert cache.try_acquire_slot("test", now, 0.0, 20) is True
        state = cache.get_rate_limit("test")
        assert state["minute_count"] == 1


# ---------------------------------------------------------------------------
# stale_dir_count
# ---------------------------------------------------------------------------

class TestStaleDirCount:
    def test_no_stale(self, cache):
        cache.set_dir_listing("fresh", [
            {"name": "f", "type": "file", "node_id": "f1", "size": 1, "pick_code": "pc"},
        ])
        assert cache.stale_dir_count(3600) == 0

    def test_with_stale(self, cache):
        cache.set_dir_listing("old", [
            {"name": "f", "type": "file", "node_id": "f1", "size": 1, "pick_code": "pc"},
        ])
        cache._conn.execute("UPDATE dir_meta SET ts = 0 WHERE cid = 'old'")
        cache._conn.commit()
        assert cache.stale_dir_count(3600) == 1


# ---------------------------------------------------------------------------
# tree_entry
# ---------------------------------------------------------------------------

def _tree_entry(path="AV/DANDY-001/DANDY-001.mkv", name="DANDY-001.mkv",
                parent="AV/DANDY-001", is_video=True, is_nfo=False):
    return {"path": path, "n": name, "parent": parent,
            "is_video": is_video, "is_nfo": is_nfo}


class TestTreeEntry:
    def test_set_and_get_all(self, cache):
        entries = [
            _tree_entry("AV/DANDY-001/DANDY-001.mkv", "DANDY-001.mkv", "AV/DANDY-001",
                        is_video=True, is_nfo=False),
            _tree_entry("AV/DANDY-001/DANDY-001.nfo", "DANDY-001.nfo", "AV/DANDY-001",
                        is_video=False, is_nfo=True),
            _tree_entry("电影/Dune (2021)/Dune (2021).mkv", "Dune (2021).mkv",
                        "电影/Dune (2021)", is_video=True, is_nfo=False),
        ]
        cache.set_tree(entries)

        result = cache.get_tree_entries()
        assert len(result) == 3
        paths = {r["path"] for r in result}
        assert "AV/DANDY-001/DANDY-001.mkv" in paths
        assert "AV/DANDY-001/DANDY-001.nfo" in paths
        assert "电影/Dune (2021)/Dune (2021).mkv" in paths

    def test_get_entries_fields(self, cache):
        cache.set_tree([_tree_entry()])
        result = cache.get_tree_entries()
        assert len(result) == 1
        e = result[0]
        assert e["path"] == "AV/DANDY-001/DANDY-001.mkv"
        assert e["n"] == "DANDY-001.mkv"
        assert e["parent"] == "AV/DANDY-001"
        assert e["is_video"] is True
        assert e["is_nfo"] is False

    def test_get_with_category_filter(self, cache):
        cache.set_tree([
            _tree_entry("AV/DANDY-001/DANDY-001.mkv", "DANDY-001.mkv",
                        "AV/DANDY-001", is_video=True, is_nfo=False),
            _tree_entry("电影/Dune (2021)/Dune (2021).mkv", "Dune (2021).mkv",
                        "电影/Dune (2021)", is_video=True, is_nfo=False),
        ])
        result = cache.get_tree_entries("AV")
        assert len(result) == 1
        assert result[0]["path"] == "AV/DANDY-001/DANDY-001.mkv"

    def test_set_tree_replaces_existing(self, cache):
        cache.set_tree([_tree_entry("old/file.mkv", "file.mkv", "old", is_video=True)])
        cache.set_tree([_tree_entry("new/film.mkv", "film.mkv", "new", is_video=True)])
        result = cache.get_tree_entries()
        assert len(result) == 1
        assert result[0]["path"] == "new/film.mkv"

    def test_set_tree_empty(self, cache):
        cache.set_tree([_tree_entry()])
        cache.set_tree([])
        assert cache.get_tree_entries() == []

    def test_tree_stats_empty(self, cache):
        stats = cache.tree_stats()
        assert stats == {"total": 0, "videos": 0, "nfos": 0}

    def test_tree_stats(self, cache):
        cache.set_tree([
            _tree_entry("AV/X/X.mkv", "X.mkv", "AV/X", is_video=True, is_nfo=False),
            _tree_entry("AV/X/X.nfo", "X.nfo", "AV/X", is_video=False, is_nfo=True),
            _tree_entry("电影/Y/Y.mkv", "Y.mkv", "电影/Y", is_video=True, is_nfo=False),
        ])
        stats = cache.tree_stats()
        assert stats["total"] == 3
        assert stats["videos"] == 2
        assert stats["nfos"] == 1

    def test_clear_tree(self, cache):
        cache.set_tree([_tree_entry()])
        cache.clear_tree()
        assert cache.get_tree_entries() == []

    def test_clear_metadata_includes_tree(self, cache):
        cache.set_tree([_tree_entry()])
        cache.set_path("/test", "c1")
        cache.clear_metadata()
        assert cache.get_tree_entries() == []
        assert cache.get_path("/test") is None

    def test_stats_includes_tree_entries(self, cache):
        cache.set_tree([
            _tree_entry("AV/X/X.mkv", "X.mkv", "AV/X", is_video=True, is_nfo=False),
        ])
        s = cache.stats()
        assert s["tree_entries"] == 1


# ---------------------------------------------------------------------------
# id-first PK: same-name files with different node_ids
# ---------------------------------------------------------------------------

class TestIdFirstPK:
    def test_same_name_different_node_ids_coexist(self, cache):
        """PK is (parent_cid, node_id) — same-name files must coexist."""
        e1 = _entry("dup.mkv", node_id="n1", size=100)
        e2 = _entry("dup.mkv", node_id="n2", size=200)
        cache.set_dir_listing("d1", [e1, e2])
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 2
        node_ids = {e["node_id"] for e in entries}
        assert node_ids == {"n1", "n2"}

    def test_add_entry_upserts_by_node_id(self, cache):
        """add_entry with same node_id updates rather than duplicates."""
        cache.add_entry("d1", _entry("a.txt", node_id="n1", size=10))
        cache.add_entry("d1", _entry("a.txt", node_id="n1", size=99))
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["size"] == 99

    def test_add_entry_different_node_id_creates_new_row(self, cache):
        """add_entry with different node_id creates a second row."""
        cache.add_entry("d1", _entry("dup.txt", node_id="n1", size=10))
        cache.add_entry("d1", _entry("dup.txt", node_id="n2", size=20))
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 2

    def test_find_entry_returns_first_match(self, cache):
        """find_entry still works when there are duplicates — returns first."""
        cache.set_dir_listing("d1", [
            _entry("dup.mkv", node_id="n1"),
            _entry("dup.mkv", node_id="n2"),
        ])
        result = cache.find_entry("d1", "dup.mkv")
        assert result is not None
        assert result["node_id"] in {"n1", "n2"}

    def test_find_entries_returns_all_matches(self, cache):
        """find_entries returns every entry with the given name."""
        cache.set_dir_listing("d1", [
            _entry("dup.mkv", node_id="n1", size=100),
            _entry("dup.mkv", node_id="n2", size=200),
            _entry("other.txt", node_id="n3"),
        ])
        results = cache.find_entries("d1", "dup.mkv")
        assert len(results) == 2
        node_ids = {r["node_id"] for r in results}
        assert node_ids == {"n1", "n2"}

    def test_find_entries_empty_when_not_found(self, cache):
        cache.set_dir_listing("d1", [_entry("a.txt", node_id="n1")])
        assert cache.find_entries("d1", "ghost.txt") == []

    def test_remove_entry_removes_all_same_name(self, cache):
        """remove_entry(name) removes ALL entries with that name."""
        cache.set_dir_listing("d1", [
            _entry("dup.mkv", node_id="n1"),
            _entry("dup.mkv", node_id="n2"),
            _entry("keep.txt", node_id="n3"),
        ])
        cache.remove_entry("d1", "dup.mkv")
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["name"] == "keep.txt"

    def test_remove_entry_by_id_targeted_removal(self, cache):
        """remove_entry_by_id removes exactly one entry."""
        cache.set_dir_listing("d1", [
            _entry("dup.mkv", node_id="n1"),
            _entry("dup.mkv", node_id="n2"),
        ])
        cache.remove_entry_by_id("d1", "n1")
        entries = cache.get_dir_entries("d1")
        assert len(entries) == 1
        assert entries[0]["node_id"] == "n2"

    def test_move_entry_updates_parent_cid(self, cache):
        """move_entry changes parent_cid atomically."""
        cache.set_dir_listing("src", [_entry("file.mkv", node_id="n1")])
        cache.set_dir_listing("dst", [])

        cache.move_entry("src", "n1", "dst")

        assert cache.find_entry("src", "file.mkv") is None
        result = cache.find_entry("dst", "file.mkv")
        assert result is not None
        assert result["node_id"] == "n1"

    def test_move_entry_noop_when_not_found(self, cache):
        """move_entry with unknown node_id is silently a no-op."""
        cache.set_dir_listing("src", [_entry("file.mkv", node_id="n1")])
        cache.move_entry("src", "nonexistent", "dst")
        # Original entry still in src
        assert cache.find_entry("src", "file.mkv") is not None


# ---------------------------------------------------------------------------
# DB migration: old schema → new schema
# ---------------------------------------------------------------------------

class TestDbMigration:
    def test_old_schema_is_migrated(self, tmp_path):
        """A DB with old PK (parent_cid, name) is silently wiped and recreated."""
        db_file = tmp_path / "old.db"

        # Build a v1 database with old PK and some data
        conn = sqlite3.connect(str(db_file))
        conn.execute("""
            CREATE TABLE dir_entry (
                parent_cid TEXT NOT NULL,
                name       TEXT NOT NULL,
                type       TEXT NOT NULL,
                node_id    TEXT NOT NULL,
                size       INTEGER,
                pick_code  TEXT,
                PRIMARY KEY (parent_cid, name)
            )
        """)
        conn.execute(
            "INSERT INTO dir_entry VALUES ('p1','f.txt','file','old_node',100,'pc')"
        )
        conn.execute("PRAGMA user_version = 1")
        conn.commit()
        conn.close()

        # Opening with FileCache must not raise and must reset to new schema
        cache = FileCache(db_file)
        try:
            # Old data gone (migration wiped tables)
            assert cache.find_entry("p1", "f.txt") is None
            # New schema accepts id-first operations
            cache.add_entry("p1", _entry("new.txt", node_id="nA"))
            cache.add_entry("p1", _entry("new.txt", node_id="nB"))
            assert len(cache.get_dir_entries("p1")) == 2
        finally:
            cache.close()

    def test_schema_version_is_set(self, tmp_path):
        """FileCache sets PRAGMA user_version to _SCHEMA_VERSION."""
        cache = FileCache(tmp_path / "v.db")
        version = cache._conn.execute("PRAGMA user_version").fetchone()[0]
        cache.close()
        assert version == _SCHEMA_VERSION

    def test_fresh_db_needs_no_migration(self, tmp_path):
        """Opening a brand-new DB twice does not wipe data."""
        db_file = tmp_path / "fresh.db"
        c1 = FileCache(db_file)
        c1.add_entry("d1", _entry("keep.txt", node_id="n1"))
        c1.close()

        c2 = FileCache(db_file)
        assert c2.find_entry("d1", "keep.txt") is not None
        c2.close()

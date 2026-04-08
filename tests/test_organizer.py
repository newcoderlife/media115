"""Organizer tests: SHA1, pre-SHA1, execute_organize_plan, _sanitize, _upload."""

import hashlib
from unittest.mock import MagicMock

import pytest

from media115.organizer import (
    compute_pre_sha1,
    compute_sha1,
)


@pytest.fixture
def sample_file(tmp_path):
    f = tmp_path / "test_video.mkv"
    data = b"x" * (1024 * 1024)
    f.write_bytes(data)
    return f


@pytest.fixture
def large_file(tmp_path):
    f = tmp_path / "large_video.mkv"
    chunk = b"a" * (1024 * 1024)
    with open(f, "wb") as fh:
        for _ in range(200):
            fh.write(chunk)
    return f


class TestComputeSha1:
    def test_small_file(self, sample_file):
        result = compute_sha1(sample_file)
        expected = hashlib.sha1(sample_file.read_bytes()).hexdigest()
        assert result == expected

    def test_consistent(self, sample_file):
        r1 = compute_sha1(sample_file)
        r2 = compute_sha1(sample_file)
        assert r1 == r2

    def test_hex_format(self, sample_file):
        result = compute_sha1(sample_file)
        assert len(result) == 40
        assert all(c in "0123456789abcdef" for c in result)


class TestComputePreSha1:
    def test_small_file_equals_full_sha1(self, sample_file):
        pre = compute_pre_sha1(sample_file)
        full = compute_sha1(sample_file)
        assert pre == full

    def test_large_file_differs_from_full(self, large_file):
        pre = compute_pre_sha1(large_file)
        full = compute_sha1(large_file)
        assert pre != full

    def test_large_file_only_reads_128mb(self, large_file):
        pre = compute_pre_sha1(large_file)
        with open(large_file, "rb") as f:
            data = f.read(128 * 1024 * 1024)
        expected = hashlib.sha1(data).hexdigest()
        assert pre == expected


# ── Mock cache for build_organize_plan tests ────────────────────────


class MockCache:
    """Dict-backed stand-in for media115.cache used by build_organize_plan."""

    def __init__(self):
        self._data = {}

    def get(self, source, key, max_age=None):
        return self._data.get(f"{source}/{key}")

    def put(self, source, key, data):
        self._data[f"{source}/{key}"] = data


# ── build_organize_plan tests ───────────────────────────────────────


class TestBuildOrganizePlanMovie:
    """test_build_plan_movie: movie file with a file_map entry."""

    def test_produces_correct_folder_and_name(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "Inception.2010.BluRay.1080p",
            {
                "type": "movie",
                "title": "Inception",
                "year": 2010,
                "tmdb_id": 27205,
            },
        )

        tree = [
            {
                "n": "Inception.2010.BluRay.1080p.mkv",
                "path": "影音/电影/some-raw-folder/Inception.2010.BluRay.1080p.mkv",
                "parent": "影音/电影/some-raw-folder",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电影", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "rename"
        assert op["new_folder"] == "Inception (2010)"
        assert op["new_name"] == "Inception (2010).mkv"
        assert op["type"] == "movie"


class TestBuildOrganizePlanAV:
    """test_build_plan_av: AV file with番号 naming including Part suffix."""

    def test_produces_number_based_naming(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "dandy-992",
            {
                "type": "av",
                "number": "DANDY-992",
                "title": "Some Title",
            },
        )

        tree = [
            {
                "n": "dandy-992.mp4",
                "path": "影音/AV/old-folder/dandy-992.mp4",
                "parent": "影音/AV/old-folder",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("AV", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "rename"
        assert op["new_folder"] == "DANDY-992"
        assert op["new_name"] == "DANDY-992.mp4"

    def test_preserves_part_suffix(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        # Filename stem differs from the番号-based name so rename is needed
        cache.put(
            "file_map",
            "abp123.Part1.hd",
            {
                "type": "av",
                "number": "ABP-123",
                "title": "Some Title",
            },
        )

        tree = [
            {
                "n": "abp123.Part1.hd.mkv",
                "path": "影音/AV/random/abp123.Part1.hd.mkv",
                "parent": "影音/AV/random",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("AV", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "rename"
        # Part suffix is detected from original filename and preserved
        assert op["new_name"] == "ABP-123.Part1.mkv"
        assert op["new_folder"] == "ABP-123"

    def test_part_suffix_only_folder_rename(self):
        """When filename already matches target but folder differs,
        only new_folder is set (new_name is None)."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "ABP-123.Part1",
            {
                "type": "av",
                "number": "ABP-123",
                "title": "Some Title",
            },
        )

        tree = [
            {
                "n": "ABP-123.Part1.mkv",
                "path": "影音/AV/random/ABP-123.Part1.mkv",
                "parent": "影音/AV/random",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("AV", tree, cache)
        op = ops[0]
        assert op["action"] == "rename"
        assert op["new_folder"] == "ABP-123"
        # Filename already correct, so new_name is None
        assert op["new_name"] is None

    def test_part2_suffix(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "ABP-123_Part2",
            {
                "type": "av",
                "number": "ABP-123",
                "title": "Some Title",
            },
        )

        tree = [
            {
                "n": "ABP-123_Part2.mp4",
                "path": "影音/AV/random/ABP-123_Part2.mp4",
                "parent": "影音/AV/random",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("AV", tree, cache)
        op = ops[0]
        assert op["new_name"] == "ABP-123.Part2.mp4"


class TestBuildOrganizePlanTV:
    """test_build_plan_tv: TV show with season/episode naming."""

    def test_produces_show_folder_and_episode_name(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "breaking.bad.s01e01.720p",
            {
                "type": "tv",
                "title": "Breaking Bad",
                "showtitle": "Breaking Bad",
                "year": 2008,
                "season": 1,
                "episode": 1,
            },
        )

        tree = [
            {
                "n": "breaking.bad.s01e01.720p.mkv",
                "path": "影音/电视剧/raw-folder/breaking.bad.s01e01.720p.mkv",
                "parent": "影音/电视剧/raw-folder",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电视剧", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "rename"
        assert op["new_folder"] == "Breaking Bad (2008)"
        assert op["new_name"] == "Breaking Bad S01E01.mkv"

    def test_tv_no_year(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "show.s02e03",
            {
                "type": "tv",
                "title": "Some Show",
                "showtitle": "Some Show",
                "season": 2,
                "episode": 3,
            },
        )

        tree = [
            {
                "n": "show.s02e03.mp4",
                "path": "影音/电视剧/old/show.s02e03.mp4",
                "parent": "影音/电视剧/old",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电视剧", tree, cache)
        op = ops[0]
        assert op["action"] == "rename"
        assert op["new_folder"] == "Some Show"
        assert op["new_name"] == "Some Show S02E03.mp4"

    def test_tv_already_correct(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "Breaking Bad S01E01",
            {
                "type": "tv",
                "title": "Breaking Bad",
                "showtitle": "Breaking Bad",
                "year": 2008,
                "season": 1,
                "episode": 1,
            },
        )

        tree = [
            {
                "n": "Breaking Bad S01E01.mkv",
                "path": "影音/电视剧/Breaking Bad (2008)/Breaking Bad S01E01.mkv",
                "parent": "影音/电视剧/Breaking Bad (2008)",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电视剧", tree, cache)
        op = ops[0]
        assert op["action"] == "skip"
        assert op["reason"] == "already correct"


class TestBuildOrganizePlanNoFileMap:
    """test_build_plan_no_file_map: files without file_map are skipped."""

    def test_skip_reason_no_scrape_result(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()  # empty -- no file_map entries

        tree = [
            {
                "n": "random_video.mkv",
                "path": "影音/电影/junk/random_video.mkv",
                "parent": "影音/电影/junk",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电影", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "skip"
        assert op["reason"] == "no scrape result cached"


class TestBuildOrganizePlanAlreadyCorrect:
    """test_build_plan_already_correct: files already in standard format."""

    def test_already_standard_format_no_file_map(self):
        """File matches 'Title (Year).ext' regex but has no file_map entry."""
        from media115.organizer import build_organize_plan

        cache = MockCache()

        tree = [
            {
                "n": "Inception (2010).mkv",
                "path": "影音/电影/Inception (2010)/Inception (2010).mkv",
                "parent": "影音/电影/Inception (2010)",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电影", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "skip"
        assert op["reason"] == "already in standard format"

    def test_already_correct_with_file_map(self):
        """File has file_map AND is already in the target folder/name."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "Inception (2010)",
            {
                "type": "movie",
                "title": "Inception",
                "year": 2010,
                "tmdb_id": 27205,
            },
        )

        tree = [
            {
                "n": "Inception (2010).mkv",
                "path": "影音/电影/Inception (2010)/Inception (2010).mkv",
                "parent": "影音/电影/Inception (2010)",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电影", tree, cache)
        assert len(ops) == 1
        op = ops[0]
        assert op["action"] == "skip"
        assert op["reason"] == "already correct"


class TestBuildOrganizePlanSourceId:
    """test_build_plan_source_id: source_id populated from file_map."""

    def test_movie_source_id_is_tmdb_id(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "Inception.2010.BluRay",
            {
                "type": "movie",
                "title": "Inception",
                "year": 2010,
                "tmdb_id": 27205,
            },
        )

        tree = [
            {
                "n": "Inception.2010.BluRay.mkv",
                "path": "影音/电影/raw/Inception.2010.BluRay.mkv",
                "parent": "影音/电影/raw",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电影", tree, cache)
        assert ops[0]["source_id"] == "27205"

    def test_av_source_id_is_number(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put(
            "file_map",
            "DANDY-992",
            {
                "type": "av",
                "number": "DANDY-992",
                "title": "Some Title",
            },
        )

        tree = [
            {
                "n": "DANDY-992.mp4",
                "path": "影音/AV/old/DANDY-992.mp4",
                "parent": "影音/AV/old",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("AV", tree, cache)
        assert ops[0]["source_id"] == "DANDY-992"

    def test_no_source_id_when_no_file_map(self):
        from media115.organizer import build_organize_plan

        cache = MockCache()

        tree = [
            {
                "n": "unknown.mkv",
                "path": "影音/电影/junk/unknown.mkv",
                "parent": "影音/电影/junk",
                "is_video": True,
                "is_nfo": False,
            }
        ]

        ops = build_organize_plan("电影", tree, cache)
        assert ops[0]["source_id"] == ""


# ── scrape_av file_map tests ────────────────────────────────────────


class TestScrapeAvWritesFileMap:
    """test_scrape_av_writes_file_map: scrape_av writes file_map on both
    cache-hit and first-scrape paths."""

    def test_first_scrape_writes_file_map(self, tmp_path, monkeypatch):
        """Fresh scrape via jav321 writes file_map cache entry."""
        from media115 import cache as real_cache
        from media115.scraper import scrape as scrape_mod

        # Make sure no existing AV cache or not_found cache
        assert real_cache.get("av", "TEST-001") is None

        # Mock jav321 to return valid metadata
        fake_meta = {
            "title": "Test Title",
            "number": "TEST-001",
            "release_date": "2023-01-15",
            "runtime": "120",
            "genres": ["Drama"],
            "director": "Director A",
            "actors": ["Actor A"],
            "studio": "Studio X",
            "cover_url": None,
        }
        monkeypatch.setattr("media115.scraper.jav321.fetch_metadata", lambda num: fake_meta)
        # javfree should not be called since jav321 succeeds
        monkeypatch.setattr("media115.scraper.javfree.fetch_metadata", lambda num: None)

        # Mock NFO generation to avoid file I/O side effects
        monkeypatch.setattr(scrape_mod, "_write_av_nfo", lambda *a, **kw: None)

        out_dir = tmp_path / "nfo_output"
        out_dir.mkdir()

        result = scrape_mod.scrape_av("TEST-001", "test-001.mp4", out_dir)
        assert result["status"] == "ok"
        assert result["number"] == "TEST-001"

        # Verify file_map was written
        fm = real_cache.get("file_map", "test-001")
        assert fm is not None
        assert fm["type"] == "av"
        assert fm["number"] == "TEST-001"
        assert fm["title"] == "Test Title"

    def test_cache_hit_writes_file_map(self, tmp_path, monkeypatch):
        """When AV metadata is already cached, file_map is still written."""
        from media115 import cache as real_cache
        from media115.scraper import scrape as scrape_mod

        # Pre-populate AV cache (simulating a previous scrape)
        real_cache.put(
            "av",
            "CACHED-002",
            {
                "title": "Cached Title",
                "number": "CACHED-002",
                "release_date": "2022-06-01",
                "_source": "jav321",
            },
        )

        # Mock NFO generation
        monkeypatch.setattr(scrape_mod, "_write_av_nfo", lambda *a, **kw: None)

        out_dir = tmp_path / "nfo_output"
        out_dir.mkdir()

        result = scrape_mod.scrape_av("CACHED-002", "cached-002.mkv", out_dir)
        assert result["status"] == "ok"

        # Verify file_map was written even on cache hit
        fm = real_cache.get("file_map", "cached-002")
        assert fm is not None
        assert fm["type"] == "av"
        assert fm["number"] == "CACHED-002"
        assert fm["title"] == "Cached Title"

    def test_fallback_to_javfree(self, tmp_path, monkeypatch):
        """When jav321 returns nothing, javfree is tried and file_map written."""
        from media115 import cache as real_cache
        from media115.scraper import scrape as scrape_mod

        # jav321 returns nothing
        monkeypatch.setattr("media115.scraper.jav321.fetch_metadata", lambda num: None)
        # javfree returns metadata
        fake_meta = {
            "title": "Javfree Title",
            "number": "JF-003",
            "release_date": "2024-03-20",
            "runtime": "90",
            "genres": [],
            "director": "",
            "actors": [],
            "studio": "",
            "cover_url": None,
        }
        monkeypatch.setattr("media115.scraper.javfree.fetch_metadata", lambda num: fake_meta)

        monkeypatch.setattr(scrape_mod, "_write_av_nfo", lambda *a, **kw: None)

        out_dir = tmp_path / "nfo_output"
        out_dir.mkdir()

        result = scrape_mod.scrape_av("JF-003", "jf-003.mp4", out_dir)
        assert result["status"] == "ok"

        fm = real_cache.get("file_map", "jf-003")
        assert fm is not None
        assert fm["type"] == "av"
        assert fm["number"] == "JF-003"
        assert fm["title"] == "Javfree Title"


# ── execute_organize_plan tests ────────────────────────────────────


def make_mock_client():
    """Create a mock CachedClient with sensible defaults."""
    client = MagicMock()
    client.resolve_path.return_value = "cat_cid_1"
    client.list_dir.return_value = [
        {"name": "OldDir", "type": "dir", "cid": "dir_cid_1"},
    ]
    client.mkdir.return_value = "new_cid_1"
    client.move.return_value = None
    client.batch_rename.return_value = None
    client.delete.return_value = None
    client.upload.return_value = None
    return client


def _make_op(
    *,
    file="old.mkv",
    path="影音/电影/OldDir/old.mkv",
    parent="影音/电影/OldDir",
    type_="movie",
    action="rename",
    new_folder="New Title (2024)",
    new_name="New Title (2024).mkv",
    source_id="12345",
    reason="",
):
    return {
        "file": file,
        "path": path,
        "parent": parent,
        "type": type_,
        "action": action,
        "new_folder": new_folder,
        "new_name": new_name,
        "source_id": source_id,
        "reason": reason,
    }


class TestExecuteOrganizePlanBasic:
    """test_execute_organize_plan_basic: new_folder + new_name, full happy path."""

    def test_mkdir_move_rename_called(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        # Phase 1: list_dir for parent dir returns the file
        # Phase 6: list_dir for category + OldDir cleanup
        client.list_dir.side_effect = [
            # First call: list files inside OldDir (Phase 1)
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
            # Second call: Phase 6 category listing
            [{"name": "OldDir", "type": "dir", "cid": "dir_cid_1"}],
            # Third call: Phase 6 list OldDir contents (no videos)
            [{"name": "poster.jpg", "type": "file", "fid": "fid_poster"}],
        ]

        # Stub _plan_scrape_upload to avoid filesystem access (no sidecar files)
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "ok"
        client.mkdir.assert_called_once_with("/影音/电影/New Title (2024)")
        client.move.assert_called_once_with(
            ["/影音/电影/OldDir/old.mkv"], "/影音/电影/New Title (2024)"
        )
        client.batch_rename.assert_called_once_with(
            [("/影音/电影/New Title (2024)/old.mkv", "New Title (2024).mkv")]
        )


class TestExecuteOrganizePlanRenameOnly:
    """test_execute_organize_plan_rename_only: new_name set, new_folder=None."""

    def test_no_mkdir_no_move(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_dir.side_effect = [
            # Phase 1: list files inside OldDir
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
        ]
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op(new_folder=None, new_name="New Title (2024).mkv")]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "ok"
        client.mkdir.assert_not_called()
        client.move.assert_not_called()
        client.batch_rename.assert_called_once_with(
            [("/影音/电影/OldDir/old.mkv", "New Title (2024).mkv")]
        )


class TestExecuteOrganizePlanMoveFailure:
    """test_execute_organize_plan_move_failure: move raises, tracked as failed."""

    def test_failed_ops_tracked(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_dir.side_effect = [
            # Phase 1: list files inside OldDir
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
            # Phase 6: category listing (OldDir still has video since move failed)
            [{"name": "OldDir", "type": "dir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir contents
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
        ]
        client.move.side_effect = Exception("115 API error")

        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "error"
        assert "move or rename failed" in results[0]["error"]
        # Rename should NOT be attempted for a file that failed to move
        client.batch_rename.assert_not_called()


class TestExecuteOrganizePlanRenameFailure:
    """test_execute_organize_plan_rename_failure: batch_rename raises."""

    def test_status_error(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_dir.side_effect = [
            # Phase 1: list files inside OldDir
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
        ]
        # No new_folder so no move happens; only rename
        client.batch_rename.side_effect = Exception("rename API error")

        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op(new_folder=None, new_name="New Title (2024).mkv")]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "error"
        assert "move or rename failed" in results[0]["error"]


class TestExecuteOrganizePlanPhase6Cleanup:
    """test_execute_organize_plan_phase6_cleanup: old dir deleted when no video."""

    def test_delete_called(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_dir.side_effect = [
            # Phase 1: files in OldDir
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
            # Phase 6: category listing
            [{"name": "OldDir", "type": "dir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir contents -- only metadata, no video
            [{"name": "poster.jpg", "type": "file", "fid": "fid_poster"}],
        ]
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert results[0]["status"] == "ok"
        client.delete.assert_called_once_with(["/影音/电影/OldDir"])


class TestExecuteOrganizePlanPhase6SkipVideo:
    """test_execute_organize_plan_phase6_skip_video: old dir still has video."""

    def test_delete_not_called(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_dir.side_effect = [
            # Phase 1: list files inside OldDir
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
            # Phase 6: category listing
            [{"name": "OldDir", "type": "dir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir still has a video file
            [{"name": "other_video.mp4", "type": "file", "fid": "fid_other"}],
        ]
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert results[0]["status"] == "ok"
        client.delete.assert_not_called()


class TestExecuteOrganizePlanNotFound:
    """test_execute_organize_plan_not_found: file not in directory listing."""

    def test_status_not_found(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_dir.side_effect = [
            # Files in OldDir -- does NOT include old.mkv
            [{"name": "different.mkv", "type": "file", "fid": "fid_other"}],
        ]
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "not_found"
        assert "file not in dir" in results[0]["error"]


class TestExecuteOrganizePlanCategoryNotFound:
    """test_execute_organize_plan_category_not_found: resolve_path raises."""

    def test_all_ops_error(self, monkeypatch):
        from media115.organizer import execute_organize_plan

        client = make_mock_client()
        client.resolve_path.side_effect = FileNotFoundError("not found")

        ops = [
            _make_op(),
            _make_op(file="second.mkv", new_name="Second (2024).mkv"),
        ]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 2
        for r in results:
            assert r["status"] == "error"
            assert "category dir not found" in r["error"]


class TestExecuteOrganizePlanSkipAction:
    """test_execute_organize_plan_skip: skip ops are returned as skipped."""

    def test_skip_ops_skipped(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op(action="skip", new_folder=None, new_name=None, reason="already correct")]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "skipped"


class TestExecuteOrganizePlanMkdirException:
    """test_execute_organize_plan_mkdir_exception: mkdir raises, fallback to resolve_path."""

    def test_mkdir_exception_fallback(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.mkdir.side_effect = Exception("already exists")
        # resolve_path succeeds for both category and the target folder
        client.resolve_path.return_value = "existing_cid_1"
        client.list_dir.side_effect = [
            # Phase 1: files in OldDir
            [{"name": "old.mkv", "type": "file", "fid": "fid_1"}],
            # Phase 6: category listing
            [{"name": "OldDir", "type": "dir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir contents
            [{"name": "poster.jpg", "type": "file", "fid": "fid_poster"}],
        ]
        monkeypatch.setattr("media115.organizer._plan_scrape_upload", lambda *a, **kw: [])

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert results[0]["status"] == "ok"
        # Move uses path-based API
        client.move.assert_called_once_with(
            ["/影音/电影/OldDir/old.mkv"], "/影音/电影/New Title (2024)"
        )


class TestSanitize:
    """test_sanitize: _sanitize removes invalid filename chars."""

    def test_removes_invalid_chars(self):
        from media115.organizer import _sanitize

        assert _sanitize('Movie: "The Best" <2024>') == "Movie The Best 2024"

    def test_preserves_valid_chars(self):
        from media115.organizer import _sanitize

        assert _sanitize("Hello World (2024)") == "Hello World (2024)"

    def test_strips_whitespace(self):
        from media115.organizer import _sanitize

        assert _sanitize("  title  ") == "title"

    def test_removes_all_special(self):
        from media115.organizer import _sanitize

        assert _sanitize('<>:"/\\|?*') == ""


class TestUploadScrapeOutput:
    """test_upload_scrape_output: upload NFO + poster from scrape_output."""

    def test_uploads_nfo_and_poster(self, tmp_path, monkeypatch):
        from media115.cache import _cache_root
        from media115.organizer import _upload_scrape_output

        # Create scrape_output directory structure in the XDG cache location:
        # <XDG_CACHE_HOME>/media115/scrape_output/<category>/<parent_path_with_underscores>/
        scrape_dir = _cache_root() / "scrape_output" / "movie" / "影音_电影_OldDir"
        scrape_dir.mkdir(parents=True)
        nfo_file = scrape_dir / "old.nfo"
        nfo_file.write_text("<movie><title>Test</title></movie>")
        poster_file = scrape_dir / "poster.jpg"
        poster_file.write_bytes(b"\xff\xd8fake-jpg")

        client = MagicMock()
        # list_dir returns empty (no existing files to delete)
        client.list_dir.return_value = []
        op = {
            "file": "old.mkv",
            "parent": "影音/电影/OldDir",
            "new_name": "New Title (2024).mkv",
        }

        target_dir = "/影音/电影/New Title (2024)"
        _upload_scrape_output(client, op, target_dir, "New Title (2024).mkv")

        assert client.upload.call_count == 2
        # Collect the remote names used in upload calls
        upload_calls = client.upload.call_args_list
        remote_names = set()
        for call in upload_calls:
            # upload(local_path, remote_dir, filename)
            remote_names.add(call.args[2] if len(call.args) > 2 else call.kwargs.get("filename", ""))
        # NFO should be renamed to match the new video filename
        assert "New Title (2024).nfo" in remote_names
        # Poster keeps its original name
        assert "poster.jpg" in remote_names

    def test_no_scrape_dir_is_noop(self, tmp_path, monkeypatch):
        from media115.organizer import _upload_scrape_output

        # No scrape_output directory exists in the isolated XDG cache
        client = MagicMock()
        op = {"file": "old.mkv", "parent": "影音/电影/OldDir"}
        _upload_scrape_output(client, op, "/影音/电影/Target", "New Title (2024).mkv")

        client.upload.assert_not_called()

    def test_existing_files_batch_deleted(self, tmp_path, monkeypatch):
        """When old sidecars exist, _upload_scrape_output issues ONE batch delete."""
        from media115.cache import _cache_root
        from media115.organizer import _upload_scrape_output

        scrape_dir = _cache_root() / "scrape_output" / "movie" / "影音_电影_OldDir"
        scrape_dir.mkdir(parents=True)
        (scrape_dir / "old.nfo").write_text("<movie/>")
        (scrape_dir / "poster.jpg").write_bytes(b"\xff\xd8fake")

        client = MagicMock()
        # Both sidecar names already exist remotely
        client.list_dir.return_value = [
            {"type": "file", "name": "New Title (2024).nfo"},
            {"type": "file", "name": "poster.jpg"},
        ]
        op = {"file": "old.mkv", "parent": "影音/电影/OldDir"}
        target_dir = "/影音/电影/New Title (2024)"

        _upload_scrape_output(client, op, target_dir, "New Title (2024).mkv")

        # One batch delete call (not two individual calls)
        client.delete.assert_called_once()
        deleted_paths = client.delete.call_args[0][0]
        assert len(deleted_paths) == 2
        assert all(p.startswith(target_dir) for p in deleted_paths)
        # Both uploads still happen
        assert client.upload.call_count == 2


class TestPlanScrapeUpload:
    """_plan_scrape_upload: pure filesystem planner, no API calls."""

    def test_returns_pairs_for_nfo_and_poster(self, tmp_path):
        from media115.cache import _cache_root
        from media115.organizer import _plan_scrape_upload

        scrape_dir = _cache_root() / "scrape_output" / "movie" / "影音_电影_OldDir"
        scrape_dir.mkdir(parents=True)
        nfo_file = scrape_dir / "old.nfo"
        nfo_file.write_text("<movie/>")
        poster_file = scrape_dir / "poster.jpg"
        poster_file.write_bytes(b"fake")

        op = {"file": "old.mkv", "parent": "影音/电影/OldDir"}
        pairs = _plan_scrape_upload(op, "New Title (2024).mkv")

        assert len(pairs) == 2
        remote_names = {remote for _, remote in pairs}
        assert "New Title (2024).nfo" in remote_names
        assert "poster.jpg" in remote_names

    def test_no_scrape_dir_returns_empty(self):
        from media115.organizer import _plan_scrape_upload

        op = {"file": "nonexistent.mkv", "parent": "影音/电影/NoSuchDir"}
        pairs = _plan_scrape_upload(op, "Whatever (2024).mkv")
        assert pairs == []

    def test_nfo_without_new_video_name_keeps_original(self):
        """When new_video_name is None, NFO keeps its original filename."""
        from media115.cache import _cache_root
        from media115.organizer import _plan_scrape_upload

        scrape_dir = _cache_root() / "scrape_output" / "av" / "影音_AV_ABC-001"
        scrape_dir.mkdir(parents=True, exist_ok=True)
        (scrape_dir / "ABC-001.nfo").write_text("<movie/>")

        op = {"file": "ABC-001.mkv", "parent": "影音/AV/ABC-001"}
        pairs = _plan_scrape_upload(op, new_video_name=None)

        assert len(pairs) == 1
        _, remote_name = pairs[0]
        assert remote_name == "ABC-001.nfo"


def test_plan_scrape_upload_filters_by_video_stem(tmp_path, monkeypatch):
    """Only include NFOs matching the video's stem, not all NFOs in the dir."""
    from media115.organizer import _plan_scrape_upload
    from media115 import cache as media_cache

    # Create scrape_output with multiple episode NFOs
    scrape_dir = tmp_path / "scrape_output" / "剧目" / "剧目_ShowFolder"
    scrape_dir.mkdir(parents=True)
    (scrape_dir / "Show S01E01.nfo").write_text("<episode/>")
    (scrape_dir / "Show S01E02.nfo").write_text("<episode/>")
    (scrape_dir / "tvshow.nfo").write_text("<tvshow/>")
    (scrape_dir / "poster.jpg").write_bytes(b"img")

    monkeypatch.setattr(media_cache, "_cache_root", lambda: tmp_path)

    op = {"file": "Show S01E01.mkv", "parent": "剧目/ShowFolder"}
    result = _plan_scrape_upload(op, "NewShow S01E01.mkv")

    names = [name for _, name in result]
    assert "NewShow S01E01.nfo" in names   # matched NFO, renamed
    assert "tvshow.nfo" in names            # shared, kept as-is
    assert "poster.jpg" in names            # shared, kept as-is
    # Episode 2's NFO should NOT be included
    nfo_names = [n for n in names if n.endswith(".nfo")]
    assert len(nfo_names) == 2  # only ep1 NFO + tvshow.nfo
    assert "NewShow S01E02.nfo" not in names


def test_phase5_deduplicates_shared_sidecars(tmp_path, monkeypatch):
    """tvshow.nfo and poster.jpg should only be uploaded once per target dir."""
    from media115.organizer import _plan_scrape_upload
    from media115 import cache as media_cache

    scrape_dir = tmp_path / "scrape_output" / "剧目" / "剧目_ShowFolder"
    scrape_dir.mkdir(parents=True)
    (scrape_dir / "Show S01E01.nfo").write_text("<ep1/>")
    (scrape_dir / "Show S01E02.nfo").write_text("<ep2/>")
    (scrape_dir / "tvshow.nfo").write_text("<tvshow/>")
    (scrape_dir / "poster.jpg").write_bytes(b"img")

    monkeypatch.setattr(media_cache, "_cache_root", lambda: tmp_path)

    # Simulate what Phase 5 does: call _plan_scrape_upload for each episode
    op1 = {"file": "Show S01E01.mkv", "parent": "剧目/ShowFolder"}
    op2 = {"file": "Show S01E02.mkv", "parent": "剧目/ShowFolder"}

    pairs1 = _plan_scrape_upload(op1, "NewShow S01E01.mkv")
    pairs2 = _plan_scrape_upload(op2, "NewShow S01E02.mkv")

    # Simulate Phase 5 dedup logic
    upload_groups: dict = {}
    target = "/影音/剧目/NewShow (2024)"
    existing = upload_groups.setdefault(target, [])
    existing_names = {name for _, name in existing}
    for local_path, remote_name in pairs1:
        if remote_name not in existing_names:
            existing.append((local_path, remote_name))
            existing_names.add(remote_name)
    for local_path, remote_name in pairs2:
        if remote_name not in existing_names:
            existing.append((local_path, remote_name))
            existing_names.add(remote_name)

    names = [name for _, name in upload_groups[target]]
    # Each episode NFO appears once
    assert names.count("NewShow S01E01.nfo") == 1
    assert names.count("NewShow S01E02.nfo") == 1
    # Shared files appear only once
    assert names.count("tvshow.nfo") == 1
    assert names.count("poster.jpg") == 1


# ── _extract_av_suffix tests ────────────────────────────────────────


class TestExtractAvSuffix:
    def test_cut_version(self):
        from media115.organizer import _extract_av_suffix
        assert _extract_av_suffix("-C") == "-C"
        assert _extract_av_suffix("-C.mp4") == "-C"

    def test_multi_disc(self):
        from media115.organizer import _extract_av_suffix
        assert _extract_av_suffix("A.FHD") == ".A"
        assert _extract_av_suffix("B_4K^WM") == ".B"
        assert _extract_av_suffix("A") == ".A"

    def test_part(self):
        from media115.organizer import _extract_av_suffix
        assert _extract_av_suffix(".Part1") == ".Part1"
        assert _extract_av_suffix("_Part2") == ".Part2"

    def test_quality_only(self):
        from media115.organizer import _extract_av_suffix
        assert _extract_av_suffix(".FHD") == ""
        assert _extract_av_suffix(".HD") == ""
        assert _extract_av_suffix(".2160p.DMM.WEB-DL.AAC2.0.H.264-MTeam") == ""

    def test_empty(self):
        from media115.organizer import _extract_av_suffix
        assert _extract_av_suffix("") == ""


# ── AV organize plan preserves content suffixes ─────────────────────


class TestAvOrganizePreservesSuffixes:
    """AV organize should preserve -C, A/B, Part suffixes."""

    def _make_tree(self, filename, folder="影音/AV/raw-folder"):
        return [
            {
                "n": filename,
                "path": f"{folder}/{filename}",
                "parent": folder,
                "is_video": True,
                "is_nfo": False,
            }
        ]

    def test_cut_version_preserved(self):
        """ABP-612-C.mp4 should stay ABP-612-C.mp4 (not renamed to ABP-612.mp4).
        The file is already correctly named so new_name is None; only folder changes."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put("file_map", "ABP-612-C", {"type": "av", "number": "ABP-612"})

        ops = build_organize_plan("AV", self._make_tree("ABP-612-C.mp4"), cache)
        op = ops[0]
        assert op["action"] == "rename"
        assert op["new_folder"] == "ABP-612"
        # File is already named ABP-612-C.mp4 so no rename needed
        assert op["new_name"] is None

    def test_fhd_suffix_stripped(self):
        """YRH-093A.FHD.wmv: A (disc) preserved, FHD stripped."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put("file_map", "YRH-093A.FHD", {"type": "av", "number": "YRH-093"})

        ops = build_organize_plan("AV", self._make_tree("YRH-093A.FHD.wmv"), cache)
        assert ops[0]["new_name"] == "YRH-093.A.wmv"

    def test_multi_disc_b_preserved(self):
        """STARS-685A_4K^WM.mp4: A preserved, quality markers stripped."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put("file_map", "STARS-685A_4K^WM", {"type": "av", "number": "STARS-685"})

        ops = build_organize_plan("AV", self._make_tree("STARS-685A_4K^WM.mp4"), cache)
        assert ops[0]["new_name"] == "STARS-685.A.mp4"

    def test_part_suffix_still_preserved(self):
        """Part suffix still preserved via new code path."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put("file_map", "ABP-123.Part1.hd", {"type": "av", "number": "ABP-123"})

        ops = build_organize_plan("AV", self._make_tree("ABP-123.Part1.hd.mkv"), cache)
        assert ops[0]["new_name"] == "ABP-123.Part1.mkv"

    def test_plain_number_no_suffix(self):
        """dandy-992.mp4 with no extra suffix stays dandy-992.mp4."""
        from media115.organizer import build_organize_plan

        cache = MockCache()
        cache.put("file_map", "dandy-992", {"type": "av", "number": "DANDY-992"})

        ops = build_organize_plan("AV", self._make_tree("dandy-992.mp4"), cache)
        assert ops[0]["new_name"] == "DANDY-992.mp4"

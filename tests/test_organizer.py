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

        # Point cache dir to tmp_path so we don't pollute the real cache
        monkeypatch.setattr(real_cache, "CACHE_DIR", str(tmp_path / ".cache"))

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

        monkeypatch.setattr(real_cache, "CACHE_DIR", str(tmp_path / ".cache"))

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

        monkeypatch.setattr(real_cache, "CACHE_DIR", str(tmp_path / ".cache"))

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
    client = MagicMock()
    client.get_dir_id.return_value = "cat_cid_1"
    client.list_files_all.return_value = [
        {"n": "OldDir", "cid": "dir_cid_1"},  # subdirectory
    ]
    client.mkdir.return_value = {"cid": "new_cid_1"}
    client.move.return_value = True
    client.batch_rename.return_value = True
    client.list_files.return_value = []
    client.delete.return_value = True
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
        # Phase 1: category listing returns the old subdirectory
        # Phase 1 cont: list files inside OldDir to find the file's fid
        client.list_files_all.side_effect = [
            # First call: category dir listing (subdirs)
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Second call: list files inside OldDir
            [{"n": "old.mkv", "fid": "fid_1"}],
            # Third call: Phase 6 refresh category listing
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Fourth call: Phase 6 list OldDir contents (no videos)
            [{"n": "poster.jpg", "fid": "fid_poster"}],
        ]

        # Stub _upload_scrape_output to avoid filesystem access
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "ok"
        client.mkdir.assert_called_once_with("cat_cid_1", "New Title (2024)")
        client.move.assert_called_once_with(["fid_1"], "new_cid_1")
        client.batch_rename.assert_called_once_with({"fid_1": "New Title (2024).mkv"})


class TestExecuteOrganizePlanRenameOnly:
    """test_execute_organize_plan_rename_only: new_name set, new_folder=None."""

    def test_no_mkdir_no_move(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "old.mkv", "fid": "fid_1"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op(new_folder=None, new_name="New Title (2024).mkv")]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "ok"
        client.mkdir.assert_not_called()
        client.move.assert_not_called()
        client.batch_rename.assert_called_once_with({"fid_1": "New Title (2024).mkv"})


class TestExecuteOrganizePlanMoveFailure:
    """test_execute_organize_plan_move_failure: move raises, tracked as failed."""

    def test_failed_fids_tracked(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "old.mkv", "fid": "fid_1"}],
            # Phase 6: refreshed category listing
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir contents (still has the file since move failed)
            [{"n": "old.mkv", "fid": "fid_1"}],
        ]
        client.move.side_effect = Exception("115 API error")

        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

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
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "old.mkv", "fid": "fid_1"}],
        ]
        # No new_folder so no move happens; only rename
        client.batch_rename.side_effect = Exception("rename API error")

        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

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
        client.list_files_all.side_effect = [
            # Phase 1: category listing
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Phase 1: files in OldDir
            [{"n": "old.mkv", "fid": "fid_1"}],
            # Phase 6: refreshed category listing
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir contents -- only metadata, no video
            [{"n": "poster.jpg", "fid": "fid_poster"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert results[0]["status"] == "ok"
        client.delete.assert_called_once_with(["dir_cid_1"])


class TestExecuteOrganizePlanPhase6SkipVideo:
    """test_execute_organize_plan_phase6_skip_video: old dir still has video."""

    def test_delete_not_called(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "old.mkv", "fid": "fid_1"}],
            # Phase 6: refreshed listing
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Phase 6: OldDir still has a video file
            [{"n": "other_video.mp4", "fid": "fid_other"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

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
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            # Files in OldDir -- does NOT include old.mkv
            [{"n": "different.mkv", "fid": "fid_other"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "not_found"
        assert "file not in dir" in results[0]["error"]


class TestExecuteOrganizePlanCategoryNotFound:
    """test_execute_organize_plan_category_not_found: get_dir_id returns None."""

    def test_all_ops_error(self, monkeypatch):
        from media115.organizer import execute_organize_plan

        client = make_mock_client()
        client.get_dir_id.return_value = None

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
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op(action="skip", new_folder=None, new_name=None, reason="already correct")]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert len(results) == 1
        assert results[0]["status"] == "skipped"


class TestExecuteOrganizePlanMkdirFallback:
    """test_execute_organize_plan_mkdir_fallback: mkdir returns no cid, fallback to get_dir_id."""

    def test_mkdir_no_cid_fallback(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        # mkdir returns empty cid, forcing fallback
        client.mkdir.return_value = {}
        # get_dir_id: first call returns category_cid, second call returns
        # the existing folder cid for the fallback
        client.get_dir_id.side_effect = ["cat_cid_1", "existing_cid_1"]
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "old.mkv", "fid": "fid_1"}],
            # Phase 6: refreshed listing
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "poster.jpg", "fid": "fid_poster"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert results[0]["status"] == "ok"
        # Move should use the fallback cid
        client.move.assert_called_once_with(["fid_1"], "existing_cid_1")


class TestExecuteOrganizePlanMkdirException:
    """test_execute_organize_plan_mkdir_exception: mkdir raises, fallback to get_dir_id."""

    def test_mkdir_exception_fallback(self, monkeypatch):
        from media115 import cache as _cache
        from media115.organizer import execute_organize_plan

        monkeypatch.setattr(_cache, "get", lambda *a, **kw: None)
        monkeypatch.setattr(_cache, "put", lambda *a, **kw: None)

        client = make_mock_client()
        client.mkdir.side_effect = Exception("already exists")
        client.get_dir_id.side_effect = ["cat_cid_1", "existing_cid_1"]
        client.list_files_all.side_effect = [
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "old.mkv", "fid": "fid_1"}],
            # Phase 6
            [{"n": "OldDir", "cid": "dir_cid_1"}],
            [{"n": "poster.jpg", "fid": "fid_poster"}],
        ]
        monkeypatch.setattr("media115.organizer._upload_scrape_output", lambda *a, **kw: None)

        ops = [_make_op()]
        results = execute_organize_plan(ops, client, "影音/电影")

        assert results[0]["status"] == "ok"
        client.move.assert_called_once_with(["fid_1"], "existing_cid_1")


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
        from media115.organizer import _upload_scrape_output

        # Set cwd to tmp_path so the function finds .cache/scrape_output
        monkeypatch.chdir(tmp_path)

        # Create scrape_output directory structure:
        # .cache/scrape_output/<category>/<parent_path_with_underscores>/
        scrape_dir = tmp_path / ".cache" / "scrape_output" / "movie" / "影音_电影_OldDir"
        scrape_dir.mkdir(parents=True)
        nfo_file = scrape_dir / "old.nfo"
        nfo_file.write_text("<movie><title>Test</title></movie>")
        poster_file = scrape_dir / "poster.jpg"
        poster_file.write_bytes(b"\xff\xd8fake-jpg")

        client = MagicMock()
        op = {
            "file": "old.mkv",
            "parent": "影音/电影/OldDir",
            "new_name": "New Title (2024).mkv",
        }

        _upload_scrape_output(client, op, "target_cid_1", "New Title (2024).mkv")

        assert client.upload_file.call_count == 2
        # Collect the remote names used in upload calls
        remote_names = {call.args[2] for call in client.upload_file.call_args_list}
        # NFO should be renamed to match the new video filename
        assert "New Title (2024).nfo" in remote_names
        # Poster keeps its original name
        assert "poster.jpg" in remote_names

    def test_no_scrape_dir_is_noop(self, tmp_path, monkeypatch):
        from media115.organizer import _upload_scrape_output

        monkeypatch.chdir(tmp_path)
        # No .cache/scrape_output directory exists

        client = MagicMock()
        op = {"file": "old.mkv", "parent": "影音/电影/OldDir"}
        _upload_scrape_output(client, op, "target_cid_1", "New Title (2024).mkv")

        client.upload_file.assert_not_called()

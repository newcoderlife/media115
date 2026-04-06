"""CLI tests."""

import json
import os
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


has_tmdb = bool(os.environ.get("TMDB_READ_ACCESS_TOKEN"))
has_bangumi = bool(os.environ.get("BANGUMI_ACCESS_TOKEN"))

# ── Sample tree text used across tests ──────────────────────────────────
SAMPLE_TREE = """\
影音
| |-电影
| | |-The Matrix (1999)
| | | |-The.Matrix.1999.mkv
| | | |-The Matrix (1999).nfo
| | |-Inception (2010)
| | | |-Inception.2010.1080p.mkv
| |-AV
| | |-DANDY-992
| | | |-DANDY-992.mp4
| | |-ABC-123
| | | |-ABC-123.mp4
| | | |-ABC-123.nfo
| |-剧目
| | |-Breaking Bad (2008)
| | | |-Breaking.Bad.S01E01.mkv
| | | |-Breaking.Bad.S01E02.mkv
"""


def _write_env(base="."):
    """Write a minimal .env so _load_env / _get_115_client can find credentials."""
    env_path = Path(base) / ".env"
    env_path.write_text("CLOUD_115_COOKIES=UID=test;CID=test\n")


def _write_tree(base=".", content=None):
    """Write tree_cache.txt to the XDG cache location."""
    from media115 import cache as media_cache
    tree_path = media_cache.tree_cache_path()
    tree_path.write_text(content or SAMPLE_TREE)
    return tree_path


class TestCLI:
    def test_help(self, runner):
        result = runner.invoke(main, ["--help"])
        assert result.exit_code == 0
        assert "media115" in result.output

    def test_auth_help(self, runner):
        result = runner.invoke(main, ["auth", "--help"])
        assert result.exit_code == 0
        assert "QR" in result.output or "login" in result.output

    def test_auth_check_not_logged_in(self, runner):
        with patch("media115.cli._get_115_client", return_value=None):
            result = runner.invoke(main, ["auth", "--check"])
            assert result.exit_code == 0
            assert "Not logged in" in result.output

    def test_auth_check_logged_in(self, runner):
        mock_client = MagicMock()
        mock_client.check_login.return_value = True
        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["auth", "--check"])
            assert result.exit_code == 0
            assert "Already logged in" in result.output

    @pytest.mark.live
    @pytest.mark.skipif(not has_tmdb, reason="TMDB_READ_ACCESS_TOKEN not set")
    def test_scrape_tmdb(self, runner):
        result = runner.invoke(main, ["scrape", "The Matrix", "--source", "tmdb"])
        assert result.exit_code == 0
        assert "Matrix" in result.output or "603" in result.output

    @pytest.mark.live
    @pytest.mark.skipif(not has_bangumi, reason="BANGUMI_ACCESS_TOKEN not set")
    def test_scrape_bangumi(self, runner):
        result = runner.invoke(main, ["scrape", "孤独摇滚", "--source", "bangumi"])
        assert result.exit_code == 0

    @pytest.mark.live
    @pytest.mark.skipif(not has_tmdb, reason="TMDB_READ_ACCESS_TOKEN not set")
    def test_scrape_default_source(self, runner):
        result = runner.invoke(main, ["scrape", "The Matrix"])
        assert result.exit_code == 0
        assert "Matrix" in result.output


class TestExportTree:
    def test_export_tree_writes_cache(self, runner):
        mock_client = MagicMock()
        mock_client.export_tree.return_value = SAMPLE_TREE
        mock_client.resolve_path.return_value = "12345"

        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.cli._get_115_client", return_value=mock_client):
                result = runner.invoke(main, ["export-tree", "/影音"])

            assert result.exit_code == 0
            assert "Tree exported" in result.output
            from media115 import cache as media_cache
            tree_path = media_cache.tree_cache_path()
            assert tree_path.exists()
            assert "The Matrix" in tree_path.read_text()
            # Should report video count
            assert "video files" in result.output

    def test_export_tree_no_client(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.cli._get_115_client", return_value=None):
                result = runner.invoke(main, ["export-tree", "/影音"])
            assert result.exit_code == 0  # Click still returns 0

    def test_export_tree_empty_result(self, runner):
        mock_client = MagicMock()
        mock_client.export_tree.return_value = ""
        mock_client.resolve_path.return_value = "12345"

        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.cli._get_115_client", return_value=mock_client):
                result = runner.invoke(main, ["export-tree", "/影音"])
            assert "Export failed" in result.output

    def test_export_tree_root_rejected(self, runner):
        mock_client = MagicMock()
        # path "0" => root, _resolve_dir returns "0"
        mock_client.resolve_path.return_value = "0"

        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.cli._get_115_client", return_value=mock_client):
                # "0" is a digit so _resolve_dir returns it directly
                result = runner.invoke(main, ["export-tree", "0"])
            assert "cannot export root" in result.output


class TestScanTree:
    def test_scan_tree_summary(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result = runner.invoke(main, ["scan-tree"])

        assert result.exit_code == 0
        assert "Summary:" in result.output
        assert "video files" in result.output.lower() or "files" in result.output

    def test_scan_tree_category_filter(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result = runner.invoke(main, ["scan-tree", "AV"])

        assert result.exit_code == 0
        assert "Summary:" in result.output

    def test_scan_tree_show_all(self, runner):
        """--all flag should show files that would be skipped (have NFO)."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result_default = runner.invoke(main, ["scan-tree"])
            result_all = runner.invoke(main, ["scan-tree", "--all"])

        assert result_all.exit_code == 0
        # --all output should be >= default output (it includes skipped files)
        assert len(result_all.output) >= len(result_default.output)

    def test_scan_tree_no_cache(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            result = runner.invoke(main, ["scan-tree"])
        assert "No tree cache" in result.output

    def test_scan_tree_anomaly_detection(self, runner):
        """Non-standard naming should be reported as anomaly."""
        tree_with_anomaly = """\
影音
| |-电影
| | |-weird folder name
| | | |-movie.mkv
| | | |-movie.nfo
"""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree(content=tree_with_anomaly)
            result = runner.invoke(main, ["scan-tree", "--all"])

        assert result.exit_code == 0
        # Should detect anomaly: folder "weird folder name" doesn't match "Title (Year)" pattern
        assert "Anomal" in result.output or "Non-standard" in result.output


class TestBatchScrape:
    def test_batch_scrape_progress_format(self, runner):
        """Verify batch-scrape shows [1/N X%] progress."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            mock_result = {
                "status": "ok",
                "match": "DANDY-992",
                "number": "DANDY-992",
            }

            with (
                patch("media115.scraper.scrape.scrape_av", return_value=mock_result),
                patch(
                    "media115.scraper.scrape.scrape_movie",
                    return_value={"status": "ok", "match": "Test Movie", "tmdb_id": 123},
                ),
                patch(
                    "media115.scraper.scrape.scrape_tv",
                    return_value={"status": "ok", "match": "Test TV", "tmdb_id": 456},
                ),
            ):
                result = runner.invoke(main, ["batch-scrape", "AV"])

        assert result.exit_code == 0
        assert "[1/" in result.output
        assert "%]" in result.output
        assert "Done:" in result.output

    def test_batch_scrape_no_videos(self, runner):
        """Category with no matching videos should say so."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result = runner.invoke(main, ["batch-scrape", "nonexistent_category"])

        assert result.exit_code == 0
        assert "No files to scrape" in result.output

    def test_batch_scrape_force_skips_nfo_check(self, runner):
        """--force should include files that already have NFO."""
        with runner.isolated_filesystem():
            _write_env()
            # ABC-123 has an NFO in the tree
            _write_tree()

            mock_result = {
                "status": "ok",
                "match": "ABC-123",
                "number": "ABC-123",
            }

            with patch("media115.scraper.scrape.scrape_av", return_value=mock_result):
                # Without --force: ABC-123 has NFO so only DANDY-992 processed
                result_normal = runner.invoke(main, ["batch-scrape", "AV"])
                # With --force: both files processed
                result_force = runner.invoke(main, ["batch-scrape", "AV", "--force"])

        # --force processes more files (2 vs 1)
        assert "Scraping 1 files" in result_normal.output
        assert "Scraping 2 files" in result_force.output

    def test_batch_scrape_limit(self, runner):
        """--limit should cap the number of files scraped."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            mock_result = {"status": "ok", "match": "Test", "number": "TEST-001"}

            with (
                patch("media115.scraper.scrape.scrape_av", return_value=mock_result),
                patch(
                    "media115.scraper.scrape.scrape_movie",
                    return_value={"status": "ok", "match": "M", "tmdb_id": 1},
                ),
                patch(
                    "media115.scraper.scrape.scrape_tv",
                    return_value={"status": "ok", "match": "T", "tmdb_id": 1},
                ),
            ):
                # Use "AV" category (paths are "AV/..." in parsed tree)
                # --force so NFO-existing files are included (2 AV files)
                # --limit 1 to cap at 1
                result = runner.invoke(main, ["batch-scrape", "AV", "--force", "--limit", "1"])

        assert result.exit_code == 0
        assert "Scraping 1 files" in result.output

    def test_batch_scrape_writes_log(self, runner):
        """Scrape should save a JSON log file."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            mock_result = {
                "status": "ok",
                "match": "DANDY-992",
                "number": "DANDY-992",
            }

            with patch("media115.scraper.scrape.scrape_av", return_value=mock_result):
                result = runner.invoke(main, ["batch-scrape", "AV"])

            assert result.exit_code == 0
            assert "Log saved to" in result.output
            # Check log dir exists
            from media115 import cache as media_cache
            log_dir = media_cache._cache_root() / "logs"
            assert log_dir.exists()
            log_files = list(log_dir.glob("scrape_AV_*.json"))
            assert len(log_files) == 1
            log_data = json.loads(log_files[0].read_text())
            assert isinstance(log_data, list)
            assert log_data[0]["status"] == "ok"

    def test_batch_scrape_error_handling(self, runner):
        """Scrape errors should be caught and reported."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with patch(
                "media115.scraper.scrape.scrape_av",
                side_effect=Exception("network timeout"),
            ):
                result = runner.invoke(main, ["batch-scrape", "AV"])

            assert result.exit_code == 0
            assert "error" in result.output.lower()


class TestOrganize:
    def test_organize_dry_run_shows_table(self, runner):
        """Organize dry-run should show table with source IDs."""
        plan = [
            {
                "file": "DANDY-992.mp4",
                "path": "影音/AV/some_dir/DANDY-992.mp4",
                "parent": "影音/AV/some_dir",
                "type": "av",
                "action": "rename",
                "new_folder": "DANDY-992",
                "new_name": "DANDY-992.mp4",
                "reason": "",
                "source_id": "DANDY-992",
            },
        ]

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with patch("media115.organizer.build_organize_plan", return_value=plan):
                result = runner.invoke(main, ["organize", "AV"])

        assert result.exit_code == 0
        assert "Organize plan" in result.output
        assert "1 to rename" in result.output
        assert "DANDY-992" in result.output
        assert "Dry-run complete" in result.output

    def test_organize_no_tree_cache(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            result = runner.invoke(main, ["organize", "AV"])
        assert "No tree cache" in result.output

    def test_organize_nothing_to_rename(self, runner):
        plan = [
            {
                "file": "ABC-123.mp4",
                "path": "影音/AV/ABC-123/ABC-123.mp4",
                "parent": "影音/AV/ABC-123",
                "type": "av",
                "action": "skip",
                "new_folder": None,
                "new_name": None,
                "reason": "already correct",
                "source_id": "",
            },
        ]

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with patch("media115.organizer.build_organize_plan", return_value=plan):
                result = runner.invoke(main, ["organize", "AV"])

        assert result.exit_code == 0
        assert "Nothing to rename" in result.output

    def test_organize_conflict_detection(self, runner):
        """Two files targeting same new name should be flagged as conflict."""
        plan = [
            {
                "file": "file1.mp4",
                "path": "影音/电影/d1/file1.mp4",
                "parent": "影音/电影/d1",
                "type": "movie",
                "action": "rename",
                "new_folder": "The Matrix (1999)",
                "new_name": "The Matrix (1999).mp4",
                "reason": "",
                "source_id": "603",
            },
            {
                "file": "file2.mp4",
                "path": "影音/电影/d2/file2.mp4",
                "parent": "影音/电影/d2",
                "type": "movie",
                "action": "rename",
                "new_folder": "The Matrix (1999)",
                "new_name": "The Matrix (1999).mp4",
                "reason": "",
                "source_id": "603",
            },
        ]

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with patch("media115.organizer.build_organize_plan", return_value=plan):
                result = runner.invoke(main, ["organize", "电影"])

        assert result.exit_code == 0
        assert "conflict" in result.output.lower()


class TestUpload:
    def test_upload_success(self, runner, tmp_path):
        """Upload command should call client.upload_file."""
        mock_client = MagicMock()
        mock_client.upload_file.return_value = {"status": "ok"}

        test_file = tmp_path / "test_video.mkv"
        test_file.write_bytes(b"\x00" * 1024)

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["upload", str(test_file)])

        assert result.exit_code == 0
        assert "Upload success" in result.output
        mock_client.upload_file.assert_called_once()
        call_args = mock_client.upload_file.call_args
        assert call_args[0][1] == "0"  # default remote_dir

    def test_upload_with_remote_dir(self, runner, tmp_path):
        mock_client = MagicMock()
        mock_client.upload_file.return_value = {"status": "ok"}

        test_file = tmp_path / "test.mkv"
        test_file.write_bytes(b"\x00" * 100)

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["upload", str(test_file), "--remote-dir", "99999"])

        assert result.exit_code == 0
        call_args = mock_client.upload_file.call_args
        assert call_args[0][1] == "99999"

    def test_upload_no_client(self, runner, tmp_path):
        test_file = tmp_path / "test.mkv"
        test_file.write_bytes(b"\x00" * 100)

        with patch("media115.cli._get_115_client", return_value=None):
            result = runner.invoke(main, ["upload", str(test_file)])
        assert result.exit_code == 0

    def test_upload_not_implemented(self, runner, tmp_path):
        mock_client = MagicMock()
        mock_client.upload_file.side_effect = NotImplementedError("no cookie mode")

        test_file = tmp_path / "test.mkv"
        test_file.write_bytes(b"\x00" * 100)

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["upload", str(test_file)])
        assert "cookie mode" in result.output.lower() or "auth" in result.output.lower()

    def test_upload_failure(self, runner, tmp_path):
        mock_client = MagicMock()
        mock_client.upload_file.return_value = None

        test_file = tmp_path / "test.mkv"
        test_file.write_bytes(b"\x00" * 100)

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["upload", str(test_file)])
        assert "failed" in result.output.lower()

    def test_upload_exception(self, runner, tmp_path):
        mock_client = MagicMock()
        mock_client.upload_file.side_effect = RuntimeError("disk full")

        test_file = tmp_path / "test.mkv"
        test_file.write_bytes(b"\x00" * 100)

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["upload", str(test_file)])
        assert "failed" in result.output.lower()


class TestLs:
    """Tests for the new PathResolver-based ls command."""

    def _mock_resolver(self, items=None):
        from unittest.mock import MagicMock
        r = MagicMock()
        r.resolve_dir.return_value = "0"
        r.listing.return_value = items or []
        return r

    def test_ls_lists_files(self, runner):
        resolver = self._mock_resolver([
            {"name": "movie.mkv", "fid": "f1", "size": 1048576, "pick_code": "pc"},
            {"name": "subdir", "cid": "c1"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "/影音"])
        assert result.exit_code == 0
        assert "movie.mkv" in result.output
        assert "subdir" in result.output

    def test_ls_default_root(self, runner):
        resolver = self._mock_resolver([])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls"])
        assert result.exit_code == 0
        resolver.resolve_dir.assert_called_with("/")

    def test_ls_no_client(self, runner):
        with patch("media115.fs_cli._get_resolver", side_effect=Exception("ClickException: 未登录")):
            result = runner.invoke(main, ["ls"])
        assert result.exit_code != 0

    def test_ls_error_handling(self, runner):
        resolver = self._mock_resolver()
        resolver.resolve_dir.side_effect = FileNotFoundError("path not found")
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "/no/such/path"])
        assert result.exit_code != 0

    def test_ls_path_resolution(self, runner):
        """Path should be passed to resolve_dir."""
        resolver = self._mock_resolver([
            {"name": "file.txt", "fid": "f1", "size": 42, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "/影音/电影"])
        assert result.exit_code == 0
        resolver.resolve_dir.assert_called_with("/影音/电影")


class TestStrm:
    def test_strm_generates_files(self, runner):
        """strm command should create .strm files with proxy URLs."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            # Parsed tree paths start at the first child level (e.g. "电影/...")
            # not "影音/电影/..." so use "电影" as the filter path
            result = runner.invoke(
                main,
                ["strm", "电影", "--output", "./strm_out", "--host", "myhost", "--port", "8080"],
            )

            assert result.exit_code == 0
            assert "Generated" in result.output
            assert ".strm" in result.output

            strm_dir = Path("strm_out")
            assert strm_dir.exists()
            strm_files = list(strm_dir.rglob("*.strm"))
            assert len(strm_files) > 0
            # Check content of first strm file
            content = strm_files[0].read_text().strip()
            assert content.startswith("http://myhost:8080/redirect/")

    def test_strm_no_tree_cache(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            result = runner.invoke(main, ["strm", "电影", "--output", "./strm_out"])
        assert "No tree cache" in result.output

    def test_strm_no_matching_videos(self, runner):
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result = runner.invoke(
                main,
                ["strm", "nonexistent/path", "--output", "./strm_out"],
            )
        assert "No video files found" in result.output

    def test_strm_default_config(self, runner):
        """Without --host/--port it should use config defaults."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result = runner.invoke(main, ["strm", "电影", "--output", "./strm_out"])

        assert result.exit_code == 0
        strm_files = list(Path("strm_out").rglob("*.strm"))
        if strm_files:
            content = strm_files[0].read_text().strip()
            # Default proxy is localhost:9000
            assert "localhost:9000" in content

    def test_strm_url_encoding(self, runner):
        """Chinese characters in paths should be URL-encoded."""
        with runner.isolated_filesystem():
            _write_env()
            _write_tree()
            result = runner.invoke(main, ["strm", "电影", "--output", "./strm_out"])

        assert result.exit_code == 0
        strm_files = list(Path("strm_out").rglob("*.strm"))
        if strm_files:
            content = strm_files[0].read_text().strip()
            # Chinese chars should be percent-encoded, not literal
            assert "影音" not in content or "%E5%BD%B1%E9%9F%B3" in content


class TestScrapeCommand:
    def test_scrape_tmdb_no_token(self, runner):
        """Should report error when TMDB token not set."""
        with runner.isolated_filesystem():
            # Write env without TMDB token
            Path(".env").write_text("DUMMY_KEY=1\n")
            with patch.dict(os.environ, {"TMDB_READ_ACCESS_TOKEN": ""}, clear=False):
                result = runner.invoke(main, ["scrape", "The Matrix", "--source", "tmdb"])
        assert "TMDB_READ_ACCESS_TOKEN" in result.output or result.exit_code == 0

    def test_scrape_javbus(self, runner):
        """javbus source should print stub message."""
        result = runner.invoke(main, ["scrape", "TEST-001", "--source", "javbus"])
        assert result.exit_code == 0
        assert "JavBus" in result.output


class TestAuthRenew:
    def test_auth_renew_no_existing(self, runner):
        """Renew without existing cookies should error."""
        with patch("media115.cli._get_115_client", return_value=None):
            result = runner.invoke(main, ["auth", "--renew"])
        assert "No existing cookies" in result.output

    def test_auth_renew_success(self, runner):
        mock_client = MagicMock()
        mock_client.renew_cookies.return_value = True

        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.cli._get_115_client", return_value=mock_client):
                result = runner.invoke(main, ["auth", "--renew"])
        assert "renewed" in result.output.lower()

    def test_auth_renew_failure(self, runner):
        mock_client = MagicMock()
        mock_client.renew_cookies.return_value = False

        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.cli._get_115_client", return_value=mock_client):
                result = runner.invoke(main, ["auth", "--renew"])
        assert "failed" in result.output.lower() or "re-login" in result.output.lower()

    def test_auth_already_logged_in_no_force(self, runner):
        mock_client = MagicMock()
        mock_client.check_login.return_value = True

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["auth"])
        assert "Already logged in" in result.output


class TestBatchScrapeMovieTV:
    """Cover the movie and TV branches of batch-scrape (lines 507-515)."""

    def test_batch_scrape_movie(self, runner):
        """batch-scrape should handle movie type files."""
        # Tree with only a movie file that has no NFO
        tree = """\
影音
| |-电影
| | |-Inception (2010)
| | | |-Inception.2010.1080p.mkv
"""
        mock_movie = {
            "status": "ok",
            "match": "Inception (2010)",
            "tmdb_id": 27205,
        }
        with runner.isolated_filesystem():
            _write_env()
            _write_tree(content=tree)

            with patch("media115.scraper.scrape.scrape_movie", return_value=mock_movie):
                result = runner.invoke(main, ["batch-scrape", "电影"])

        assert result.exit_code == 0
        assert "Scraping 1 files" in result.output
        assert "Done:" in result.output

    def test_batch_scrape_tv(self, runner):
        """batch-scrape should handle TV type files."""
        tree = """\
影音
| |-剧目
| | |-Breaking Bad (2008)
| | | |-Breaking.Bad.S01E01.mkv
"""
        mock_tv = {
            "status": "ok",
            "match": "Breaking Bad S01E01",
            "tmdb_id": 1396,
        }
        with runner.isolated_filesystem():
            _write_env()
            _write_tree(content=tree)

            with patch("media115.scraper.scrape.scrape_tv", return_value=mock_tv):
                result = runner.invoke(main, ["batch-scrape", "剧目"])

        assert result.exit_code == 0
        assert "Scraping 1 files" in result.output

    def test_batch_scrape_movie_not_found(self, runner):
        """batch-scrape with not_found status should report in summary."""
        tree = """\
影音
| |-电影
| | |-Unknown Movie
| | | |-Unknown.Movie.2020.mkv
"""
        mock_result = {"status": "not_found"}
        with runner.isolated_filesystem():
            _write_env()
            _write_tree(content=tree)

            with patch("media115.scraper.scrape.scrape_movie", return_value=mock_result):
                result = runner.invoke(main, ["batch-scrape", "电影"])

        assert result.exit_code == 0
        assert "1 failed" in result.output

    def test_batch_scrape_skip_type(self, runner):
        """batch-scrape with unrecognized type should produce skip."""
        tree = """\
影音
| |-other
| | |-randomdir
| | | |-random_file.mp4
"""
        # analyze_filename returns media_type='unknown' for random names
        # which hits the movie branch (analysis.media_type in ('movie', 'unknown'))
        mock_result = {"status": "ok", "match": "Random", "tmdb_id": 999}
        with runner.isolated_filesystem():
            _write_env()
            _write_tree(content=tree)

            with patch("media115.scraper.scrape.scrape_movie", return_value=mock_result):
                result = runner.invoke(main, ["batch-scrape", "other"])

        assert result.exit_code == 0
        assert "Done:" in result.output


class TestOrganizeExecute:
    """Cover organize --execute path (lines 618-682)."""

    def test_organize_execute(self, runner):
        plan = [
            {
                "file": "DANDY-992.mp4",
                "path": "AV/old_dir/DANDY-992.mp4",
                "parent": "AV/old_dir",
                "type": "av",
                "action": "rename",
                "new_folder": "DANDY-992",
                "new_name": "DANDY-992.mp4",
                "reason": "",
                "source_id": "DANDY-992",
            },
        ]

        mock_exec_results = [
            {**plan[0], "status": "ok"},
        ]

        mock_client = MagicMock()

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with (
                patch("media115.organizer.build_organize_plan", return_value=plan),
                patch(
                    "media115.organizer.execute_organize_plan",
                    return_value=mock_exec_results,
                ),
                patch("media115.cli._get_115_client", return_value=mock_client),
            ):
                result = runner.invoke(main, ["organize", "AV", "--execute"])

        assert result.exit_code == 0
        assert "Executing" in result.output
        assert "1 renamed" in result.output

    def test_organize_execute_no_client(self, runner):
        plan = [
            {
                "file": "test.mp4",
                "path": "AV/d/test.mp4",
                "parent": "AV/d",
                "type": "av",
                "action": "rename",
                "new_folder": "TEST",
                "new_name": "TEST.mp4",
                "reason": "",
                "source_id": "",
            },
        ]

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with (
                patch("media115.organizer.build_organize_plan", return_value=plan),
                patch("media115.cli._get_115_client", return_value=None),
            ):
                result = runner.invoke(main, ["organize", "AV", "--execute"])

        assert result.exit_code == 0

    def test_organize_execute_with_errors(self, runner):
        plan = [
            {
                "file": "f1.mp4",
                "path": "AV/d1/f1.mp4",
                "parent": "AV/d1",
                "type": "av",
                "action": "rename",
                "new_folder": "NUM-001",
                "new_name": "NUM-001.mp4",
                "reason": "",
                "source_id": "",
            },
            {
                "file": "f2.mp4",
                "path": "AV/d2/f2.mp4",
                "parent": "AV/d2",
                "type": "av",
                "action": "rename",
                "new_folder": "NUM-002",
                "new_name": "NUM-002.mp4",
                "reason": "",
                "source_id": "",
            },
        ]

        mock_exec_results = [
            {**plan[0], "status": "ok"},
            {**plan[1], "status": "error", "error": "dir not found"},
        ]

        mock_client = MagicMock()

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with (
                patch("media115.organizer.build_organize_plan", return_value=plan),
                patch(
                    "media115.organizer.execute_organize_plan",
                    return_value=mock_exec_results,
                ),
                patch("media115.cli._get_115_client", return_value=mock_client),
            ):
                result = runner.invoke(main, ["organize", "AV", "--execute"])

        assert result.exit_code == 0
        assert "1 renamed" in result.output
        assert "1 failed" in result.output
        assert "Error:" in result.output

    def test_organize_execute_not_found(self, runner):
        plan = [
            {
                "file": "f1.mp4",
                "path": "AV/d1/f1.mp4",
                "parent": "AV/d1",
                "type": "av",
                "action": "rename",
                "new_folder": "NUM-001",
                "new_name": "NUM-001.mp4",
                "reason": "",
                "source_id": "",
            },
        ]

        mock_exec_results = [
            {**plan[0], "status": "not_found"},
        ]

        mock_client = MagicMock()

        with runner.isolated_filesystem():
            _write_env()
            _write_tree()

            with (
                patch("media115.organizer.build_organize_plan", return_value=plan),
                patch(
                    "media115.organizer.execute_organize_plan",
                    return_value=mock_exec_results,
                ),
                patch("media115.cli._get_115_client", return_value=mock_client),
            ):
                result = runner.invoke(main, ["organize", "AV", "--execute"])

        assert result.exit_code == 0
        assert "Not found on 115" in result.output


class TestScrapeCommandMocked:
    """Cover scrape command branches with mocked API clients."""

    def test_scrape_tmdb_with_results(self, runner):
        """TMDB scrape should display results."""
        mock_tmdb = MagicMock()
        mock_tmdb.search_movie.return_value = [
            {"id": 603, "title": "The Matrix", "release_date": "1999-03-31"},
        ]

        with runner.isolated_filesystem():
            _write_env()
            with patch.dict(os.environ, {"TMDB_READ_ACCESS_TOKEN": "fake_token"}):
                with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_tmdb):
                    result = runner.invoke(main, ["scrape", "The Matrix", "--source", "tmdb"])

        assert result.exit_code == 0
        assert "603" in result.output
        assert "Matrix" in result.output

    def test_scrape_tmdb_no_results(self, runner):
        """TMDB scrape with no results should report it."""
        mock_tmdb = MagicMock()
        mock_tmdb.search_movie.return_value = []
        mock_tmdb.search_tv.return_value = []

        with runner.isolated_filesystem():
            _write_env()
            with patch.dict(os.environ, {"TMDB_READ_ACCESS_TOKEN": "fake_token"}):
                with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_tmdb):
                    result = runner.invoke(main, ["scrape", "xyznonexistent", "--source", "tmdb"])

        assert result.exit_code == 0
        assert "No results" in result.output

    def test_scrape_tmdb_tv_fallback(self, runner):
        """When movie search returns nothing, should fall back to TV search."""
        mock_tmdb = MagicMock()
        mock_tmdb.search_movie.return_value = []
        mock_tmdb.search_tv.return_value = [
            {"id": 1396, "name": "Breaking Bad", "first_air_date": "2008-01-20"},
        ]

        with runner.isolated_filesystem():
            _write_env()
            with patch.dict(os.environ, {"TMDB_READ_ACCESS_TOKEN": "fake_token"}):
                with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_tmdb):
                    result = runner.invoke(main, ["scrape", "Breaking Bad", "--source", "tmdb"])

        assert result.exit_code == 0
        assert "1396" in result.output
        assert "Breaking Bad" in result.output

    def test_scrape_bangumi_with_results(self, runner):
        """Bangumi scrape should display results."""
        mock_bangumi = MagicMock()
        mock_bangumi.search.return_value = [
            {"id": 328609, "name": "Bocchi the Rock!", "name_cn": "孤独摇滚"},
        ]

        with runner.isolated_filesystem():
            _write_env()
            with patch.dict(os.environ, {"BANGUMI_ACCESS_TOKEN": "fake_token"}):
                with patch(
                    "media115.scraper.bangumi.BangumiClient",
                    return_value=mock_bangumi,
                ):
                    result = runner.invoke(main, ["scrape", "孤独摇滚", "--source", "bangumi"])

        assert result.exit_code == 0
        assert "328609" in result.output

    def test_scrape_bangumi_no_results(self, runner):
        mock_bangumi = MagicMock()
        mock_bangumi.search.return_value = []

        with runner.isolated_filesystem():
            _write_env()
            with patch.dict(os.environ, {"BANGUMI_ACCESS_TOKEN": "fake_token"}):
                with patch(
                    "media115.scraper.bangumi.BangumiClient",
                    return_value=mock_bangumi,
                ):
                    result = runner.invoke(
                        main, ["scrape", "xyznonexistent", "--source", "bangumi"]
                    )

        assert result.exit_code == 0
        assert "No results" in result.output


class TestLsTree:
    """Cover ls -R recursive path."""

    def test_ls_recursive(self, runner):
        root_items = [
            {"name": "电影", "cid": "100"},
            {"name": "readme.txt", "fid": "f1", "size": 100, "pick_code": "pc"},
        ]
        child_items = [
            {"name": "The Matrix.mkv", "fid": "f2", "size": 5000000, "pick_code": "pc2"},
        ]
        resolver = MagicMock()
        resolver.resolve_dir.return_value = "0"
        resolver.listing.side_effect = lambda cid: (
            child_items if cid == "100" else root_items
        )

        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "-R", "--depth", "1", "/"])

        assert result.exit_code == 0
        assert "电影" in result.output
        assert "readme.txt" in result.output


class TestConfigLoading:
    """Cover _load_config with user config.yaml (lines 68-76)."""

    def test_load_config_with_yaml(self, runner):
        """config.yaml should override defaults."""
        from media115.cli import _load_config

        with runner.isolated_filesystem():
            Path("config.yaml").write_text('root: "/custom"\njellyfin_url: "http://myhost:8096"\n')
            config = _load_config()

        assert config["root"] == "/custom"
        assert config["jellyfin_url"] == "http://myhost:8096"
        # Defaults should still be present
        assert "categories" in config

    def test_load_config_without_yaml(self, runner):
        from media115.cli import _load_config

        with runner.isolated_filesystem():
            config = _load_config()

        assert config["root"] == "/影音"

    def test_load_config_merge_dicts(self, runner):
        """Dict values in config.yaml should be merged, not replaced."""
        from media115.cli import _load_config

        with runner.isolated_filesystem():
            Path("config.yaml").write_text("strm_proxy:\n  host: myhost\n")
            config = _load_config()

        # "host" overridden but "port" preserved from defaults
        assert config["strm_proxy"]["host"] == "myhost"
        assert config["strm_proxy"]["port"] == 9000


class TestLoadEnv:
    """Cover _load_env (line 17)."""

    def test_load_env_sets_vars(self, runner):
        with runner.isolated_filesystem():
            Path(".env").write_text("TEST_VAR_1=hello\n# comment line\n\nTEST_VAR_2='world'\n")
            # Remove the vars if set
            os.environ.pop("TEST_VAR_1", None)
            os.environ.pop("TEST_VAR_2", None)

            from media115.cli import _load_env

            _load_env()

            assert os.environ.get("TEST_VAR_1") == "hello"
            assert os.environ.get("TEST_VAR_2") == "world"

            # Cleanup
            os.environ.pop("TEST_VAR_1", None)
            os.environ.pop("TEST_VAR_2", None)


class TestGetClient:
    def test_get_client_from_cookies(self, runner):
        """_get_115_client should use CLOUD_115_COOKIES env var."""
        with runner.isolated_filesystem():
            _write_env()
            with patch("media115.client.Cloud115Client.from_cookies") as mock_from:
                mock_from.return_value = MagicMock()
                with patch.dict(os.environ, {"CLOUD_115_COOKIES": "test_cookie"}):
                    from media115.cli import _get_115_client

                    client = _get_115_client()
                assert client is not None
                mock_from.assert_called_once_with("test_cookie")

    def test_get_client_no_credentials(self, runner):
        with patch.dict(
            os.environ,
            {"CLOUD_115_COOKIES": ""},
            clear=False,
        ):
            from media115.cli import _get_115_client

            client = _get_115_client()
        assert client is None

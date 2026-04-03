"""Scan command and path resolution tests (all mocked)."""

import pytest
from unittest.mock import patch, MagicMock
from click.testing import CliRunner

from media115.client import Cloud115Client
from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


class TestResolvePath:
    def test_root(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        with patch.object(client, "list_files_all") as mock_ls:
            result = client.resolve_path("/")
            assert result == "0"
            mock_ls.assert_not_called()

    def test_single_level(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        with patch.object(client, "list_files_all") as mock_ls:
            mock_ls.return_value = [
                {"fn": "other", "cid": "10"},
                {"fn": "影音", "cid": "20"},
            ]
            result = client.resolve_path("/影音")
            assert result == "20"

    def test_nested_path(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        with patch.object(client, "list_files_all") as mock_ls:
            # First call: root listing
            # Second call: /影音 listing
            mock_ls.side_effect = [
                [{"fn": "影音", "cid": "20"}],
                [{"fn": "电影", "cid": "30"}, {"fn": "动漫", "cid": "31"}],
            ]
            result = client.resolve_path("/影音/电影")
            assert result == "30"

    def test_not_found(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        with patch.object(client, "list_files_all") as mock_ls:
            mock_ls.return_value = [{"fn": "other", "cid": "10"}]
            with pytest.raises(FileNotFoundError, match="not_exist"):
                client.resolve_path("/not_exist")


class TestListFilesRecursive:
    def test_flat_directory(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        with patch.object(client, "list_files_all") as mock_ls:
            mock_ls.return_value = [
                {"fn": "movie.mkv", "fid": "1", "sha": "abc", "pc": "p1"},
                {"fn": "sub.srt", "fid": "2", "sha": "def", "pc": "p2"},
            ]
            result = client.list_files_recursive("100")
            assert len(result) == 2
            assert all(not item["_is_dir"] for item in result)

    def test_with_subdirectory(self):
        client = Cloud115Client.from_cookies("UID=1_A1_0; CID=x; SEID=y")
        with patch.object(client, "list_files_all") as mock_ls:
            mock_ls.side_effect = [
                # Root listing
                [
                    {"fn": "subdir", "cid": "200"},
                    {"fn": "root.mkv", "fid": "1", "sha": "abc", "pc": "p1"},
                ],
                # Subdir listing
                [
                    {"fn": "child.mkv", "fid": "2", "sha": "def", "pc": "p2"},
                ],
            ]
            result = client.list_files_recursive("100")
            assert len(result) == 3
            names = [item.get("fn") for item in result]
            assert "subdir" in names
            assert "root.mkv" in names
            assert "child.mkv" in names


class TestScanCommand:
    def test_scan_dry_run(self, runner):
        mock_client = MagicMock()
        mock_client.resolve_path.return_value = "100"
        mock_client.list_files_recursive.return_value = [
            {
                "fn": "The.Matrix.1999.BluRay.mkv",
                "fid": "1",
                "sha": "abc",
                "pc": "p1",
                "_is_dir": False,
                "_parent_id": "100",
            },
            {
                "fn": "ABC-123.mp4",
                "fid": "2",
                "sha": "def",
                "pc": "p2",
                "_is_dir": False,
                "_parent_id": "100",
            },
            {
                "fn": "The.Matrix.1999.BluRay.nfo",
                "fid": "3",
                "sha": "ghi",
                "pc": "p3",
                "_is_dir": False,
                "_parent_id": "100",
            },
        ]

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["scan", "/影音"])
            assert result.exit_code == 0
            assert "2 video files" in result.output
            assert "Matrix" in result.output
            assert "ABC-123" in result.output
            assert "Dry-run" in result.output

    def test_scan_detects_nfo(self, runner):
        mock_client = MagicMock()
        mock_client.resolve_path.return_value = "100"
        mock_client.list_files_recursive.return_value = [
            {
                "fn": "movie.mkv",
                "fid": "1",
                "sha": "abc",
                "pc": "p1",
                "_is_dir": False,
                "_parent_id": "100",
            },
            {
                "fn": "movie.nfo",
                "fid": "2",
                "sha": "def",
                "pc": "p2",
                "_is_dir": False,
                "_parent_id": "100",
            },
        ]

        with patch("media115.cli._get_115_client", return_value=mock_client):
            result = runner.invoke(main, ["scan", "100"])
            assert result.exit_code == 0
            assert "skip" in result.output

    def test_scan_no_credentials(self, runner):
        with patch("media115.cli._get_115_client", return_value=None):
            result = runner.invoke(main, ["scan", "/影音"])
            # Should exit gracefully
            assert result.exit_code == 0

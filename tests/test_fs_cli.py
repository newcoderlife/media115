"""Tests for fs CLI commands: ls, stat, find."""
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


def _mock_resolver(items=None):
    r = MagicMock()
    r.resolve_dir.return_value = "100"
    r.listing.return_value = items or []
    return r


class TestLs:
    def test_ls_shows_files_and_dirs(self, runner):
        resolver = _mock_resolver([
            {"name": "子目录", "cid": "200"},
            {"name": "test.mkv", "fid": "300", "size": 1048576, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "/影音"])
        assert result.exit_code == 0
        assert "子目录" in result.output
        assert "test.mkv" in result.output

    def test_ls_dir_has_trailing_slash(self, runner):
        resolver = _mock_resolver([
            {"name": "子目录", "cid": "200"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "/影音"])
        assert result.exit_code == 0
        assert "子目录/" in result.output

    def test_ls_long_format(self, runner):
        resolver = _mock_resolver([
            {"name": "test.mkv", "fid": "300", "size": 1048576, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "-l", "/影音"])
        assert result.exit_code == 0
        assert "1.0M" in result.output

    def test_ls_long_format_shows_type(self, runner):
        resolver = _mock_resolver([
            {"name": "subdir", "cid": "200"},
            {"name": "test.mkv", "fid": "300", "size": 1048576, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "-l", "/影音"])
        assert result.exit_code == 0
        # long format shows type indicators
        assert "d" in result.output or "f" in result.output or "DIR" in result.output or "FILE" in result.output

    def test_ls_root_default(self, runner):
        resolver = _mock_resolver([])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls"])
        resolver.resolve_dir.assert_called_with("/")

    def test_ls_recursive(self, runner):
        root_items = [{"name": "子目录", "cid": "200"}]
        child_items = [
            {"name": "movie.mkv", "fid": "999", "size": 1024, "pick_code": "xyz"},
        ]
        resolver = _mock_resolver(root_items)
        # Root cid="100" → root_items; child cid="200" → child_items
        resolver.listing.side_effect = lambda cid: (
            child_items if cid == "200" else root_items
        )
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "-R", "/影音"])
        assert result.exit_code == 0
        assert "子目录" in result.output

    def test_ls_format_sizes(self, runner):
        resolver = _mock_resolver([
            {"name": "small.txt", "fid": "1", "size": 512, "pick_code": "a"},
            {"name": "big.mkv", "fid": "2", "size": 4 * 1024 * 1024 * 1024, "pick_code": "b"},
        ])
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["ls", "-l", "/"])
        assert result.exit_code == 0
        assert "512B" in result.output
        assert "4.0G" in result.output


class TestStat:
    def test_stat_file(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = (
            "fid1", "100",
            {"name": "a.mkv", "fid": "fid1", "size": 1000, "pick_code": "pc1"},
        )
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["stat", "/影音/a.mkv"])
        assert result.exit_code == 0
        assert "a.mkv" in result.output

    def test_stat_file_outputs_json(self, runner):
        import json
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = (
            "fid1", "100",
            {"name": "a.mkv", "fid": "fid1", "size": 1000, "pick_code": "pc1"},
        )
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["stat", "/影音/a.mkv"])
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["name"] == "a.mkv"
        assert data["fid"] == "fid1"

    def test_stat_dir(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = FileNotFoundError
        resolver.resolve_dir.return_value = "100"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["stat", "/影音"])
        assert result.exit_code == 0
        assert "directory" in result.output

    def test_stat_dir_outputs_json(self, runner):
        import json
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = FileNotFoundError
        resolver.resolve_dir.return_value = "100"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["stat", "/影音"])
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["type"] == "directory"
        assert data["cid"] == "100"

    def test_stat_not_found(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = FileNotFoundError
        resolver.resolve_dir.side_effect = FileNotFoundError
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["stat", "/no/such/path"])
        assert result.exit_code != 0


class TestFind:
    def test_find(self, runner):
        resolver = _mock_resolver()
        resolver.client.search.return_value = [
            {"fn": "满江红.mkv", "fid": "1"},
            {"fn": "满江红.nfo", "fid": "2"},
        ]
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["find", "满江红", "/影音"])
        assert result.exit_code == 0
        assert "满江红.mkv" in result.output
        assert "满江红.nfo" in result.output

    def test_find_default_root(self, runner):
        resolver = _mock_resolver()
        resolver.client.search.return_value = []
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["find", "keyword"])
        assert result.exit_code == 0
        resolver.resolve_dir.assert_called_with("/")

    def test_find_passes_cid_to_search(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_dir.return_value = "999"
        resolver.client.search.return_value = []
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["find", "test", "/some/dir"])
        resolver.client.search.assert_called_with("test", dir_id="999")

    def test_find_no_results(self, runner):
        resolver = _mock_resolver()
        resolver.client.search.return_value = []
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["find", "nonexistent"])
        assert result.exit_code == 0

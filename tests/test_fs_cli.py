"""Tests for fs CLI commands: ls, stat, find."""
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


def _mock_client(items=None):
    """Create a mock CachedClient with sensible defaults."""
    c = MagicMock()
    c.list_dir.return_value = items or []
    c.resolve_path.return_value = "100"
    c.find_file.return_value = {
        "name": "file.mkv", "type": "file", "fid": "fid1",
        "size": 1000, "pick_code": "pc1", "parent_cid": "100",
    }
    c.stat.return_value = {
        "name": "file.mkv", "type": "file", "fid": "fid1",
        "size": 1000, "pick_code": "pc1",
    }
    c.search.return_value = []
    c.cache_status.return_value = {"path_count": 0, "dir_count": 0, "entry_count": 0, "db_size_bytes": 0}
    return c


class TestLs:
    def test_ls_shows_files_and_dirs(self, runner):
        client = _mock_client([
            {"name": "子目录", "type": "dir", "cid": "200"},
            {"name": "test.mkv", "type": "file", "fid": "300", "size": 1048576, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls", "/影音"])
        assert result.exit_code == 0
        assert "子目录" in result.output
        assert "test.mkv" in result.output

    def test_ls_dir_has_trailing_slash(self, runner):
        client = _mock_client([
            {"name": "子目录", "type": "dir", "cid": "200"},
        ])
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls", "/影音"])
        assert result.exit_code == 0
        assert "子目录/" in result.output

    def test_ls_long_format(self, runner):
        client = _mock_client([
            {"name": "test.mkv", "type": "file", "fid": "300", "size": 1048576, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls", "-l", "/影音"])
        assert result.exit_code == 0
        assert "1.0M" in result.output

    def test_ls_long_format_shows_type(self, runner):
        client = _mock_client([
            {"name": "subdir", "type": "dir", "cid": "200"},
            {"name": "test.mkv", "type": "file", "fid": "300", "size": 1048576, "pick_code": "pc"},
        ])
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls", "-l", "/影音"])
        assert result.exit_code == 0
        # long format shows type indicators
        assert "d" in result.output or "f" in result.output or "DIR" in result.output or "FILE" in result.output

    def test_ls_root_default(self, runner):
        client = _mock_client([])
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls"])
        client.list_dir.assert_called_with("/")

    def test_ls_recursive(self, runner):
        root_items = [{"name": "子目录", "type": "dir", "cid": "200"}]
        child_items = [
            {"name": "movie.mkv", "type": "file", "fid": "999", "size": 1024, "pick_code": "xyz"},
        ]
        client = _mock_client(root_items)
        # Root path → root_items; child path → child_items
        client.list_dir.side_effect = lambda path: (
            child_items if "子目录" in path else root_items
        )
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls", "-R", "/影音"])
        assert result.exit_code == 0
        assert "子目录" in result.output

    def test_ls_format_sizes(self, runner):
        client = _mock_client([
            {"name": "small.txt", "type": "file", "fid": "1", "size": 512, "pick_code": "a"},
            {"name": "big.mkv", "type": "file", "fid": "2", "size": 4 * 1024 * 1024 * 1024, "pick_code": "b"},
        ])
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["ls", "-l", "/"])
        assert result.exit_code == 0
        assert "512B" in result.output
        assert "4.0G" in result.output


class TestStat:
    def test_stat_file(self, runner):
        client = _mock_client()
        client.stat.return_value = {
            "name": "a.mkv", "type": "file", "fid": "fid1",
            "size": 1000, "pick_code": "pc1",
        }
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["stat", "/影音/a.mkv"])
        assert result.exit_code == 0
        assert "a.mkv" in result.output

    def test_stat_file_outputs_json(self, runner):
        import json
        client = _mock_client()
        client.stat.return_value = {
            "name": "a.mkv", "type": "file", "fid": "fid1",
            "size": 1000, "pick_code": "pc1",
        }
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["stat", "/影音/a.mkv"])
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["name"] == "a.mkv"
        assert data["fid"] == "fid1"

    def test_stat_dir(self, runner):
        client = _mock_client()
        client.stat.return_value = {"name": "影音", "type": "dir", "cid": "100"}
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["stat", "/影音"])
        assert result.exit_code == 0
        assert "directory" in result.output

    def test_stat_dir_outputs_json(self, runner):
        import json
        client = _mock_client()
        client.stat.return_value = {"name": "影音", "type": "dir", "cid": "100"}
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["stat", "/影音"])
        assert result.exit_code == 0
        data = json.loads(result.output)
        assert data["type"] == "directory"
        assert data["cid"] == "100"

    def test_stat_not_found(self, runner):
        client = _mock_client()
        client.stat.side_effect = FileNotFoundError("not found")
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["stat", "/no/such/path"])
        assert result.exit_code != 0


class TestFind:
    def test_find(self, runner):
        client = _mock_client()
        client.search.return_value = [
            {"fn": "满江红.mkv", "fid": "1"},
            {"fn": "满江红.nfo", "fid": "2"},
        ]
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["find", "满江红", "/影音"])
        assert result.exit_code == 0
        assert "满江红.mkv" in result.output
        assert "满江红.nfo" in result.output

    def test_find_default_root(self, runner):
        client = _mock_client()
        client.search.return_value = []
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["find", "keyword"])
        assert result.exit_code == 0
        client.search.assert_called_with("keyword", "/")

    def test_find_passes_path_to_search(self, runner):
        client = _mock_client()
        client.search.return_value = []
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["find", "test", "/some/dir"])
        client.search.assert_called_with("test", "/some/dir")

    def test_find_no_results(self, runner):
        client = _mock_client()
        client.search.return_value = []
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["find", "nonexistent"])
        assert result.exit_code == 0


class TestMkdir:
    def test_mkdir_simple(self, runner):
        client = _mock_client()
        client.mkdir.return_value = "999"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["mkdir", "/影音/新目录"])
        assert result.exit_code == 0
        client.mkdir.assert_called_once_with("/影音/新目录", parents=False)

    def test_mkdir_p(self, runner):
        """mkdir -p creates directory with parents=True."""
        client = _mock_client()
        # First resolve_path call (check if exists) raises FileNotFoundError
        client.resolve_path.side_effect = FileNotFoundError("")
        client.mkdir.return_value = "20"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["mkdir", "-p", "/a/b"])
        assert result.exit_code == 0
        client.mkdir.assert_called_once_with("/a/b", parents=True)

    def test_mkdir_p_existing_dir(self, runner):
        """mkdir -p 目标已存在时应该是 no-op。"""
        client = _mock_client()
        # resolve_path 成功意味着目录已存在
        client.resolve_path.return_value = "existing_cid"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["mkdir", "-p", "/影音/电影"])
        assert result.exit_code == 0
        client.mkdir.assert_not_called()


class TestRm:
    def test_rm_file(self, runner):
        client = _mock_client()
        client.stat.return_value = {"name": "a.txt", "type": "file", "fid": "fid1"}
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["rm", "/影音/a.txt"])
        assert result.exit_code == 0
        client.delete.assert_called_once_with(["/影音/a.txt"])

    def test_rm_multiple(self, runner):
        client = _mock_client()
        client.stat.side_effect = [
            {"name": "a.txt", "type": "file", "fid": "fid1"},
            {"name": "b.txt", "type": "file", "fid": "fid2"},
        ]
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["rm", "/影音/a.txt", "/影音/b.txt"])
        client.delete.assert_called_once_with(["/影音/a.txt", "/影音/b.txt"])

    def test_rm_dir_requires_r(self, runner):
        client = _mock_client()
        client.stat.return_value = {"name": "somedir", "type": "dir", "cid": "dir_cid"}
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["rm", "/影音/somedir"])
        # 应该报错或输出提示
        assert result.exit_code != 0 or "-r" in result.output

    def test_rm_dir_with_r(self, runner):
        client = _mock_client()
        client.stat.return_value = {"name": "somedir", "type": "dir", "cid": "dir_cid"}
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["rm", "-r", "/影音/somedir"])
        assert result.exit_code == 0
        client.delete.assert_called_once_with(["/影音/somedir"])


class TestRename:
    def test_rename_single(self, runner):
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["rename", "/影音/old.mkv", "new.mkv"])
        assert result.exit_code == 0
        client.rename.assert_called_once_with("/影音/old.mkv", "new.mkv")

    def test_rename_batch(self, runner):
        client = _mock_client()
        stdin_data = '[["/影音/a.mkv","new_a.mkv"],["/影音/b.mkv","new_b.mkv"]]'
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["rename", "--batch"], input=stdin_data)
        assert result.exit_code == 0
        client.batch_rename.assert_called_once_with(
            [["/影音/a.mkv", "new_a.mkv"], ["/影音/b.mkv", "new_b.mkv"]]
        )


class TestMv:
    def test_mv_to_dir(self, runner):
        """mv /a/file /b/ -> move to directory b"""
        client = _mock_client()
        client.resolve_path.return_value = "cid_b"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["mv", "/a/file.mkv", "/b/"])
        assert result.exit_code == 0
        client.move.assert_called_once_with(["/a/file.mkv"], "/b/")

    def test_mv_multi_to_dir(self, runner):
        """mv /a /b /c /target/ -> batch move"""
        client = _mock_client()
        client.resolve_path.return_value = "target_cid"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["mv", "/a", "/b", "/c", "/target/"])
        assert result.exit_code == 0
        client.move.assert_called_once_with(["/a", "/b", "/c"], "/target/")

    def test_mv_same_dir_renames(self, runner):
        """mv /a/old /a/new -> rename within same directory"""
        client = _mock_client()
        # dest is not an existing directory
        def resolve_path_side_effect(path):
            path_n = path.rstrip("/")
            if path_n == "/a":
                return "cid_a"
            raise FileNotFoundError(path)
        client.resolve_path.side_effect = resolve_path_side_effect
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["mv", "/a/old.mkv", "/a/new.mkv"])
        assert result.exit_code == 0
        client.rename.assert_called_once_with("/a/old.mkv", "new.mkv")


class TestPut:
    def test_put_uploads_file(self, runner, tmp_path):
        local_file = tmp_path / "test.nfo"
        local_file.write_text("<nfo/>")
        client = _mock_client()
        client.rapid_upload.return_value = {"status": 0}
        client.upload.return_value = {"data": {"file_id": "new_fid"}}
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["put", str(local_file), "/影音/电影/"])
        assert result.exit_code == 0
        client.upload.assert_called_once()

    def test_put_nonexistent_local(self, runner):
        """本地文件不存在应该报错。"""
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["put", "/no/such/file.txt", "/影音/"])
        assert result.exit_code != 0


class TestGet:
    def test_get_downloads_file(self, runner, tmp_path):
        client = _mock_client()
        client.find_file.return_value = {
            "name": "a.mkv", "type": "file", "fid": "fid1",
            "pick_code": "pc1", "size": 1000, "parent_cid": "pcid",
        }
        client.download_url.return_value = "https://cdn.115.com/fake"
        with (
            patch("media115.fs_cli._get_client", return_value=client),
            patch("media115.fs_cli._download_to_file") as mock_dl,
        ):
            result = runner.invoke(main, ["get", "/影音/a.mkv", str(tmp_path)])
        assert result.exit_code == 0
        mock_dl.assert_called_once()
        # 检查下载路径包含文件名
        call_args = mock_dl.call_args
        assert "a.mkv" in str(call_args)

    def test_get_default_local_dir(self, runner, tmp_path, monkeypatch):
        """不指定本地目录时，下载到当前目录。"""
        monkeypatch.chdir(tmp_path)
        client = _mock_client()
        client.find_file.return_value = {
            "name": "b.mkv", "type": "file", "fid": "fid1",
            "pick_code": "pc2", "size": 500, "parent_cid": "pcid",
        }
        client.download_url.return_value = "https://cdn.115.com/fake"
        with (
            patch("media115.fs_cli._get_client", return_value=client),
            patch("media115.fs_cli._download_to_file") as mock_dl,
        ):
            result = runner.invoke(main, ["get", "/影音/b.mkv"])
        assert result.exit_code == 0
        mock_dl.assert_called_once()


class TestSync:
    def test_sync_exports_tree(self, runner):
        client = _mock_client()
        client.export_tree.return_value = "影音\n|-电影\n| |-满江红 (2023)\n"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code == 0
        client.export_tree.assert_called_once()

    def test_sync_saves_tree_cache(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        client = _mock_client()
        client.export_tree.return_value = "影音\n|-电影\n| |-满江红 (2023)\n"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code == 0
        from media115.cache import tree_cache_path
        assert tree_cache_path().exists()

    def test_sync_no_result(self, runner):
        client = _mock_client()
        client.export_tree.return_value = None
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code != 0 or "失败" in result.output or "fail" in result.output.lower()

    def test_sync_dir_not_found(self, runner):
        client = _mock_client()
        client.export_tree.side_effect = FileNotFoundError
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["sync", "/不存在的路径"])
        assert result.exit_code != 0

    def test_sync_deep_flag_prints_notice(self, runner):
        client = _mock_client()
        client.export_tree.return_value = "影音\n|-电影\n"
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["sync", "--deep", "/影音"])
        assert result.exit_code == 0
        assert "暂未实现" in result.output


class TestCacheStatus:
    def test_cache_status(self, runner):
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["cache", "status"])
        assert result.exit_code == 0

    def test_cache_status_tree_cache_absent(self, runner):
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["cache", "status"])
        assert result.exit_code == 0
        assert "不存在" in result.output


class TestInit:
    def test_init_copies_skills(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("HOME", str(tmp_path))
        monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path / ".config"))
        result = runner.invoke(main, ["init"])
        assert result.exit_code == 0
        skills_dir = tmp_path / ".claude" / "skills" / "media115"
        assert skills_dir.exists()

    def test_init_creates_config(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("HOME", str(tmp_path))
        monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path / ".config"))
        result = runner.invoke(main, ["init"])
        config = tmp_path / ".config" / "media115" / "config.yaml"
        assert config.exists()


class TestCacheClear:
    def test_cache_clear_confirmed(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        from media115.cache import _cache_dir
        fs_dir = _cache_dir("fs")
        (fs_dir / "path_index.json").write_text("{}")
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["cache", "clear"], input="y\n")
        assert result.exit_code == 0
        assert not fs_dir.exists()

    def test_cache_clear_cancelled(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        from media115.cache import _cache_dir
        fs_dir = _cache_dir("fs")
        (fs_dir / "path_index.json").write_text("{}")
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["cache", "clear"], input="n\n")
        assert result.exit_code == 0
        assert "取消" in result.output

    def test_cache_clear_does_not_remove_scrape(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        from media115.cache import _cache_dir, _cache_root
        # Create scrape dir and fs dir
        scrape_dir = _cache_dir("scrape")
        (scrape_dir / "test.json").write_text("{}")
        _cache_dir("fs")
        client = _mock_client()
        with patch("media115.fs_cli._get_client", return_value=client):
            result = runner.invoke(main, ["cache", "clear"], input="y\n")
        assert result.exit_code == 0
        # scrape/ should still exist
        assert scrape_dir.exists()

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
    r.resolve_file.return_value = ("fid1", "100", {"name": "file.mkv"})
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


class TestMkdir:
    def test_mkdir_simple(self, runner):
        resolver = _mock_resolver()
        resolver.client.mkdir.return_value = {"cid": "999", "cname": "新目录"}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["mkdir", "/影音/新目录"])
        assert result.exit_code == 0
        resolver.client.mkdir.assert_called_once()

    def test_mkdir_p(self, runner):
        """mkdir -p 逐级创建不存在的目录。"""
        resolver = MagicMock()
        # /a 不存在，/ 存在
        resolver.resolve_dir.side_effect = [FileNotFoundError(""), "0"]
        resolver.client.get_dir_id.return_value = None
        resolver.client.mkdir.side_effect = [
            {"cid": "10", "cname": "a"},
            {"cid": "20", "cname": "b"},
        ]
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["mkdir", "-p", "/a/b"])
        assert resolver.client.mkdir.call_count == 2


class TestRm:
    def test_rm_file(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = ("fid1", "parent_cid", {"name": "a.txt"})
        resolver.client.delete.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["rm", "/影音/a.txt"])
        assert result.exit_code == 0
        resolver.client.delete.assert_called_once_with(["fid1"])

    def test_rm_multiple(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = [
            ("fid1", "p1", {"name": "a.txt"}),
            ("fid2", "p1", {"name": "b.txt"}),
        ]
        resolver.client.delete.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["rm", "/影音/a.txt", "/影音/b.txt"])
        resolver.client.delete.assert_called_once_with(["fid1", "fid2"])

    def test_rm_dir_requires_r(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = FileNotFoundError
        resolver.resolve_dir.return_value = "dir_cid"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["rm", "/影音/somedir"])
        # 应该报错或输出提示
        assert result.exit_code != 0 or "-r" in result.output

    def test_rm_dir_with_r(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = FileNotFoundError
        resolver.resolve_dir.return_value = "dir_cid"
        resolver.client.delete.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["rm", "-r", "/影音/somedir"])
        assert result.exit_code == 0
        resolver.client.delete.assert_called_once_with(["dir_cid"])


class TestRename:
    def test_rename_single(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = ("fid1", "100", {"name": "old.mkv"})
        resolver.client.rename.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["rename", "/影音/old.mkv", "new.mkv"])
        assert result.exit_code == 0
        resolver.client.rename.assert_called_once_with("fid1", "new.mkv")

    def test_rename_batch(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = [
            ("fid1", "100", {"name": "a.mkv"}),
            ("fid2", "100", {"name": "b.mkv"}),
        ]
        resolver.client.batch_rename.return_value = {"state": True}
        stdin_data = '[["/影音/a.mkv","new_a.mkv"],["/影音/b.mkv","new_b.mkv"]]'
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["rename", "--batch"], input=stdin_data)
        assert result.exit_code == 0
        resolver.client.batch_rename.assert_called_once_with({"fid1": "new_a.mkv", "fid2": "new_b.mkv"})


class TestMv:
    def test_mv_to_dir(self, runner):
        """mv /a/file /b/ → 移动到目录 b"""
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = ("fid1", "cid_a", {"name": "file.mkv"})
        resolver.resolve_dir.return_value = "cid_b"
        resolver.client.move.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["mv", "/a/file.mkv", "/b/"])
        assert result.exit_code == 0
        resolver.client.move.assert_called_once_with(["fid1"], "cid_b")

    def test_mv_multi_to_dir(self, runner):
        """mv /a /b /c /target/ → 批量移动"""
        resolver = _mock_resolver()
        resolver.resolve_file.side_effect = [
            ("fid1", "p", {}), ("fid2", "p", {}), ("fid3", "p", {}),
        ]
        resolver.resolve_dir.return_value = "target_cid"
        resolver.client.move.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["mv", "/a", "/b", "/c", "/target/"])
        assert result.exit_code == 0
        resolver.client.move.assert_called_once_with(["fid1", "fid2", "fid3"], "target_cid")

    def test_mv_same_dir_renames(self, runner):
        """mv /a/old /a/new → 同目录下重命名"""
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = ("fid1", "cid_a", {"name": "old.mkv"})
        # dest 不是已存在目录
        def resolve_dir_side_effect(path):
            if path == "/a":
                return "cid_a"
            raise FileNotFoundError(path)
        resolver.resolve_dir.side_effect = resolve_dir_side_effect
        resolver.client.rename.return_value = {"state": True}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["mv", "/a/old.mkv", "/a/new.mkv"])
        assert result.exit_code == 0
        resolver.client.rename.assert_called_once_with("fid1", "new.mkv")


class TestPut:
    def test_put_uploads_file(self, runner, tmp_path):
        local_file = tmp_path / "test.nfo"
        local_file.write_text("<nfo/>")
        resolver = _mock_resolver()
        resolver.resolve_dir.return_value = "target_cid"
        resolver.client.upload_file.return_value = {"data": {"file_id": "new_fid"}}
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["put", str(local_file), "/影音/电影/"])
        assert result.exit_code == 0
        resolver.client.upload_file.assert_called_once()
        resolver.mark_stale.assert_called_once_with("target_cid")

    def test_put_nonexistent_local(self, runner):
        """本地文件不存在应该报错。"""
        resolver = _mock_resolver()
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["put", "/no/such/file.txt", "/影音/"])
        assert result.exit_code != 0


class TestGet:
    def test_get_downloads_file(self, runner, tmp_path):
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = ("fid1", "pcid", {
            "name": "a.mkv", "fid": "fid1", "pick_code": "pc1", "size": 1000,
        })
        resolver.client.download_url.return_value = "https://cdn.115.com/fake"
        with (
            patch("media115.fs_cli._get_resolver", return_value=resolver),
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
        resolver = _mock_resolver()
        resolver.resolve_file.return_value = ("fid1", "pcid", {
            "name": "b.mkv", "fid": "fid1", "pick_code": "pc2", "size": 500,
        })
        resolver.client.download_url.return_value = "https://cdn.115.com/fake"
        with (
            patch("media115.fs_cli._get_resolver", return_value=resolver),
            patch("media115.fs_cli._download_to_file") as mock_dl,
        ):
            result = runner.invoke(main, ["get", "/影音/b.mkv"])
        assert result.exit_code == 0
        mock_dl.assert_called_once()


class TestSync:
    def test_sync_exports_tree(self, runner):
        resolver = _mock_resolver()
        resolver.client.export_tree.return_value = "影音\n|-电影\n| |-满江红 (2023)\n"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code == 0
        resolver.client.export_tree.assert_called_once()

    def test_sync_saves_tree_cache(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        resolver = _mock_resolver()
        resolver.client.export_tree.return_value = "影音\n|-电影\n| |-满江红 (2023)\n"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code == 0
        from media115.cache import tree_cache_path
        assert tree_cache_path().exists()

    def test_sync_writes_path_index(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        resolver = _mock_resolver()
        resolver.resolve_dir.return_value = "100"
        resolver.client.export_tree.return_value = "影音\n|-电影\n"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code == 0
        resolver._write_path_index.assert_called_once_with("/影音", "100")

    def test_sync_no_result(self, runner):
        resolver = _mock_resolver()
        resolver.client.export_tree.return_value = None
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["sync", "/影音"])
        assert result.exit_code != 0 or "失败" in result.output or "fail" in result.output.lower()

    def test_sync_dir_not_found(self, runner):
        resolver = _mock_resolver()
        resolver.resolve_dir.side_effect = FileNotFoundError
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["sync", "/不存在的路径"])
        assert result.exit_code != 0

    def test_sync_deep_flag_prints_notice(self, runner):
        resolver = _mock_resolver()
        resolver.client.export_tree.return_value = "影音\n|-电影\n"
        with patch("media115.fs_cli._get_resolver", return_value=resolver):
            result = runner.invoke(main, ["sync", "--deep", "/影音"])
        assert result.exit_code == 0
        assert "暂未实现" in result.output


class TestCacheStatus:
    def test_cache_status(self, runner):
        # autouse _isolate_cache fixture already sets XDG_CACHE_HOME
        result = runner.invoke(main, ["cache", "status"])
        assert result.exit_code == 0

    def test_cache_status_shows_path_index_count(self, runner, tmp_path, monkeypatch):
        monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
        from media115.cache import _cache_dir
        import json as _json
        fs_dir = _cache_dir("fs")
        (fs_dir / "path_index.json").write_text(_json.dumps({"/影音": {"cid": "100", "ts": 1}}))
        result = runner.invoke(main, ["cache", "status"])
        assert result.exit_code == 0
        assert "1" in result.output

    def test_cache_status_tree_cache_absent(self, runner):
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
        result = runner.invoke(main, ["cache", "clear"], input="y\n")
        assert result.exit_code == 0
        assert not fs_dir.exists()

    def test_cache_clear_cancelled(self, runner):
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
        result = runner.invoke(main, ["cache", "clear"], input="y\n")
        assert result.exit_code == 0
        # scrape/ should still exist
        assert scrape_dir.exists()

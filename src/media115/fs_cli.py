"""115 网盘文件系统 CLI 命令。"""
from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import TYPE_CHECKING

import click

from media115.log import get_logger

if TYPE_CHECKING:
    from cloud115 import CachedClient


def _download_to_file(url: str, local_path: Path):
    """用 httpx 流式下载文件。"""
    import httpx
    with httpx.stream("GET", url, follow_redirects=True, timeout=60) as resp:
        resp.raise_for_status()
        with open(local_path, "wb") as f:
            for chunk in resp.iter_bytes(chunk_size=1024 * 1024):
                f.write(chunk)


def _get_client() -> "CachedClient":
    """获取已认证的 CachedClient。"""
    import os
    from cloud115 import CachedClient

    cookies = os.environ.get("CLOUD_115_COOKIES", "")
    if not cookies:
        raise click.ClickException("未登录。请先运行: media115 auth")
    return CachedClient(cookies)


def _get_cache_only():
    """获取 FileCache，不需要登录。用于 cache status/clear。"""
    from cloud115.cache import FileCache, _default_db_path
    db_path = _default_db_path()
    if not db_path.exists():
        return None
    return FileCache(db_path)


def _format_size(size: int) -> str:
    """把字节数格式化为人类可读的大小。"""
    for unit in ("B", "K", "M", "G", "T"):
        if size < 1024:
            return f"{size:.1f}{unit}" if unit != "B" else f"{size}{unit}"
        size /= 1024
    return f"{size:.1f}P"


def register(cli: click.Group):
    """把 fs 命令注册到 Click 命令组。"""

    @cli.command("ls")
    @click.argument("path", default="/")
    @click.option("-l", "long_fmt", is_flag=True, help="长格式（显示大小和类型）")
    @click.option("-R", "recursive", is_flag=True, help="递归显示")
    @click.option("--depth", default=2, type=int, help="递归深度（配合 -R，默认 2）")
    def ls(path, long_fmt, recursive, depth):
        """列出 115 网盘目录内容。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        try:
            items = client.list_dir(path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        _ls_dir(client, items, path, long_fmt, recursive, depth, indent=0)

    @cli.command("stat")
    @click.argument("path")
    def stat(path):
        """显示文件或目录的元数据（JSON 格式）。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        try:
            info = client.stat(path)
        except FileNotFoundError:
            raise click.ClickException(f"路径不存在: {path!r}")

        if info.get("type") == "dir":
            result = {"type": "directory", "cid": info.get("cid", ""), "path": path}
            click.echo(json.dumps(result, ensure_ascii=False, indent=2))
        else:
            click.echo(json.dumps(info, ensure_ascii=False, indent=2))

    @cli.command("find")
    @click.argument("keyword")
    @click.argument("path", default="/")
    def find(keyword, path):
        """在 115 网盘中搜索文件。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        try:
            results = client.search(keyword, path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        for item in results:
            name = item.get("fn", item.get("n", "?"))
            click.echo(name)

    @cli.command("mkdir")
    @click.argument("path")
    @click.option("-p", "parents", is_flag=True, help="递归创建中间目录")
    def mkdir(path, parents):
        """在 115 网盘中创建目录。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        # 规范化路径
        path = path.rstrip("/") or "/"
        if not path.startswith("/"):
            path = "/" + path

        if parents:
            # 先检查目标是否已存在
            try:
                client.resolve_path(path)
                click.echo(f"目录已存在: {path}")
                return
            except FileNotFoundError:
                pass

        try:
            client.mkdir(path, parents=parents)
        except FileNotFoundError:
            parent = path.rpartition("/")[0] or "/"
            raise click.ClickException(f"父目录不存在: {parent!r}，可以使用 -p 递归创建")

        get_logger().info("mkdir %s", path)
        click.echo(f"已创建: {path}")

    @cli.command("rm")
    @click.argument("paths", nargs=-1, required=True)
    @click.option("-r", "recursive", is_flag=True, help="递归删除目录")
    def rm(paths, recursive):
        """删除 115 网盘中的文件或目录。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        # Validate all paths first, check if dirs need -r
        resolved = []
        total = len(paths)
        for i, path in enumerate(paths, 1):
            if total > 1:
                print(f"\r  [{i}/{total}] {path[:60]}", end="", file=sys.stderr, flush=True)
            try:
                info = client.stat(path)
            except FileNotFoundError:
                raise click.ClickException(f"路径不存在: {path!r}")

            if info.get("type") == "dir" and not recursive:
                raise click.ClickException(f"{path!r} 是目录，需要 -r")
            resolved.append(path)

        if not resolved:
            return

        if total > 1:
            print("", file=sys.stderr)

        client.delete(resolved)
        get_logger().info("rm %d items", len(resolved))
        click.echo(f"已删除 {len(resolved)} 个项目")

    @cli.command("rename")
    @click.argument("path", required=False)
    @click.argument("new_name", required=False)
    @click.option("--batch", is_flag=True, help="从 stdin 读 JSON 批量重命名")
    def rename(path, new_name, batch):
        """重命名 115 网盘中的文件。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        if batch:
            raw = click.get_text_stream("stdin").read()
            try:
                pairs = json.loads(raw)
            except json.JSONDecodeError as e:
                raise click.ClickException(f"JSON 解析失败: {e}")

            total = len(pairs)
            for i, (item_path, _) in enumerate(pairs, 1):
                if total > 1:
                    print(f"\r  [{i}/{total}] {item_path[:60]}", end="", file=sys.stderr, flush=True)
            if total > 1:
                print("", file=sys.stderr)

            client.batch_rename(pairs)
            click.echo(f"已批量重命名 {len(pairs)} 个文件")
        else:
            if not path or not new_name:
                raise click.ClickException("需要提供 PATH 和 NEW_NAME，或使用 --batch 模式")

            old_name = path.rsplit("/", 1)[-1]
            try:
                client.rename(path, new_name)
            except FileNotFoundError:
                raise click.ClickException(f"文件不存在: {path!r}")
            get_logger().info("rename %s → %s", old_name, new_name)
            click.echo(f"已重命名: {old_name} → {new_name}")

    @cli.command("rapid")
    @click.argument("local_path", type=click.Path(exists=True))
    @click.argument("remote_dir")
    def rapid(local_path, remote_dir):
        """秒传：只传哈希，115 端去重。失败不 fallback。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        try:
            result = client.rapid_upload(local_path, remote_dir)
        except FileNotFoundError:
            raise click.ClickException(f"远程目录不存在: {remote_dir!r}")

        if result.get("status") == 2:
            pickcode = result.get("pickcode", "")
            local = Path(local_path)
            get_logger().info("rapid %s → %s (pickcode=%s)", local.name, remote_dir, pickcode)
            click.echo(f"秒传成功: {local.name} (pickcode={pickcode})")
        else:
            local = Path(local_path)
            raise click.ClickException(f"秒传失败: {local.name} (115 上没有此文件)")

    @cli.command("put")
    @click.argument("local_path", type=click.Path(exists=True))
    @click.argument("remote_dir")
    @click.option("--no-rapid", "no_rapid", is_flag=True,
                  help="跳过秒传，直接走普通上传")
    def put(local_path, remote_dir, no_rapid):
        """上传本地文件到 115 网盘目录。默认先尝试秒传，失败则走普通上传。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        local = Path(local_path)

        if not no_rapid:
            try:
                rapid_result = client.rapid_upload(local_path, remote_dir)
            except FileNotFoundError:
                raise click.ClickException(f"远程目录不存在: {remote_dir!r}")

            if rapid_result.get("status") == 2:
                get_logger().info("put %s → %s", local.name, remote_dir)
                click.echo(f"已上传（秒传）: {local.name}")
                return

        # 普通上传
        try:
            client.upload(local_path, remote_dir)
        except FileNotFoundError:
            raise click.ClickException(f"远程目录不存在: {remote_dir!r}")
        get_logger().info("put %s → %s", local.name, remote_dir)
        click.echo(f"已上传: {local.name}")

    @cli.command("get")
    @click.argument("remote_path")
    @click.argument("local_dir", default=".", required=False)
    def get(remote_path, local_dir):
        """从 115 网盘下载文件到本地目录。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        try:
            meta = client.find_file(remote_path)
        except FileNotFoundError:
            raise click.ClickException(f"远程文件不存在: {remote_path!r}")

        pick_code = meta.get("pick_code")
        if not pick_code:
            raise click.ClickException(f"文件缺少 pick_code: {remote_path!r}")

        filename = meta.get("name") or remote_path.rsplit("/", 1)[-1]
        url = client.download_url(pick_code)
        dest = Path(local_dir) / filename
        _download_to_file(url, dest)
        get_logger().info("get %s → %s", remote_path, local_dir)
        click.echo(f"已下载: {dest}")

    @cli.command("sync")
    @click.argument("path")
    @click.option("--deep", is_flag=True, help="递归预热目录 listing 缓存")
    @click.option("--depth", default=3, type=int, help="预热深度（配合 --deep，默认 3）")
    def sync(path, deep, depth):
        """刷新路径缓存。默认导出目录树，--deep 额外预热 listing 缓存。"""
        import media115.cache as media_cache

        client = _get_client()

        # Always export tree
        click.echo(f"正在导出目录树...")
        text = client.export_tree(path)
        if not text:
            raise click.ClickException(f"导出失败: {path!r}")

        tree_path = media_cache.tree_cache_path()
        tree_path.write_text(text, encoding="utf-8")

        lines = text.strip().split("\n")
        VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v"}
        video_count = sum(
            1 for ln in lines if any(ln.rstrip().lower().endswith(ext) for ext in VIDEO_EXTS)
        )
        click.echo(f"已保存 tree_cache.txt：{len(lines)} 行，{video_count} 个视频文件")

        if deep:
            click.echo(f"正在预热缓存 (深度={depth})...")
            client.warm(path, depth=depth)
            stats = client.cache_status()
            click.echo(f"预热完成：{stats['path_count']} 路径，{stats['dir_count']} 目录，{stats['entry_count']} 条目")

    @cli.group("cache")
    def cache_group():
        """缓存管理命令。"""

    @cache_group.command("status")
    def cache_status():
        """显示缓存状态统计。"""
        import time as _time
        import media115.cache as media_cache

        # cloud115 SQLite stats
        try:
            file_cache = _get_cache_only()
            if file_cache is not None:
                stats = file_cache.stats()
                path_count = stats.get("path_count", 0)
                dir_count = stats.get("dir_count", 0)
                entry_count = stats.get("entry_count", 0)
                db_size = stats.get("db_size_bytes", 0)
                rate_limit = stats.get("rate_limit", {})
            else:
                path_count = dir_count = entry_count = 0
                db_size = 0
                rate_limit = {}
        except Exception:
            path_count = dir_count = entry_count = "?"
            db_size = 0
            rate_limit = {}

        click.echo(f"cloud115 缓存:")
        click.echo(f"  路径映射:    {path_count} 条目")
        click.echo(f"  目录缓存:    {dir_count} 个目录, {entry_count} 条目")
        click.echo(f"  数据库大小:  {_format_size(db_size) if isinstance(db_size, int) else '?'}")

        # Rate limit status
        now = _time.time()
        for name, state in rate_limit.items():
            cooldown = state.get("cooldown_until", 0)
            if now < cooldown:
                remaining = int(cooldown - now)
                click.echo(f"  限流 ({name}):  ⚠ cooldown 中 (剩余 {remaining}s)")
            else:
                count = state.get("minute_count", 0)
                click.echo(f"  限流 ({name}):  正常 ({count}/20 QPM)")

        # media115 tree cache
        tree_path = media_cache.tree_cache_path()
        if tree_path.exists():
            tree_size = tree_path.stat().st_size
            click.echo(f"  tree_cache:  {_format_size(tree_size)}")
        else:
            click.echo(f"  tree_cache:  不存在")

    @cache_group.command("clear")
    @click.option("--tree", is_flag=True, help="也清除 tree_cache.txt")
    @click.option("--scrape", is_flag=True, help="也清除刮削缓存和 scrape_output")
    @click.option("--all", "clear_all", is_flag=True, help="清除全部缓存")
    def cache_clear(tree, scrape, clear_all):
        """清除缓存。默认只清 cloud115 路径/目录缓存。"""
        import shutil
        from media115.cache import _cache_root

        root = _cache_root()

        targets = []

        # cloud115 SQLite cache is always included
        try:
            file_cache = _get_cache_only()
            if file_cache is not None:
                stats = file_cache.stats()
                desc = "cloud115 路径/目录缓存 (%d paths, %d dirs, %d entries)" % (
                    stats.get("path_count", 0), stats.get("dir_count", 0), stats.get("entry_count", 0)
                )
                targets.append(("cloud115", desc, lambda fc=file_cache: fc.clear_metadata()))
        except Exception:
            pass

        if tree or clear_all:
            p = root / "tree_cache.txt"
            if p.exists():
                targets.append(("tree", "目录树缓存 (tree_cache.txt)", lambda: p.unlink()))

        if scrape or clear_all:
            for name, path in [("scrape", root / "scrape"), ("scrape_output", root / "scrape_output")]:
                if path.exists():
                    targets.append((name, f"刮削缓存 ({name}/)", lambda p=path: shutil.rmtree(p)))

        if clear_all:
            logs = root / "logs"
            if logs.exists():
                targets.append(("logs", "日志 (logs/)", lambda: shutil.rmtree(logs)))

        if not targets:
            click.echo("没有可清除的缓存")
            return

        click.echo("将清除：")
        for _, desc, _ in targets:
            click.echo(f"  - {desc}")

        if not click.confirm("确认？", default=False):
            click.echo("已取消")
            return

        for name, desc, action in targets:
            action()
            click.echo(f"  已清除: {desc}")

    @cli.command("mv")
    @click.argument("args", nargs=-1, required=True)
    def mv(args):
        """移动或重命名 115 网盘中的文件。"""
        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        if len(args) < 2:
            raise click.ClickException("需要至少两个路径参数：SRC DEST")

        srcs = list(args[:-1])
        dest = args[-1]

        if len(args) >= 3:
            # 多个源文件 → 目标必须是目录
            try:
                client.resolve_path(dest)
            except FileNotFoundError:
                raise click.ClickException(f"目标目录不存在: {dest!r}")

            client.move(srcs, dest)
            get_logger().info("mv %s → %s", srcs, dest)
            click.echo(f"已移动 {len(srcs)} 个文件到 {dest}")

        else:
            # 两个参数：src dest
            src = srcs[0]

            # 1. 先试 dest 是否已存在的目录
            try:
                client.resolve_path(dest)
                # dest 是已存在的目录 → 移动
                client.move([src], dest)
                get_logger().info("mv %s → %s", src, dest)
                click.echo(f"已移动: {src} → {dest}")
                return
            except FileNotFoundError:
                pass

            # 2. dest 不是已存在目录 → 解析 dest 的父目录和文件名
            dest_parent, _, dest_name = dest.rstrip("/").rpartition("/")
            dest_parent = dest_parent or "/"

            try:
                client.resolve_path(dest_parent)
            except FileNotFoundError:
                raise click.ClickException(f"目标父目录不存在: {dest_parent!r}")

            # 确定 src 的父目录
            src_parent = src.rstrip("/").rpartition("/")[0] or "/"

            # 规范化比较
            src_parent_n = src_parent.rstrip("/") or "/"
            dest_parent_n = dest_parent.rstrip("/") or "/"

            if src_parent_n == dest_parent_n:
                # 同目录 → 重命名
                client.rename(src, dest_name)
                src_name = src.rstrip("/").rsplit("/", 1)[-1]
                get_logger().info("mv %s → %s", src, dest)
                click.echo(f"已重命名: {src_name} → {dest_name}")
            else:
                # 跨目录 + 改名 → 先移动到目标父目录，再重命名
                src_name = src.rstrip("/").rsplit("/", 1)[-1]
                client.move([src], dest_parent)
                new_path = dest_parent.rstrip("/") + "/" + src_name
                client.rename(new_path, dest_name)
                get_logger().info("mv %s → %s", src, dest)
                click.echo(f"已移动并重命名: {src} → {dest}")


    @cli.command()
    def init():
        """初始化 media115：复制 skills 到 ~/.claude/skills/，生成默认配置。"""
        import shutil
        from media115.cache import _config_root

        # 查找 _skills 目录
        skills_src = Path(__file__).parent / "_skills"
        if not skills_src.exists():
            raise click.ClickException(f"Skills 目录不存在: {skills_src}")

        # 复制 skills
        skills_dst = Path.home() / ".claude" / "skills" / "media115"
        if skills_dst.exists():
            shutil.rmtree(skills_dst)
        shutil.copytree(skills_src, skills_dst)
        click.echo(f"Skills: {skills_dst}")

        # 创建配置目录
        config_dir = _config_root()
        config_dir.mkdir(parents=True, exist_ok=True)

        # 生成默认配置
        config_path = config_dir / "config.yaml"
        if not config_path.exists():
            import yaml
            from media115.cli import _DEFAULT_CONFIG
            config_path.write_text(yaml.dump(_DEFAULT_CONFIG, allow_unicode=True, default_flow_style=False))
        click.echo(f"Config: {config_path}")

        # 生成 .env 模板
        env_path = config_dir / ".env"
        if not env_path.exists():
            env_path.write_text(
                "# media115 credentials\n"
                "TMDB_READ_ACCESS_TOKEN=\n"
                "# BANGUMI_ACCESS_TOKEN=\n"
                "# CLOUD_115_COOKIES= (set by media115 auth)\n"
            )
            env_path.chmod(0o600)
        click.echo(f"Env: {env_path}")

        click.echo("\nNext: media115 auth")

    @cli.command()
    def doctor():
        """检查环境状态和配置。"""
        import importlib.metadata
        import sys
        from media115.cache import _cache_root, _config_root

        # Python + 版本
        click.echo("环境检查:")
        click.echo(f"  Python:        {sys.version.split()[0]}")
        try:
            ver = importlib.metadata.version("media115")
        except importlib.metadata.PackageNotFoundError:
            ver = "dev"
        click.echo(f"  media115:      {ver}")
        click.echo()

        # 配置文件
        config_root = _config_root()
        env_path = config_root / ".env"
        config_path = config_root / "config.yaml"
        click.echo("配置:")
        click.echo(f"  .env:          {env_path} {'✓' if env_path.exists() else '✗ 不存在'}")
        click.echo(f"  config.yaml:   {config_path} {'✓' if config_path.exists() else '✗ (使用默认)'}")
        click.echo()

        # 认证
        import os
        from media115.cli import _load_env
        _load_env()
        cookies = os.environ.get("CLOUD_115_COOKIES", "")
        tmdb = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
        bangumi = os.environ.get("BANGUMI_ACCESS_TOKEN", "")
        click.echo("认证:")
        click.echo(f"  115 Cookies:   {'✓ 已配置' if cookies else '✗ 未配置'}")
        click.echo(f"  TMDB Token:    {'✓ 已配置' if tmdb else '✗ 未配置'}")
        click.echo(f"  Bangumi Token: {'✓ 已配置' if bangumi else '✗ 未配置 (可选)'}")
        click.echo()

        # 缓存
        import time as _time
        cache_root = _cache_root()
        click.echo("缓存:")
        click.echo(f"  缓存根目录:    {cache_root}")

        # cloud115 SQLite cache
        try:
            client = _get_client()
            stats = client.cache_status()
            db_size = stats.get("db_size_bytes", 0)
            click.echo(f"  path_index:    {stats.get('path_count', 0)} 条目")
            click.echo(f"  dir listings:  {stats.get('dir_count', 0)} 目录, {stats.get('entry_count', 0)} 条目")
            click.echo(f"  db 大小:       {_format_size(db_size) if isinstance(db_size, int) else '?'}")
        except Exception:
            click.echo("  path_index:    ? (需要登录)")
            click.echo("  dir listings:  ? (需要登录)")
            click.echo("  db 大小:       ? (需要登录)")

        # tree_cache
        tc = cache_root / "tree_cache.txt"
        if tc.exists():
            lines = tc.read_text().count("\n")
            mtime = _time.strftime("%Y-%m-%d %H:%M", _time.localtime(tc.stat().st_mtime))
            click.echo(f"  tree_cache:    ✓ {lines} 行 ({mtime})")
        else:
            click.echo("  tree_cache:    ✗ 不存在 (run: media115 sync)")

        # scrape_output
        scrape_output = cache_root / "scrape_output"
        if scrape_output.exists():
            cats = [d.name for d in scrape_output.iterdir() if d.is_dir()]
            click.echo(f"  scrape_output: {len(cats)} 分类")
        else:
            click.echo("  scrape_output: ✗ 不存在")

        click.echo()

        # Skills
        skills_dir = Path.home() / ".claude" / "skills" / "media115"
        click.echo("Skills:")
        if skills_dir.exists():
            skill_count = len([d for d in skills_dir.iterdir() if d.is_dir() and (d / "SKILL.md").exists()])
            click.echo(f"  {skills_dir}: ✓ {skill_count} skills")
        else:
            click.echo(f"  {skills_dir}: ✗ 未安装 (run: media115 init)")


    @cli.command("dedup")
    @click.argument("path")
    @click.option("--execute", is_flag=True, help="执行删除（默认 dry-run）")
    def dedup(path, execute):
        """清理目录下的重复文件。

        扫描子目录，按文件名分组，同名文件保留一个，删除其余副本。
        115 上传同名文件时不覆盖而是创建完全同名的副本，此命令清理这些副本。
        """
        from collections import defaultdict

        try:
            client = _get_client()
        except click.ClickException as e:
            raise e

        try:
            items = client.list_dir(path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        subdirs = [
            (item["name"], path.rstrip("/") + "/" + item["name"])
            for item in items
            if item.get("type") == "dir"
        ]

        total_dupes = 0
        all_dupe_paths: list[str] = []

        for dir_name, dir_path in subdirs:
            files = client.list_dir_uncached(dir_path)
            # 按文件名分组
            by_name: dict[str, list[dict]] = defaultdict(list)
            for f in files:
                if f.get("type") != "file":
                    continue
                by_name[f["name"]].append(f)

            # 同名文件 > 1 个的，保留第一个，其余是重复
            dupes = []
            for name, file_list in by_name.items():
                if len(file_list) > 1:
                    # Keep first, mark rest as dupes
                    for f in file_list[1:]:
                        dupe_path = dir_path.rstrip("/") + "/" + f["name"]
                        dupes.append((name, dupe_path, f.get("fid", "")))

            if dupes:
                click.echo(f"{dir_name}/: {len(dupes)} 个重复文件")
                shown = set()
                for name, _, _ in dupes[:5]:
                    if name not in shown:
                        count = sum(1 for n, _, _ in dupes if n == name)
                        click.echo(f"  {name} (x{count} 副本)")
                        shown.add(name)
                if len(dupes) > 5:
                    click.echo(f"  ...")
                total_dupes += len(dupes)
                all_dupe_paths.extend(dp for _, dp, _ in dupes)

        click.echo(f"\n共 {total_dupes} 个重复文件待清理")

        if not all_dupe_paths:
            return

        if execute:
            batch_size = 50
            deleted = 0
            for i in range(0, len(all_dupe_paths), batch_size):
                batch = all_dupe_paths[i:i + batch_size]
                client.delete(batch)
                deleted += len(batch)
                click.echo(f"  已删除 {deleted}/{len(all_dupe_paths)}")
            click.echo(f"清理完成: 删除 {len(all_dupe_paths)} 个重复文件")
        else:
            click.echo("(dry-run) 使用 --execute 执行删除")


def _ls_dir(
    client,
    items: list,
    label: str,
    long_fmt: bool,
    recursive: bool,
    depth: int,
    indent: int,
):
    """递归打印目录内容。"""
    prefix = "  " * indent

    for item in items:
        is_dir = item.get("type") == "dir"
        name = item["name"]

        if long_fmt:
            if is_dir:
                click.echo(f"{prefix}d  {'':>8}  {name}/")
            else:
                size_str = _format_size(item.get("size", 0))
                click.echo(f"{prefix}f  {size_str:>8}  {name}")
        else:
            if is_dir:
                click.echo(f"{prefix}{name}/")
            else:
                click.echo(f"{prefix}{name}")

        if recursive and is_dir and depth > 0:
            child_path = label.rstrip("/") + "/" + name
            child_items = client.list_dir(child_path)
            _ls_dir(client, child_items, child_path, long_fmt, recursive, depth - 1, indent + 1)

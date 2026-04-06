"""115 网盘文件系统 CLI 命令。"""
from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import TYPE_CHECKING

import click

from media115.log import get_logger

if TYPE_CHECKING:
    from media115.fs import PathResolver


def _download_to_file(url: str, local_path: Path):
    """用 httpx 流式下载文件。"""
    import httpx
    with httpx.stream("GET", url, follow_redirects=True, timeout=60) as resp:
        resp.raise_for_status()
        with open(local_path, "wb") as f:
            for chunk in resp.iter_bytes(chunk_size=1024 * 1024):
                f.write(chunk)


def _get_resolver() -> "PathResolver":
    """获取已认证的 PathResolver。"""
    from media115.cli import _get_115_client
    from media115.fs import PathResolver

    client = _get_115_client()
    if not client:
        raise click.ClickException("未登录。请先运行: media115 auth")
    return PathResolver(client)


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
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            cid = resolver.resolve_dir(path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        _ls_dir(resolver, cid, path, long_fmt, recursive, depth, indent=0)

    @cli.command("stat")
    @click.argument("path")
    def stat(path):
        """显示文件或目录的元数据（JSON 格式）。"""
        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        # 先尝试解析为文件
        try:
            fid, parent_cid, meta = resolver.resolve_file(path)
            click.echo(json.dumps(meta, ensure_ascii=False, indent=2))
            return
        except FileNotFoundError:
            pass

        # 再尝试解析为目录
        try:
            cid = resolver.resolve_dir(path)
            result = {"type": "directory", "cid": cid, "path": path}
            click.echo(json.dumps(result, ensure_ascii=False, indent=2))
            return
        except FileNotFoundError:
            pass

        raise click.ClickException(f"路径不存在: {path!r}")

    @cli.command("find")
    @click.argument("keyword")
    @click.argument("path", default="/")
    def find(keyword, path):
        """在 115 网盘中搜索文件。"""
        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            cid = resolver.resolve_dir(path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        results = resolver.client.search(keyword, dir_id=cid)
        for item in results:
            name = item.get("fn", item.get("n", "?"))
            click.echo(name)

    @cli.command("mkdir")
    @click.argument("path")
    @click.option("-p", "parents", is_flag=True, help="递归创建中间目录")
    def mkdir(path, parents):
        """在 115 网盘中创建目录。"""
        import posixpath

        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        # 规范化路径
        path = path.rstrip("/") or "/"
        if not path.startswith("/"):
            path = "/" + path

        if parents:
            # 先检查目标是否已存在
            try:
                resolver.resolve_dir(path)
                click.echo(f"目录已存在: {path}")
                return
            except FileNotFoundError:
                pass
            # 找到第一个存在的祖先目录，然后逐级创建
            parts = [p for p in path.split("/") if p]
            # 从父目录往上找第一个存在的（跳过目标本身，从 len(parts)-1 往下）
            existing_cid = None
            existing_idx = -1
            for i in range(len(parts) - 1, -1, -1):
                ancestor = "/" + "/".join(parts[:i]) if i > 0 else "/"
                try:
                    existing_cid = resolver.resolve_dir(ancestor)
                    existing_idx = i
                    break
                except FileNotFoundError:
                    continue

            if existing_cid is None:
                raise click.ClickException("无法解析根目录")

            # 逐级创建缺失的目录
            current_cid = existing_cid
            current_path = "/" + "/".join(parts[:existing_idx]) if existing_idx > 0 else "/"
            for part in parts[existing_idx:]:
                result = resolver.client.mkdir(current_cid, part)
                new_cid = str(result["cid"])
                resolver.update_dir_entry(current_path, current_cid, part, new_cid)
                current_path = current_path.rstrip("/") + "/" + part
                current_cid = new_cid
            get_logger().info("mkdir %s", path)
            click.echo(f"已创建: {path}")
        else:
            # 普通模式：解析父目录，创建最后一段
            parent, _, name = path.rpartition("/")
            parent = parent or "/"
            if not name:
                raise click.ClickException(f"无效路径: {path!r}")

            try:
                parent_cid = resolver.resolve_dir(parent)
            except FileNotFoundError:
                raise click.ClickException(f"父目录不存在: {parent!r}，可以使用 -p 递归创建")

            result = resolver.client.mkdir(parent_cid, name)
            new_cid = str(result["cid"])
            resolver.update_dir_entry(parent, parent_cid, name, new_cid)
            get_logger().info("mkdir %s", path)
            click.echo(f"已创建: {path}")

    @cli.command("rm")
    @click.argument("paths", nargs=-1, required=True)
    @click.option("-r", "recursive", is_flag=True, help="递归删除目录")
    def rm(paths, recursive):
        """删除 115 网盘中的文件或目录。"""
        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        ids = []
        # (parent_cid, name) pairs for cache update
        cache_entries = []

        total = len(paths)
        for i, path in enumerate(paths, 1):
            if total > 1:
                print(f"\r  [{i}/{total}] {path[:60]}", end="", file=sys.stderr, flush=True)
            # 先尝试解析为文件
            try:
                fid, parent_cid, meta = resolver.resolve_file(path)
                ids.append(fid)
                name = meta.get("name", path.rsplit("/", 1)[-1])
                cache_entries.append((parent_cid, name))
                continue
            except FileNotFoundError:
                pass

            # 再尝试解析为目录
            try:
                cid = resolver.resolve_dir(path)
                if not recursive:
                    raise click.ClickException(f"{path!r} 是目录，需要 -r")
                ids.append(cid)
                parent = path.rstrip("/").rpartition("/")[0] or "/"
                name = path.rstrip("/").rsplit("/", 1)[-1]
                # 目录删除后需要 invalidate，记录 parent_cid 稍后处理
                cache_entries.append((None, path))  # sentinel for dir
                continue
            except FileNotFoundError:
                pass

            raise click.ClickException(f"路径不存在: {path!r}")

        if not ids:
            return

        if total > 1:
            print("", file=sys.stderr)
        resolver.client.delete(ids)

        # 更新缓存
        for entry in cache_entries:
            parent_cid, name_or_path = entry
            if parent_cid is None:
                # 目录：从 path_index 中移除
                resolver.invalidate(name_or_path)
            else:
                resolver.remove_from_listing(parent_cid, name_or_path)

        get_logger().info("rm %d items", len(ids))
        click.echo(f"已删除 {len(ids)} 个项目")

    @cli.command("rename")
    @click.argument("path", required=False)
    @click.argument("new_name", required=False)
    @click.option("--batch", is_flag=True, help="从 stdin 读 JSON 批量重命名")
    def rename(path, new_name, batch):
        """重命名 115 网盘中的文件。"""
        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        if batch:
            import sys
            raw = click.get_text_stream("stdin").read()
            try:
                pairs = json.loads(raw)
            except json.JSONDecodeError as e:
                raise click.ClickException(f"JSON 解析失败: {e}")

            renames = {}
            cache_updates = []
            total = len(pairs)
            for i, (item_path, item_new_name) in enumerate(pairs, 1):
                if total > 1:
                    print(f"\r  [{i}/{total}] {item_path[:60]}", end="", file=sys.stderr, flush=True)
                try:
                    fid, parent_cid, meta = resolver.resolve_file(item_path)
                except FileNotFoundError:
                    raise click.ClickException(f"文件不存在: {item_path!r}")
                old_name = meta.get("name", item_path.rsplit("/", 1)[-1])
                renames[fid] = item_new_name
                cache_updates.append((parent_cid, old_name, fid, item_new_name))
            if total > 1:
                print("", file=sys.stderr)

            resolver.client.batch_rename(renames)

            for parent_cid, old_name, fid, item_new_name in cache_updates:
                resolver.remove_from_listing(parent_cid, old_name)
                resolver.mark_stale(parent_cid)

            click.echo(f"已批量重命名 {len(renames)} 个文件")
        else:
            if not path or not new_name:
                raise click.ClickException("需要提供 PATH 和 NEW_NAME，或使用 --batch 模式")

            try:
                fid, parent_cid, meta = resolver.resolve_file(path)
            except FileNotFoundError:
                raise click.ClickException(f"文件不存在: {path!r}")

            old_name = meta.get("name", path.rsplit("/", 1)[-1])
            resolver.client.rename(fid, new_name)
            resolver.remove_from_listing(parent_cid, old_name)
            resolver.mark_stale(parent_cid)
            get_logger().info("rename %s → %s", old_name, new_name)
            click.echo(f"已重命名: {old_name} → {new_name}")

    @cli.command("rapid")
    @click.argument("local_path", type=click.Path(exists=True))
    @click.argument("remote_dir")
    def rapid(local_path, remote_dir):
        """秒传：只传哈希，115 端去重。失败不 fallback。"""
        import hashlib as _hl

        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            cid = resolver.resolve_dir(remote_dir)
        except FileNotFoundError:
            raise click.ClickException(f"远程目录不存在: {remote_dir!r}")

        local = Path(local_path)

        # 计算文件完整 SHA1
        h = _hl.sha1()
        with open(local, "rb") as f:
            while chunk := f.read(1024 * 1024):
                h.update(chunk)
        file_sha1 = h.hexdigest().upper()

        with open(local, "rb") as f:
            result = resolver.client.rapid_upload(
                cid,
                local.name,
                local.stat().st_size,
                file_sha1,
                file_stream=f,
            )

        if result.get("status") == 2:
            pickcode = result['pickcode']
            get_logger().info("rapid %s → %s (pickcode=%s)", local.name, remote_dir, pickcode)
            click.echo(f"秒传成功: {local.name} (pickcode={pickcode})")
            resolver.mark_stale(cid)
        else:
            raise click.ClickException(f"秒传失败: {local.name} (115 上没有此文件)")

    @cli.command("put")
    @click.argument("local_path", type=click.Path(exists=True))
    @click.argument("remote_dir")
    @click.option("--no-rapid", "no_rapid", is_flag=True,
                  help="跳过秒传，直接走普通上传")
    def put(local_path, remote_dir, no_rapid):
        """上传本地文件到 115 网盘目录。默认先尝试秒传，失败则走普通上传。"""
        import hashlib as _hl

        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            cid = resolver.resolve_dir(remote_dir)
        except FileNotFoundError:
            raise click.ClickException(f"远程目录不存在: {remote_dir!r}")

        local = Path(local_path)

        if not no_rapid:
            # 先尝试秒传
            h = _hl.sha1()
            with open(local, "rb") as f:
                while chunk := f.read(1024 * 1024):
                    h.update(chunk)
            file_sha1 = h.hexdigest().upper()

            with open(local, "rb") as f:
                rapid_result = resolver.client.rapid_upload(
                    cid,
                    local.name,
                    local.stat().st_size,
                    file_sha1,
                    file_stream=f,
                )

            if rapid_result.get("status") == 2:
                get_logger().info("put %s → %s", local.name, remote_dir)
                click.echo(f"已上传（秒传）: {local.name}")
                resolver.mark_stale(cid)
                return

        # 普通上传
        resolver.client.upload_file(local, cid)
        resolver.mark_stale(cid)
        get_logger().info("put %s → %s", local.name, remote_dir)
        click.echo(f"已上传: {local.name}")

    @cli.command("get")
    @click.argument("remote_path")
    @click.argument("local_dir", default=".", required=False)
    def get(remote_path, local_dir):
        """从 115 网盘下载文件到本地目录。"""
        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            fid, parent_cid, meta = resolver.resolve_file(remote_path)
        except FileNotFoundError:
            raise click.ClickException(f"远程文件不存在: {remote_path!r}")

        pick_code = meta.get("pick_code")
        if not pick_code:
            raise click.ClickException(f"文件缺少 pick_code: {remote_path!r}")

        filename = meta.get("name") or remote_path.rsplit("/", 1)[-1]
        url = resolver.client.download_url(pick_code)
        dest = Path(local_dir) / filename
        _download_to_file(url, dest)
        get_logger().info("get %s → %s", remote_path, local_dir)
        click.echo(f"已下载: {dest}")

    @cli.command("sync")
    @click.argument("path")
    @click.option("--deep", is_flag=True, help="额外填充 dir listing 缓存（暂未实现）")
    def sync(path, deep):
        """刷新路径缓存：导出目录树并保存到 tree_cache.txt。"""
        import media115.cache as media_cache

        if deep:
            click.echo("--deep 暂未实现，已忽略该标志")

        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            cid = resolver.resolve_dir(path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        click.echo(f"正在导出目录树 (cid={cid})...")
        text = resolver.client.export_tree(cid)

        if not text:
            raise click.ClickException(f"导出失败: {path!r}")

        # 保存到 tree_cache.txt
        tree_path = media_cache.tree_cache_path()
        tree_path.write_text(text, encoding="utf-8")

        # 确保 path 本身写入 path_index（规范化路径以避免缓存 miss）
        from media115.fs import _normalize
        resolver._write_path_index(_normalize(path), cid)

        # 统计
        lines = text.strip().split("\n")
        VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v"}
        video_count = sum(
            1 for ln in lines if any(ln.rstrip().lower().endswith(ext) for ext in VIDEO_EXTS)
        )
        get_logger().info("sync %s: %d lines, %d videos", path, len(lines), video_count)
        click.echo(f"已保存 tree_cache.txt：{len(lines)} 行，{video_count} 个视频文件")

    @cli.group("cache")
    def cache_group():
        """缓存管理命令。"""

    @cache_group.command("status")
    def cache_status():
        """显示缓存状态统计。"""
        import media115.cache as media_cache

        cache_root = media_cache._cache_root()
        tree_path = media_cache.tree_cache_path()
        fs_dir = media_cache._cache_dir("fs")
        dir_dir = media_cache._cache_dir("fs/dir")

        # path_index 条目数
        path_index_file = fs_dir / "path_index.json"
        if path_index_file.exists():
            try:
                import json as _json
                index = _json.loads(path_index_file.read_text(encoding="utf-8"))
                index_count = len(index)
            except Exception:
                index_count = 0
        else:
            index_count = 0

        # dir listing 文件数
        dir_listing_count = len(list(dir_dir.glob("*.json"))) if dir_dir.exists() else 0

        # tree_cache.txt
        if tree_path.exists():
            tree_size = tree_path.stat().st_size
            tree_info = f"存在 ({_format_size(tree_size)})"
        else:
            tree_info = "不存在"

        click.echo(f"缓存根目录:        {cache_root}")
        click.echo(f"path_index 条目数: {index_count}")
        click.echo(f"dir listing 文件数: {dir_listing_count}")
        click.echo(f"tree_cache.txt:    {tree_info}")

    @cache_group.command("clear")
    @click.option("--tree", is_flag=True, help="也清除 tree_cache.txt")
    @click.option("--scrape", is_flag=True, help="也清除刮削缓存和 scrape_output")
    @click.option("--all", "clear_all", is_flag=True, help="清除全部缓存")
    def cache_clear(tree, scrape, clear_all):
        """清除缓存。默认只清路径缓存（fs/）。"""
        import shutil
        from media115.cache import _cache_root

        root = _cache_root()

        targets = []
        targets.append(("路径缓存 (fs/)", root / "fs"))
        if tree or clear_all:
            targets.append(("目录树缓存 (tree_cache.txt)", root / "tree_cache.txt"))
        if scrape or clear_all:
            targets.append(("刮削缓存 (scrape/)", root / "scrape"))
            targets.append(("刮削输出 (scrape_output/)", root / "scrape_output"))
        if clear_all:
            targets.append(("日志 (logs/)", root / "logs"))

        existing = [(name, path) for name, path in targets if path.exists()]
        if not existing:
            click.echo("没有可清除的缓存")
            return

        click.echo("将清除：")
        for name, path in existing:
            click.echo(f"  - {name}")
        if not click.confirm("确认？", default=False):
            click.echo("已取消")
            return

        for name, path in existing:
            if path.is_file():
                path.unlink()
            elif path.is_dir():
                shutil.rmtree(path)
            click.echo(f"  已清除: {name}")

    @cli.command("mv")
    @click.argument("args", nargs=-1, required=True)
    def mv(args):
        """移动或重命名 115 网盘中的文件。"""
        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        if len(args) < 2:
            raise click.ClickException("需要至少两个路径参数：SRC DEST")

        srcs = list(args[:-1])
        dest = args[-1]

        if len(args) >= 3:
            # 多个源文件 → 目标必须是目录
            try:
                target_cid = resolver.resolve_dir(dest)
            except FileNotFoundError:
                raise click.ClickException(f"目标目录不存在: {dest!r}")

            fids = []
            cache_entries = []
            for src in srcs:
                try:
                    fid, parent_cid, meta = resolver.resolve_file(src)
                    name = meta.get("name", src.rsplit("/", 1)[-1])
                except FileNotFoundError:
                    # 可能是目录
                    try:
                        cid = resolver.resolve_dir(src)
                        fid = cid
                        src_parent = src.rstrip("/").rpartition("/")[0] or "/"
                        parent_cid = resolver.resolve_dir(src_parent)
                        name = src.rstrip("/").rsplit("/", 1)[-1]
                    except FileNotFoundError:
                        raise click.ClickException(f"文件或目录不存在: {src!r}")
                fids.append(fid)
                cache_entries.append((parent_cid, name))

            resolver.client.move(fids, target_cid)

            for parent_cid, name in cache_entries:
                resolver.remove_from_listing(parent_cid, name)
            resolver.mark_stale(target_cid)
            get_logger().info("mv %s → %s", srcs, dest)
            click.echo(f"已移动 {len(fids)} 个文件到 {dest}")

        else:
            # 两个参数：src dest
            src = srcs[0]

            is_dir = False
            try:
                fid, src_parent_cid, meta = resolver.resolve_file(src)
                src_name = meta.get("name", src.rsplit("/", 1)[-1])
            except FileNotFoundError:
                # 可能是目录
                try:
                    fid = resolver.resolve_dir(src)
                    is_dir = True
                    src_parent = src.rstrip("/").rpartition("/")[0] or "/"
                    src_parent_cid = resolver.resolve_dir(src_parent)
                    src_name = src.rstrip("/").rsplit("/", 1)[-1]
                except FileNotFoundError:
                    raise click.ClickException(f"文件或目录不存在: {src!r}")

            # 1. 先试 dest 是否已存在的目录
            try:
                target_cid = resolver.resolve_dir(dest)
                # dest 是已存在的目录 → 移动
                resolver.client.move([fid], target_cid)
                resolver.remove_from_listing(src_parent_cid, src_name)
                resolver.mark_stale(target_cid)
                get_logger().info("mv %s → %s", src, dest)
                click.echo(f"已移动: {src} → {dest}")
                return
            except FileNotFoundError:
                pass

            # 2. dest 不是已存在目录 → 解析 dest 的父目录和文件名
            dest_parent, _, dest_name = dest.rstrip("/").rpartition("/")
            dest_parent = dest_parent or "/"

            try:
                dest_parent_cid = resolver.resolve_dir(dest_parent)
            except FileNotFoundError:
                raise click.ClickException(f"目标父目录不存在: {dest_parent!r}")

            if src_parent_cid == dest_parent_cid:
                # 同目录 → 重命名
                resolver.client.rename(fid, dest_name)
                resolver.remove_from_listing(src_parent_cid, src_name)
                resolver.mark_stale(src_parent_cid)
                get_logger().info("mv %s → %s", src, dest)
                click.echo(f"已重命名: {src_name} → {dest_name}")
            else:
                # 跨目录 + 改名 → 先移动，再重命名
                resolver.client.move([fid], dest_parent_cid)
                resolver.client.rename(fid, dest_name)
                resolver.remove_from_listing(src_parent_cid, src_name)
                resolver.mark_stale(dest_parent_cid)
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
        import json as _json
        import time as _time
        cache_root = _cache_root()
        click.echo("缓存:")
        click.echo(f"  缓存根目录:    {cache_root}")

        # path_index
        pi = cache_root / "fs" / "path_index.json"
        if pi.exists():
            try:
                count = len(_json.loads(pi.read_text()))
            except Exception:
                count = "?"
            click.echo(f"  path_index:    {count} 条目")
        else:
            click.echo("  path_index:    ✗ 不存在")

        # dir listings
        dir_cache = cache_root / "fs" / "dir"
        if dir_cache.exists():
            count = len(list(dir_cache.glob("*.json")))
            click.echo(f"  dir listings:  {count} 文件")
        else:
            click.echo("  dir listings:  ✗ 不存在")

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

        # rate_limit
        rl = cache_root / "rate_limit.json"
        if rl.exists():
            try:
                state = _json.loads(rl.read_text())
                cooldown = state.get("cooldown_until", 0)
                if _time.time() < cooldown:
                    remaining = int(cooldown - _time.time())
                    click.echo(f"  rate_limit:    ⚠ cooldown 中 (剩余 {remaining}s)")
                else:
                    click.echo("  rate_limit:    ✓ 正常")
            except Exception:
                click.echo("  rate_limit:    ? 读取失败")
        else:
            click.echo("  rate_limit:    ✓ 无状态文件")

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
        """清理目录下的重复文件（名字包含 (1), (2) 等后缀的副本）。"""
        import re

        try:
            resolver = _get_resolver()
        except click.ClickException as e:
            raise e

        try:
            cid = resolver.resolve_dir(path)
        except FileNotFoundError:
            raise click.ClickException(f"目录不存在: {path!r}")

        # List top-level items to find subdirectories (one level deep only)
        items = resolver.client.list_files_all(dir_id=cid)
        subdirs = [
            (item.get("n", ""), str(item.get("cid", "")))
            for item in items
            if "fid" not in item and item.get("cid")
        ]

        total_dupes = 0
        all_dupe_fids: list[str] = []

        for dir_name, dir_cid in subdirs:
            files = resolver.client.list_files_all(dir_id=dir_cid)
            dupes = []
            for f in files:
                if "fid" not in f:
                    continue
                name = f.get("fn", f.get("n", ""))
                # Match "xxx(1).nfo", "xxx(2).jpg", etc. — 115's automatic dupe suffix
                if re.search(r'\(\d+\)\.\w+$', name):
                    dupes.append((name, str(f["fid"])))

            if dupes:
                click.echo(f"{dir_name}/: {len(dupes)} 个重复文件")
                preview = dupes[:3]
                for name, _fid in preview:
                    click.echo(f"  {name}")
                if len(dupes) > 3:
                    click.echo(f"  ... 等共 {len(dupes)} 个")
                total_dupes += len(dupes)
                all_dupe_fids.extend(fid for _, fid in dupes)

        click.echo(f"\n共 {total_dupes} 个重复文件")

        if not all_dupe_fids:
            return

        if execute:
            # Delete in batches of 50 to avoid oversized API requests
            batch_size = 50
            deleted = 0
            for i in range(0, len(all_dupe_fids), batch_size):
                batch = all_dupe_fids[i:i + batch_size]
                resolver.client.delete(batch)
                deleted += len(batch)
                click.echo(f"  已删除 {len(batch)} 个 ({deleted}/{len(all_dupe_fids)})")
            click.echo(f"清理完成: 删除 {len(all_dupe_fids)} 个重复文件")
        else:
            click.echo("(dry-run) 使用 --execute 执行删除")


def _ls_dir(
    resolver,
    cid: str,
    label: str,
    long_fmt: bool,
    recursive: bool,
    depth: int,
    indent: int,
):
    """递归打印目录内容。"""
    items = resolver.listing(cid)
    prefix = "  " * indent

    for item in items:
        is_dir = "cid" in item and "fid" not in item
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
            child_cid = item["cid"]
            _ls_dir(resolver, child_cid, name, long_fmt, recursive, depth - 1, indent + 1)

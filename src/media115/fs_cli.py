"""115 网盘文件系统 CLI 命令。"""
from __future__ import annotations

import json
from typing import TYPE_CHECKING

import click

if TYPE_CHECKING:
    from media115.fs import PathResolver


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

"""CLI entry point for 115-media."""

import os
from pathlib import Path

import click
import yaml


def _load_env():
    env_path = Path.cwd() / ".env"
    if not env_path.exists():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, val = line.split("=", 1)
        val = val.strip().strip("'\"")
        os.environ.setdefault(key.strip(), val)


def _load_config() -> dict:
    config_path = Path.cwd() / "config.yaml"
    if config_path.exists():
        return yaml.safe_load(config_path.read_text()) or {}
    return {}


@click.group()
def main():
    """115-media: Media library management with 115 cloud."""
    _load_env()


@main.command()
@click.option("--app", default="tv", help="Device type for QR login (tv/qandroid/web)")
def auth(app):
    """Login to 115 via QR code scan. Saves cookies to .env."""
    from media115.client import Cloud115Client

    client = Cloud115Client.qr_login(app=app)
    env_path = Path.cwd() / ".env"
    client.save_cookies_to_env(env_path)
    click.echo(f"Cookies saved to {env_path}")


@main.command()
@click.argument("path", default="0")
def ls(path):
    """List files in 115 cloud directory. Accepts dir_id or path like /影音/电影/."""
    client = _get_115_client()
    if not client:
        return
    try:
        dir_id = _resolve_dir(client, path)
        files = client.list_files_all(dir_id=dir_id)
        for f in files:
            name = f.get("fn", f.get("n", "?"))
            is_dir = "cid" in f or (not f.get("sha") and not f.get("pc"))
            size = f.get("s", 0)
            type_mark = "D" if is_dir else "F"
            click.echo(f"  [{type_mark}] {name:40s}  {size:>12,}")
        click.echo(f"\n  Total: {len(files)} items")
    except Exception as e:
        click.echo(f"Error: {e}", err=True)


VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v"}
NFO_EXT = ".nfo"


@main.command()
@click.argument("path")
@click.option("--recursive/--no-recursive", default=True, help="Scan subdirectories")
def scan(path, recursive):
    """Scan a 115 cloud folder and output a scraping plan (dry-run).

    PATH can be a dir_id or a path like /影音/电影/.
    """
    from media115.scraper.analyzer import (
        analyze_filename,
        load_cases,
        match_against_cases,
    )

    client = _get_115_client()
    if not client:
        return

    try:
        dir_id = _resolve_dir(client, path)
        click.echo(f"Scanning dir_id={dir_id} ...")
    except Exception as e:
        click.echo(f"Error resolving path: {e}", err=True)
        return

    # List files
    if recursive:
        items = client.list_files_recursive(dir_id=dir_id)
    else:
        items = client.list_files_all(dir_id=dir_id)

    # Separate videos and NFOs
    videos = []
    nfo_set: set[str] = set()  # parent_id + basename (without ext) that have .nfo
    for item in items:
        name = item.get("fn", item.get("n", ""))
        is_dir = item.get("_is_dir", "cid" in item)
        if is_dir:
            continue
        stem, ext = _split_ext(name)
        parent = str(item.get("_parent_id", item.get("cid", "")))
        if ext.lower() == NFO_EXT:
            nfo_set.add(f"{parent}/{stem}")
        elif ext.lower() in VIDEO_EXTS:
            videos.append(item)

    # Load regression cases
    cases_path = (
        Path(__file__).resolve().parent.parent.parent / "tests" / "scrape_cases.json"
    )
    cases = load_cases(cases_path) if cases_path.exists() else []

    # Analyze each video
    click.echo(f"\nFound {len(videos)} video files.\n")
    click.echo(
        f"| {'#':>3} | {'File':<50} | {'NFO':^5} | {'Type':<6} | {'Title':<30} | {'Source':<8} | {'Action':<12} |"
    )
    click.echo(
        f"|{'-' * 5}|{'-' * 52}|{'-' * 7}|{'-' * 8}|{'-' * 32}|{'-' * 10}|{'-' * 14}|"
    )

    for i, item in enumerate(videos, 1):
        name = item.get("fn", item.get("n", ""))
        parent = str(item.get("_parent_id", ""))
        stem, _ = _split_ext(name)
        has_nfo = f"{parent}/{stem}" in nfo_set

        # Check regression cases first
        case_result = match_against_cases(name, cases)
        if case_result:
            nfo_mark = "\u2705" if has_nfo else "\u274c"
            click.echo(
                f"| {i:>3} | {_trunc(name, 50):<50} | {nfo_mark:^5} | {case_result.media_type:<6} "
                f"| {_trunc(case_result.title, 30):<30} | {case_result.source:<8} | {'known case':<12} |"
            )
            continue

        # Heuristic analysis
        result = analyze_filename(name)
        nfo_mark = "\u2705" if has_nfo else "\u274c"

        if has_nfo:
            action = "skip"
        elif result.media_type == "unknown":
            action = "unrecognized"
        else:
            action = "scrape"

        click.echo(
            f"| {i:>3} | {_trunc(name, 50):<50} | {nfo_mark:^5} | {result.media_type:<6} "
            f"| {_trunc(result.title, 30):<30} | {result.source:<8} | {action:<12} |"
        )

    click.echo("\nDry-run complete. No files were modified.")


@main.command()
@click.option("--host", default="0.0.0.0")
@click.option("--port", default=9000, type=int)
def serve(host, port):
    """Start the strm-proxy server."""
    import uvicorn
    from media115.proxy import create_app

    config = _load_config()
    jellyfin_url = config.get("jellyfin_url", "http://localhost:8096")
    client = _get_115_client()
    if not client:
        click.echo("Error: 115 credentials required for proxy", err=True)
        return

    app = create_app(jellyfin_url=jellyfin_url, cloud115_client=client)
    click.echo(f"Starting strm-proxy on {host}:{port}")
    click.echo(f"Jellyfin upstream: {jellyfin_url}")
    uvicorn.run(app, host=host, port=port)


@main.command()
@click.argument("file_path", type=click.Path(exists=True))
@click.option("--remote-dir", default="0", help="115 remote directory ID")
def upload(file_path, remote_dir):
    """Upload a file to 115 via rapid upload."""
    from media115.organizer import compute_sha1, compute_pre_sha1

    path = Path(file_path)
    click.echo(f"Computing SHA1 for {path.name}...")
    sha1 = compute_sha1(path)
    pre_sha1 = compute_pre_sha1(path)
    file_size = path.stat().st_size

    click.echo(f"  SHA1: {sha1}")
    click.echo(f"  Pre-SHA1: {pre_sha1}")
    click.echo(f"  Size: {file_size:,} bytes")

    client = _get_115_client()
    if not client:
        return

    result = client.rapid_upload(
        dir_id=remote_dir,
        filename=path.name,
        file_size=file_size,
        sha1=sha1,
        pre_sha1=pre_sha1,
    )
    if result.get("status") == 2:
        click.echo(f"  Rapid upload success! pick_code={result['data']['pick_code']}")
    else:
        click.echo(f"  Rapid upload failed (status={result.get('status')})")


@main.command()
@click.argument("query")
@click.option(
    "--source", default="tmdb", type=click.Choice(["tmdb", "bangumi", "javbus"])
)
@click.option("--language", default="zh-CN")
def scrape(query, source, language):
    """Search metadata from a source."""
    if source == "tmdb":
        from media115.scraper.tmdb import TMDBClient

        token = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
        if not token:
            click.echo("Error: TMDB_READ_ACCESS_TOKEN not set", err=True)
            return
        client = TMDBClient(read_access_token=token, language=language)
        results = client.search_movie(query)
        if not results:
            results = client.search_tv(query)
        for r in results[:5]:
            title = r.get("title", r.get("name", "?"))
            year = (r.get("release_date", r.get("first_air_date", "")) or "")[:4]
            rid = r.get("id")
            click.echo(f"  [{rid}] {title} ({year})")

    elif source == "bangumi":
        from media115.scraper.bangumi import BangumiClient

        token = os.environ.get("BANGUMI_ACCESS_TOKEN")
        client = BangumiClient(access_token=token)
        results = client.search(query)
        for r in results[:5]:
            name = r.get("name_cn") or r.get("name", "?")
            rid = r.get("id")
            click.echo(f"  [{rid}] {name}")

    elif source == "javbus":
        click.echo(f"JavBus search: {query}")
        click.echo("  (network access to javbus.com required)")


def _get_115_client():
    from media115.client import Cloud115Client

    # Try cookie mode first (from .env)
    cookies = os.environ.get("CLOUD_115_COOKIES", "")
    if cookies:
        return Cloud115Client.from_cookies(cookies)

    # Fall back to OpenAPI mode
    app_id = os.environ.get("CLOUD_115_APP_ID", "")
    if app_id:
        return Cloud115Client.from_openapi(
            app_id=app_id,
            app_secret=os.environ.get("CLOUD_115_APP_SECRET", ""),
            access_token=os.environ.get("CLOUD_115_ACCESS_TOKEN", ""),
            refresh_token=os.environ.get("CLOUD_115_REFRESH_TOKEN", ""),
        )

    click.echo(
        "Warning: No 115 credentials. Use '115-media auth' to login or set CLOUD_115_COOKIES.",
        err=True,
    )
    return None


def _resolve_dir(client, path: str) -> str:
    """Resolve path or dir_id to a dir_id."""
    if path.isdigit():
        return path
    return client.resolve_path(path)


def _split_ext(filename: str) -> tuple[str, str]:
    """Split filename into stem and extension."""
    dot = filename.rfind(".")
    if dot == -1:
        return filename, ""
    return filename[:dot], filename[dot:]


def _trunc(s: str, maxlen: int) -> str:
    """Truncate string with ellipsis."""
    if len(s) <= maxlen:
        return s
    return s[: maxlen - 2] + ".."


if __name__ == "__main__":
    main()

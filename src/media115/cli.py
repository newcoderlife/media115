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
def auth():
    """Authenticate with 115 OpenAPI (OAuth flow)."""
    app_id = os.environ.get("CLOUD_115_APP_ID", "")
    if not app_id:
        click.echo("Error: CLOUD_115_APP_ID not set in .env", err=True)
        return
    click.echo(f"App ID: {app_id}")
    click.echo("OAuth flow not yet implemented (waiting for API approval)")


@main.command()
@click.argument("dir_id", default="0")
def ls(dir_id):
    """List files in 115 cloud directory by dir_id."""
    client = _get_115_client()
    if not client:
        return
    try:
        files = client.list_files(dir_id=dir_id)
        for f in files:
            name = f.get("fn", f.get("n", "?"))
            size = f.get("s", 0)
            click.echo(f"  {name:40s}  {size:>12,}")
    except Exception as e:
        click.echo(f"Error: {e}", err=True)


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

    app_id = os.environ.get("CLOUD_115_APP_ID", "")
    if not app_id:
        click.echo("Warning: CLOUD_115_APP_ID not set, 115 features disabled", err=True)
        return None
    return Cloud115Client(
        app_id=app_id,
        app_secret=os.environ.get("CLOUD_115_APP_SECRET", ""),
        access_token=os.environ.get("CLOUD_115_ACCESS_TOKEN", ""),
        refresh_token=os.environ.get("CLOUD_115_REFRESH_TOKEN", ""),
    )


if __name__ == "__main__":
    main()

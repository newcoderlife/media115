"""CLI entry point for 115-media."""

import os
from pathlib import Path

from media115.client import WEB_API

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
@click.option("--check", is_flag=True, help="Only check if already logged in")
@click.option("--renew", is_flag=True, help="Auto-renew cookies without scanning")
@click.option("--force", is_flag=True, help="Force re-login even if already logged in")
def auth(app, check, renew, force):
    """Login to 115 via QR code scan. Saves cookies to .env."""
    from media115.client import Cloud115Client

    existing = _get_115_client()

    if check:
        if existing and existing.check_login():
            click.echo("Already logged in to 115.")
        else:
            click.echo("Not logged in to 115.")
        return

    if renew:
        if not existing:
            click.echo(
                "No existing cookies to renew. Run '115-media auth' first.", err=True
            )
            return
        if existing.renew_cookies(app=app):
            env_path = Path.cwd() / ".env"
            existing.save_cookies_to_env(env_path)
            click.echo("Cookies renewed and saved.")
        else:
            click.echo(
                "Cookie renewal failed. Run '115-media auth' to re-login.", err=True
            )
        return

    if not force and existing and existing.check_login():
        click.echo("Already logged in to 115.")
        return

    client = Cloud115Client.qr_login(app=app)
    env_path = Path.cwd() / ".env"
    client.save_cookies_to_env(env_path)
    click.echo(f"Cookies saved to {env_path}")


@main.command()
@click.argument("path", default="0")
@click.option("--tree", is_flag=True, help="Show directory tree recursively")
@click.option("--depth", default=3, type=int, help="Max depth for tree view")
def ls(path, tree, depth):
    """List files in 115 cloud directory. Accepts dir_id or path like /影音/电影/."""
    client = _get_115_client()
    if not client:
        return
    try:
        dir_id = _resolve_dir(client, path)
        if tree:
            _print_tree(client, dir_id, path, depth=depth)
        else:
            files = client.list_files_all(dir_id=dir_id)
            for f in files:
                name = f.get("fn", f.get("n", "?"))
                is_dir = "fid" not in f
                size = f.get("s", 0)
                type_mark = "D" if is_dir else "F"
                click.echo(f"  [{type_mark}] {name:40s}  {size:>12,}")
            click.echo(f"\n  Total: {len(files)} items")
    except Exception as e:
        click.echo(f"Error: {e}", err=True)


VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v"}
NFO_EXT = ".nfo"
TREE_CACHE = "tree_cache.txt"


@main.command("export-tree")
@click.argument("dir_id", default="3395528155081997321")
def export_tree(dir_id):
    """Export 115 directory tree to local cache file. Only needs 2-3 API calls."""
    import httpx as _httpx

    client = _get_115_client()
    if not client:
        return

    click.echo("Exporting directory tree from 115...")

    # Step 1: Start export
    resp = client._cookie_request(
        "POST",
        f"{WEB_API}/files/export_dir",
        data={"file_ids": dir_id, "target": "U_1_0"},
    )
    export_id = resp.get("data", {}).get("export_id")
    if not export_id:
        click.echo(f"Export failed: {resp}", err=True)
        return
    click.echo(f"Export started (id={export_id}), waiting...")

    # Step 2: Poll status
    import time as _time

    pick_code = None
    for _ in range(60):
        _time.sleep(3)
        status = client._cookie_request(
            "GET",
            f"{WEB_API}/files/export_dir",
            params={"export_id": export_id},
        )
        pc = status.get("data", {}).get("pick_code")
        if pc:
            pick_code = pc
            break
        click.echo("  Still exporting...")

    if not pick_code:
        click.echo("Export timed out.", err=True)
        return

    # Step 3: Download the tree file
    cookies = client._cookies
    cookie_dict = {}
    for part in cookies.split(";"):
        p = part.strip()
        if "=" in p:
            k, v = p.split("=", 1)
            cookie_dict[k.strip()] = v.strip()

    dl = _httpx.get(
        "https://115.com/",
        params={"ct": "download", "ac": "video", "pickcode": pick_code},
        cookies=cookie_dict,
        headers={"User-Agent": client._USER_AGENT},
        follow_redirects=True,
        timeout=30,
    )
    if dl.status_code != 200 or len(dl.content) < 100:
        click.echo(f"Download failed: {dl.status_code}", err=True)
        return

    # Decode UTF-16 and save as UTF-8
    text = dl.content.decode("utf-16-le", errors="replace")
    tree_path = Path.cwd() / TREE_CACHE
    tree_path.write_text(text)

    lines = text.strip().split("\n")
    video_count = sum(
        1
        for ln in lines
        if any(ln.rstrip().lower().endswith(ext) for ext in VIDEO_EXTS)
    )
    click.echo(f"Tree exported: {len(lines)} entries, {video_count} video files")
    click.echo(f"Saved to {tree_path}")


def _parse_tree_cache() -> list[dict]:
    """Parse tree_cache.txt into a list of file entries for scan."""
    tree_path = Path.cwd() / TREE_CACHE
    if not tree_path.exists():
        return []

    text = tree_path.read_text()
    entries = []
    path_stack: list[str] = []

    for line in text.strip().split("\n"):
        stripped = line.rstrip()
        if "|-" not in stripped:
            continue

        # Calculate depth by counting "| " prefixes
        depth = stripped.count("| ")
        name = stripped.split("|-", 1)[1].strip() if "|-" in stripped else ""
        if not name:
            continue

        # Maintain path stack
        while len(path_stack) >= depth:
            path_stack.pop() if path_stack else None
        path_stack.append(name)

        full_path = "/".join(path_stack)
        stem, ext = _split_ext(name)
        is_video = ext.lower() in VIDEO_EXTS
        is_nfo = ext.lower() == NFO_EXT

        if is_video or is_nfo:
            parent = "/".join(path_stack[:-1]) if len(path_stack) > 1 else ""
            entries.append(
                {
                    "n": name,
                    "path": full_path,
                    "parent": parent,
                    "is_video": is_video,
                    "is_nfo": is_nfo,
                }
            )

    return entries


@main.command("scan-tree")
@click.argument("category", default="")
def scan_tree(category):
    """Scan media files from cached directory tree (no API calls).

    Run 'export-tree' first. Then: scan-tree [AV|电影|剧目|里番|写真]
    """
    from media115.scraper.analyzer import (
        analyze_filename,
        load_cases,
        match_against_cases,
    )

    tree_path = Path.cwd() / TREE_CACHE
    if not tree_path.exists():
        click.echo("No tree cache. Run '115-media export-tree' first.", err=True)
        return

    entries = _parse_tree_cache()
    videos = [e for e in entries if e["is_video"]]
    nfo_set = {
        e["parent"] + "/" + _split_ext(e["n"])[0] for e in entries if e["is_nfo"]
    }

    if category:
        videos = [
            v
            for v in videos
            if v["path"].startswith(category) or f"/{category}/" in v["path"]
        ]

    cases_path = Path.cwd() / "tests" / "scrape_cases.json"
    cases = load_cases(cases_path) if cases_path.exists() else []

    click.echo(f"Found {len(videos)} video files (from cached tree).\n")
    click.echo(
        f"| {'#':>3} | {'File':<55} | {'NFO':^5} | {'Type':<8} | {'Title':<30} | {'Source':<8} | {'Action':<12} |"
    )
    click.echo(
        f"|{'-' * 5}|{'-' * 57}|{'-' * 7}|{'-' * 10}|{'-' * 32}|{'-' * 10}|{'-' * 14}|"
    )

    stats = {"skip": 0, "scrape": 0, "unrecognized": 0, "known": 0}

    for i, item in enumerate(videos, 1):
        name = item["n"]
        parent = item["parent"]
        stem = _split_ext(name)[0]
        has_nfo = f"{parent}/{stem}" in nfo_set

        case_result = match_against_cases(name, cases)
        if case_result:
            nfo_mark = "\u2705" if has_nfo else "\u274c"
            click.echo(
                f"| {i:>3} | {_trunc(name, 55):<55} | {nfo_mark:^5} | {case_result.media_type:<8} "
                f"| {_trunc(case_result.title, 30):<30} | {case_result.source:<8} | {'known case':<12} |"
            )
            stats["known"] += 1
            continue

        result = analyze_filename(name)
        nfo_mark = "\u2705" if has_nfo else "\u274c"

        if has_nfo:
            action = "skip"
            stats["skip"] += 1
        elif result.media_type == "unknown":
            action = "unrecognized"
            stats["unrecognized"] += 1
        else:
            action = "scrape"
            stats["scrape"] += 1

        click.echo(
            f"| {i:>3} | {_trunc(name, 55):<55} | {nfo_mark:^5} | {result.media_type:<8} "
            f"| {_trunc(result.title, 30):<30} | {result.source:<8} | {action:<12} |"
        )

    click.echo(
        f"\nSummary: {len(videos)} files — {stats['scrape']} to scrape, "
        f"{stats['skip']} skip (has NFO), {stats['unrecognized']} unrecognized, "
        f"{stats['known']} known cases"
    )
    click.echo("No API calls were made (tree cache only).")


@main.command("batch-scrape")
@click.argument("category")
@click.option(
    "--output", default="scrape_output", help="Output directory for NFO + posters"
)
@click.option(
    "--limit", "max_count", default=0, type=int, help="Max files to scrape (0=all)"
)
def batch_scrape(category, output, max_count):
    """Scrape metadata for a category from tree cache. Generates NFO + poster locally.

    CATEGORY: AV, 电影, or 剧目
    """
    from media115.scraper.analyzer import analyze_filename

    entries = _parse_tree_cache()
    videos = [e for e in entries if e["is_video"]]
    nfo_names = {
        e["parent"] + "/" + _split_ext(e["n"])[0] for e in entries if e["is_nfo"]
    }

    videos = [v for v in videos if category in v["path"]]
    # Skip files that already have NFO
    videos = [
        v for v in videos if v["parent"] + "/" + _split_ext(v["n"])[0] not in nfo_names
    ]

    if max_count > 0:
        videos = videos[:max_count]

    if not videos:
        click.echo(f"No files to scrape in category '{category}'.")
        return

    out_dir = Path.cwd() / output / category
    out_dir.mkdir(parents=True, exist_ok=True)

    click.echo(f"Scraping {len(videos)} files in '{category}' → {out_dir}")

    results = []
    for i, item in enumerate(videos, 1):
        name = item["n"]
        analysis = analyze_filename(name)
        parent = item.get("parent", "")
        file_out = out_dir / parent.replace("/", "_")
        file_out.mkdir(parents=True, exist_ok=True)

        click.echo(f"  [{i}/{len(videos)}] {_trunc(name, 60)} ...", nl=False)

        try:
            if analysis.media_type == "av":
                result = _scrape_av(analysis.title, name, file_out)
            elif analysis.media_type in ("movie", "unknown"):
                result = _scrape_movie(analysis.title, analysis.year, name, file_out)
            elif analysis.media_type == "tv":
                result = _scrape_tv(
                    analysis.title, analysis.season, analysis.episode, name, file_out
                )
            else:
                result = {"status": "skip", "reason": analysis.media_type}

            results.append({"file": name, **result})
            click.echo(f" {result.get('status', '?')}")
        except Exception as e:
            results.append({"file": name, "status": "error", "error": str(e)})
            click.echo(f" error: {e}")

    # Summary
    ok = sum(1 for r in results if r["status"] == "ok")
    fail = sum(1 for r in results if r["status"] in ("not_found", "error"))
    skip = sum(1 for r in results if r["status"] == "skip")
    click.echo(f"\nDone: {ok} scraped, {fail} failed, {skip} skipped")


def _scrape_movie(title: str, year: int | None, filename: str, out_dir: Path) -> dict:
    from media115.scraper.tmdb import TMDBClient
    from media115.scraper.nfo import generate_movie_nfo
    from media115.scraper.artwork import save_poster

    token = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
    if not token:
        return {"status": "error", "error": "TMDB_READ_ACCESS_TOKEN not set"}

    client = TMDBClient(read_access_token=token)
    query = title
    results = client.search_movie(query)
    if not results and year:
        results = client.search_movie(query.split(".")[0])

    if not results:
        return {"status": "not_found", "query": query}

    # Pick best match (first result, or match year)
    match = results[0]
    if year:
        for r in results:
            r_year = (r.get("release_date", "") or "")[:4]
            if r_year == str(year):
                match = r
                break

    detail = client.movie_detail(match["id"])
    images = client.movie_images(match["id"])

    stem = _split_ext(filename)[0]
    metadata = {
        "title": detail.get("title", ""),
        "originaltitle": detail.get("original_title", ""),
        "year": int((detail.get("release_date", "") or "0000")[:4]),
        "plot": detail.get("overview", ""),
        "runtime": detail.get("runtime"),
        "rating": detail.get("vote_average"),
        "premiered": detail.get("release_date", ""),
        "genres": [g["name"] for g in detail.get("genres", [])],
        "directors": [],
        "actors": [],
        "uniqueids": {"tmdb": str(detail["id"])},
    }
    imdb_id = detail.get("imdb_id")
    if imdb_id:
        metadata["uniqueids"]["imdb"] = imdb_id

    generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")

    if images.get("posters"):
        try:
            save_poster(
                images["posters"][0]["file_path"], out_dir, filename="poster.jpg"
            )
        except Exception:
            pass

    client.close()
    return {"status": "ok", "match": detail.get("title", ""), "tmdb_id": detail["id"]}


def _scrape_tv(
    title: str, season: int | None, episode: int | None, filename: str, out_dir: Path
) -> dict:
    from media115.scraper.tmdb import TMDBClient
    from media115.scraper.nfo import generate_episode_nfo

    token = os.environ.get("TMDB_READ_ACCESS_TOKEN", "")
    if not token:
        return {"status": "error", "error": "TMDB_READ_ACCESS_TOKEN not set"}

    client = TMDBClient(read_access_token=token)
    # Clean title: remove brackets and dots
    import re

    clean = re.sub(r"^\[.*?\]\s*", "", title).replace(".", " ").strip()
    results = client.search_tv(clean)

    if not results:
        # Try first part only
        results = client.search_tv(clean.split()[0] if clean else title)

    if not results:
        client.close()
        return {"status": "not_found", "query": clean}

    match = results[0]
    stem = _split_ext(filename)[0]

    metadata = {
        "title": match.get("name", ""),
        "season": season,
        "episode": episode,
        "aired": match.get("first_air_date", ""),
        "uniqueids": {"tmdb": str(match["id"])},
    }

    generate_episode_nfo(metadata, out_dir / f"{stem}.nfo")
    client.close()
    return {"status": "ok", "match": match.get("name", ""), "tmdb_id": match["id"]}


def _scrape_av(number: str, filename: str, out_dir: Path) -> dict:
    from media115.scraper.jav321 import fetch_metadata as jav321_fetch
    from media115.scraper.javfree import fetch_metadata as javfree_fetch
    from media115.scraper.nfo import generate_movie_nfo
    from media115.scraper.artwork import download_image

    # Try jav321 → javfree → javbus fallback chain
    meta = jav321_fetch(number)
    source = "jav321"
    if not meta or not meta.get("title"):
        meta = javfree_fetch(number)
        source = "javfree"
    if not meta or not meta.get("title"):
        return {"status": "not_found", "number": number}

    stem = _split_ext(filename)[0]
    metadata = {
        "title": meta.get("title", ""),
        "originaltitle": meta.get("title", ""),
        "year": int(meta["release_date"][:4]) if meta.get("release_date") else None,
        "plot": "",
        "runtime": int(meta["runtime"]) if meta.get("runtime") else None,
        "genres": meta.get("genres", []),
        "directors": [meta["director"]] if meta.get("director") else [],
        "actors": [{"name": a} for a in meta.get("actors", [])],
        "uniqueids": {source: meta.get("number", number)},
        "studio": meta.get("studio", ""),
    }

    generate_movie_nfo(metadata, out_dir / f"{stem}.nfo")

    if meta.get("cover_url"):
        try:
            download_image(meta["cover_url"], out_dir / "poster.jpg")
        except Exception:
            pass

    return {"status": "ok", "match": meta.get("title", ""), "number": number}


@main.command()
@click.argument("path")
@click.option("--recursive/--no-recursive", default=True, help="Scan subdirectories")
@click.option("--depth", default=3, type=int, help="Max recursion depth")
def scan(path, recursive, depth):
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
        items = client.list_files_recursive(dir_id=dir_id, max_depth=depth)
    else:
        items = client.list_files_all(dir_id=dir_id)

    # Separate videos and NFOs
    videos = []
    nfo_set: set[str] = set()  # parent_id + basename (without ext) that have .nfo
    for item in items:
        name = item.get("fn", item.get("n", ""))
        is_dir = item.get("_is_dir", "fid" not in item)
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
        if not results:
            click.echo(f"  No results found for '{query}'")
            return
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
        if not results:
            click.echo(f"  No results found for '{query}'")
            return
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


def _print_tree(client, dir_id: str, label: str, depth: int, prefix: str = ""):
    """Print directory tree recursively."""
    if depth < 0:
        return
    items = client.list_files_all(dir_id=dir_id)
    dirs = []
    files = []
    for item in items:
        is_dir = "fid" not in item
        if is_dir:
            dirs.append(item)
        else:
            files.append(item)

    entries = dirs + files
    for i, item in enumerate(entries):
        name = item.get("fn", item.get("n", "?"))
        is_last = i == len(entries) - 1
        connector = "\u2514\u2500\u2500 " if is_last else "\u251c\u2500\u2500 "
        is_dir = item in dirs
        mark = "\U0001f4c1" if is_dir else "\U0001f4c4"
        click.echo(f"{prefix}{connector}{mark} {name}")

        if is_dir and depth > 0:
            child_id = str(item.get("cid", item.get("fid", "")))
            child_prefix = prefix + ("    " if is_last else "\u2502   ")
            _print_tree(client, child_id, name, depth - 1, child_prefix)


if __name__ == "__main__":
    main()

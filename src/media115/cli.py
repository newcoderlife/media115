"""CLI entry point for media115."""

from __future__ import annotations

import os
from pathlib import Path

import click
import yaml

from media115 import cache as media_cache
from media115.utils import split_ext as _split_ext
from media115.utils import trunc as _trunc


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


_DEFAULT_CONFIG = {
    "root": "/影音",
    "categories": {
        "电影": {
            "type": "movie",
            "naming": "{title} ({year})",
            "sources": ["tmdb"],
        },
        "剧目": {
            "type": "tv",
            "naming": "{title} ({year})",
            "episode_naming": "{title} S{season:02d}E{episode:02d}",
            "sources": ["tmdb", "bangumi"],
        },
        "AV": {
            "type": "av",
            "naming": "{number}",
            "sources": ["jav321", "javfree"],
        },
    },
    "rate_limit": {
        "qps": 0.5,
        "qpm": 20,
        "cooldown_seconds": 3600,
    },
    "jellyfin_url": "http://localhost:8096",
    "strm_proxy": {
        "host": "localhost",
        "port": 9000,
    },
}


def _load_config() -> dict:
    """Load config.yaml from project root, merged with defaults.

    Looks for config.yaml in the current working directory.
    Missing keys fall back to _DEFAULT_CONFIG.
    """
    config_path = Path.cwd() / "config.yaml"
    if config_path.exists():
        user_config = yaml.safe_load(config_path.read_text()) or {}
        # Shallow merge: top-level keys from user override defaults
        merged = dict(_DEFAULT_CONFIG)
        for key, val in user_config.items():
            if isinstance(val, dict) and isinstance(merged.get(key), dict):
                merged[key] = {**merged[key], **val}
            else:
                merged[key] = val
        return merged
    return dict(_DEFAULT_CONFIG)


@click.group()
def main():
    """media115: Media library management with 115 cloud."""
    _load_env()


@main.command()
@click.option("--app", default="tv", help="Device type for QR login (tv/qandroid/web)")
@click.option("--check", is_flag=True, help="Only check if already logged in")
@click.option("--renew", is_flag=True, help="Auto-renew cookies without scanning")
@click.option("--force", is_flag=True, help="Force re-login even if already logged in")
@click.option("--get-qr", is_flag=True, help="Generate QR URL and exit (non-blocking)")
@click.option("--wait-qr", is_flag=True, help="Wait for QR scan to complete and save cookies")
def auth(app, check, renew, force, get_qr, wait_qr):
    """Login to 115 via QR code scan. Saves cookies to .env."""
    import json as _json

    from media115.client import QR_API, Cloud115Client

    existing = _get_115_client()

    if check:
        if existing and existing.check_login():
            click.echo("Already logged in to 115.")
        else:
            click.echo("Not logged in to 115.")
        return

    if get_qr:
        # Non-blocking: generate QR and save session, then exit
        import httpx as _httpx

        resp = _httpx.get(f"{QR_API}/api/1.0/{app}/1.0/token/")
        token_data = resp.json()["data"]
        uid = token_data["uid"]
        qr_url = f"{QR_API}/api/1.0/web/1.0/qrcode?qrfrom=1&client=0d&uid={uid}"
        # Save session for --wait-qr
        session = {
            "uid": uid,
            "time": token_data["time"],
            "sign": token_data["sign"],
            "app": app,
        }
        qr_session_path = Path.cwd() / ".cache" / "qr_session.json"
        qr_session_path.parent.mkdir(parents=True, exist_ok=True)
        qr_session_path.write_text(_json.dumps(session))
        click.echo(f"QR_URL={qr_url}")
        return

    if wait_qr:
        # Blocking: poll for scan result, save cookies
        import time as _time

        import httpx as _httpx

        qr_session_path = Path.cwd() / ".cache" / "qr_session.json"
        if not qr_session_path.exists():
            click.echo("No QR session. Run 'auth --get-qr' first.", err=True)
            return
        session = _json.loads(qr_session_path.read_text())
        uid, qr_time, sign, qr_app = (
            session["uid"],
            session["time"],
            session["sign"],
            session["app"],
        )

        click.echo("Waiting for QR scan...")
        while True:
            try:
                resp = _httpx.get(
                    f"{QR_API}/get/status/",
                    params={
                        "uid": uid,
                        "time": qr_time,
                        "sign": sign,
                        "_": int(_time.time()),
                    },
                    timeout=35,
                )
                if not resp.content:
                    _time.sleep(1)
                    continue
                result = resp.json()
            except Exception:
                _time.sleep(1)
                continue

            status = result.get("data", {}).get("status", 0)
            if status == 0:
                _time.sleep(2)
            elif status == 1:
                click.echo("Scanned, waiting for confirmation...")
                _time.sleep(1)
            elif status == 2:
                break
            elif status == -1:
                click.echo("QR expired. Run 'auth --get-qr' again.", err=True)
                return
            else:
                _time.sleep(1)

        from media115.client import PASSPORT_API

        resp = _httpx.post(
            f"{PASSPORT_API}/app/1.0/{qr_app}/1.0/login/qrcode",
            data={"account": uid, "app": qr_app},
        )
        cookies = resp.json().get("data", {}).get("cookie", {})
        if isinstance(cookies, dict):
            cookie_str = "; ".join(f"{k}={v}" for k, v in cookies.items())
        else:
            cookie_str = str(cookies)

        client = Cloud115Client.from_cookies(cookie_str)
        client.save_cookies_to_env(Path.cwd() / ".env")
        qr_session_path.unlink(missing_ok=True)
        click.echo("Login success! Cookies saved to .env")
        return

    if renew:
        if not existing:
            click.echo("No existing cookies to renew. Run 'media115 auth' first.", err=True)
            return
        if existing.renew_cookies(app=app):
            env_path = Path.cwd() / ".env"
            existing.save_cookies_to_env(env_path)
            click.echo("Cookies renewed and saved.")
        else:
            click.echo("Cookie renewal failed. Run 'media115 auth' to re-login.", err=True)
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


@main.command("export-tree")
@click.argument("path")
def export_tree(path):
    """Export 115 directory tree to local cache file. Only needs 2-3 API calls."""
    client = _get_115_client()
    if not client:
        return

    click.echo(f"Resolving {path}...")
    dir_id = _resolve_dir(client, path)

    if dir_id == "0":
        click.echo("Error: cannot export root directory. Specify a path like /影音", err=True)
        return

    click.echo(f"Exporting directory tree (dir_id={dir_id})...")
    text = client.export_tree(dir_id)

    if not text:
        click.echo("Export failed.", err=True)
        return

    tree_path = media_cache.tree_cache_path()
    tree_path.write_text(text)

    lines = text.strip().split("\n")
    video_count = sum(
        1 for ln in lines if any(ln.rstrip().lower().endswith(ext) for ext in VIDEO_EXTS)
    )
    click.echo(f"Tree exported: {len(lines)} entries, {video_count} video files")
    click.echo(f"Saved to {tree_path}")
    click.echo("\nNext: run 'scan-tree <category>' to preview what needs scraping.")


@main.command("scan-tree")
@click.argument("category", default="")
@click.option("--all", "show_all", is_flag=True, help="Show all files, not just actionable ones")
def scan_tree(category, show_all):
    """Scan media files from cached directory tree (no API calls).

    Run 'export-tree' first. Then: scan-tree [AV|电影|剧目|里番|写真]
    Default: only show files needing action. Use --all to show everything.
    """
    from media115.scraper.analyzer import (
        analyze_filename,
        load_cases,
        match_against_cases,
    )

    tree_path = media_cache.tree_cache_path()
    if not tree_path.exists():
        click.echo("No tree cache. Run 'media115 export-tree' first.", err=True)
        return

    entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    videos = [e for e in entries if e["is_video"]]
    nfo_set = {e["parent"] + "/" + _split_ext(e["n"])[0] for e in entries if e["is_nfo"]}

    if category:
        videos = [
            v for v in videos if v["path"].startswith(category) or f"/{category}/" in v["path"]
        ]

    cases_path = Path.cwd() / "tests" / "scrape_cases.json"
    cases = load_cases(cases_path) if cases_path.exists() else []

    click.echo(f"Found {len(videos)} video files (from cached tree).\n")
    click.echo(
        f"| {'#':>3} | {'File':<55} | {'NFO':^5} | {'Type':<8} | {'Title':<30} | {'Source':<8} | {'Action':<12} |"
    )
    click.echo(f"|{'-' * 5}|{'-' * 57}|{'-' * 7}|{'-' * 10}|{'-' * 32}|{'-' * 10}|{'-' * 14}|")

    stats = {"skip": 0, "scrape": 0, "unrecognized": 0, "known": 0}

    for i, item in enumerate(videos, 1):
        name = item["n"]
        parent = item["parent"]
        stem = _split_ext(name)[0]
        has_nfo = f"{parent}/{stem}" in nfo_set

        case_result = match_against_cases(name, cases)
        if case_result:
            nfo_mark = "\u2705" if has_nfo else "\u274c"
            if show_all or not has_nfo:
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

        if show_all or action != "skip":
            click.echo(
                f"| {i:>3} | {_trunc(name, 55):<55} | {nfo_mark:^5} | {result.media_type:<8} "
                f"| {_trunc(result.title, 30):<30} | {result.source:<8} | {action:<12} |"
            )

    click.echo(
        f"\nSummary: {len(videos)} files — {stats['scrape']} to scrape, "
        f"{stats['skip']} skip (has NFO), {stats['unrecognized']} unrecognized, "
        f"{stats['known']} known cases"
    )

    # --- Anomaly detection ---
    import re as _re
    from collections import Counter

    all_entries = entries  # includes both video and nfo
    if category:
        all_entries = [
            e
            for e in all_entries
            if e["path"].startswith(category) or f"/{category}/" in e["path"]
        ]

    anomalies = []

    # 1. Non-standard naming: has NFO but parent dir doesn't match convention
    std_movie_tv = _re.compile(r"^.+ \(\d{4}\)$")
    std_av = _re.compile(r"^[A-Za-z]+-\d+$")
    for v in videos:
        parent = v["parent"]
        parent_leaf = parent.split("/")[-1] if "/" in parent else parent
        stem = _split_ext(v["n"])[0]
        has_nfo = f"{parent}/{stem}" in nfo_set
        if not has_nfo:
            continue
        # Determine expected pattern from category path
        if "/AV/" in v["path"] or v["path"].startswith("AV/"):
            if not std_av.match(parent_leaf):
                anomalies.append(("naming", v["parent"], v["n"]))
        else:
            if not std_movie_tv.match(parent_leaf):
                anomalies.append(("naming", v["parent"], v["n"]))

    # 2. Duplicate NFOs: same NFO filename appears >1 time in same directory
    nfo_keys = [e["parent"] + "/" + e["n"] for e in all_entries if e["is_nfo"]]
    nfo_parents = [e["parent"] for e in all_entries if e["is_nfo"]]
    for key, count in Counter(nfo_keys).items():
        if count > 1:
            parent = key.rsplit("/", 1)[0]
            nfo_name = key.rsplit("/", 1)[1]
            anomalies.append(("dup_nfo", parent, f"{nfo_name} x{count}"))

    # 3. Residual directories: have NFO but no video file
    video_parents = {v["parent"] for v in videos}
    nfo_only_parents = set(nfo_parents) - video_parents
    for parent in sorted(nfo_only_parents):
        anomalies.append(("residual", parent, "NFO only, no video"))

    if anomalies:
        click.echo(f"\nAnomalies found: {len(anomalies)}")
        labels = {
            "naming": "Non-standard name",
            "dup_nfo": "Duplicate NFO",
            "residual": "Residual dir",
        }
        for atype, parent, detail in anomalies:
            click.echo(f"  [{labels[atype]}] {parent}")
            click.echo(f"    {detail}")

    click.echo("\nNo API calls were made (tree cache only).")


@main.command("batch-scrape")
@click.argument("category")
@click.option(
    "--output",
    default=".cache/scrape_output",
    help="Output directory for NFO + posters",
)
@click.option("--limit", "max_count", default=0, type=int, help="Max files to scrape (0=all)")
@click.option("--force", is_flag=True, help="Re-scrape even if NFO already exists on 115")
def batch_scrape(category, output, max_count, force):
    """Scrape metadata for a category from tree cache. Generates NFO + poster locally.

    CATEGORY: AV, 电影, or 剧目
    """
    from media115.scraper.analyzer import analyze_filename

    entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    videos = [e for e in entries if e["is_video"]]

    videos = [v for v in videos if category in v["path"]]

    if not force:
        nfo_names = {e["parent"] + "/" + _split_ext(e["n"])[0] for e in entries if e["is_nfo"]}
        # Skip files that already have NFO
        videos = [v for v in videos if v["parent"] + "/" + _split_ext(v["n"])[0] not in nfo_names]

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

        pct = 100 * i // len(videos)
        click.echo(f"  [{i}/{len(videos)} {pct}%] {_trunc(name, 60)} ...", nl=False)

        try:
            from media115.scraper.scrape import scrape_av, scrape_movie, scrape_tv

            if analysis.media_type == "av":
                result = scrape_av(analysis.title, name, file_out)
            elif analysis.media_type in ("movie", "unknown"):
                result = scrape_movie(analysis.title, analysis.year, name, file_out)
            elif analysis.media_type in ("tv", "anime"):
                season = analysis.season if analysis.season is not None else 1
                result = scrape_tv(analysis.title, season, analysis.episode, name, file_out)
            else:
                result = {"status": "skip", "reason": analysis.media_type}

            results.append({"file": name, **result})
            status = result.get("status", "?")
            match_info = result.get("match", "")
            source_id = result.get("tmdb_id", result.get("number", ""))
            if status == "ok" and match_info:
                click.echo(f" → {match_info} ({source_id})")
            else:
                click.echo(f" {status}")
        except Exception as e:
            results.append({"file": name, "status": "error", "error": str(e)})
            click.echo(f" error: {e}")

    # Summary
    ok = sum(1 for r in results if r["status"] == "ok")
    fail = sum(1 for r in results if r["status"] in ("not_found", "error"))
    skip = sum(1 for r in results if r["status"] == "skip")
    click.echo(f"\nDone: {ok} scraped, {fail} failed, {skip} skipped")

    # Save scrape log
    import json as _json
    import time as _time

    log_dir = Path.cwd() / ".cache" / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    log_file = log_dir / f"scrape_{category}_{int(_time.time())}.json"
    log_file.write_text(_json.dumps(results, ensure_ascii=False, indent=2))
    click.echo(f"Log saved to {log_file}")

    if ok > 0:
        click.echo(f"\nNext: run 'organize {category}' to preview rename plan.")


@main.command()
@click.argument("category")
@click.option("--execute", is_flag=True, help="Actually rename/move (default is dry-run)")
@click.option("--cleanup", is_flag=True, help="Delete empty directories after organize")
def organize(category, execute, cleanup):
    """Rename and reorganize files on 115 to Jellyfin standard.

    Dry-run by default (shows plan). Use --execute to apply.
    Reads from tree cache + scrape cache, no extra API calls for planning.

    CATEGORY: AV, 电影, or 剧目
    """
    from media115.organizer import build_organize_plan, execute_organize_plan

    tree_path = media_cache.tree_cache_path()
    if not tree_path.exists():
        click.echo("No tree cache. Run 'media115 export-tree' first.", err=True)
        return

    entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    plan = build_organize_plan(category, entries, media_cache)

    renames = [op for op in plan if op["action"] == "rename"]
    skips = [op for op in plan if op["action"] == "skip"]

    click.echo(
        f"Organize plan for '{category}': {len(renames)} to rename, {len(skips)} unchanged\n"
    )

    if not renames:
        click.echo("Nothing to rename.")
        return

    click.echo(f"| {'#':>3} | {'Current':<40} | {'→ New Name':<30} | {'Source':>10} |")
    click.echo(f"|{'-' * 5}|{'-' * 42}|{'-' * 32}|{'-' * 12}|")

    for i, op in enumerate(renames, 1):
        cur = _trunc(op["file"], 40)
        nn = _trunc(op.get("new_name") or op.get("new_folder") or "-", 30)
        sid = op.get("source_id", "")
        src = f"tmdb:{sid}" if op.get("type") in ("movie", "tv") and sid else sid
        click.echo(f"| {i:>3} | {cur:<40} | {nn:<30} | {src:>10} |")

    # Check for target conflicts
    from collections import Counter

    targets = Counter()
    for op in renames:
        key = (op.get("new_folder", ""), op.get("new_name", ""))
        if key != ("", ""):
            targets[key] += 1
    conflicts = {k: v for k, v in targets.items() if v > 1}
    if conflicts:
        click.echo(f"\n⚠ {len(conflicts)} target conflicts detected:")
        for (folder, name), count in conflicts.items():
            click.echo(f"  {count}x → {folder}/{name}")
        click.echo("Resolve conflicts before executing (e.g., delete duplicates).")
        return

    if not execute:
        click.echo(f"\nDry-run complete. Use --execute to apply {len(renames)} renames.")
        return

    client = _get_115_client()
    if not client:
        return

    click.echo(f"\nExecuting {len(renames)} renames...")
    category_path = f"影音/{category}"
    results = execute_organize_plan(renames, client, category_path)

    ok = sum(1 for r in results if r["status"] == "ok")
    fail = sum(1 for r in results if r["status"] in ("error", "not_found"))
    click.echo(f"\nDone: {ok} renamed, {fail} failed")
    for r in results:
        if r["status"] == "error":
            click.echo(f"  Error: {r['file']} — {r.get('error', '?')}")
        elif r["status"] == "not_found":
            click.echo(f"  Not found on 115: {r['file']}")

    # Save operation log
    import json as _json
    import time as _time

    log_dir = Path.cwd() / ".cache" / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    log_file = log_dir / f"organize_{category}_{int(_time.time())}.json"
    log_entries = []
    for r in results:
        log_entries.append(
            {
                "original_file": r.get("file", ""),
                "original_path": r.get("parent", ""),
                "new_folder": r.get("new_folder", ""),
                "new_name": r.get("new_name", ""),
                "status": r.get("status", ""),
                "error": r.get("error", ""),
            }
        )
    log_file.write_text(_json.dumps(log_entries, ensure_ascii=False, indent=2))
    click.echo(f"Log saved to {log_file}")

    if ok > 0:
        click.echo(
            "\nTree cache is now stale. Run 'export-tree /影音' to refresh.",
            err=True,
        )

    if cleanup and ok > 0:
        click.echo("\nCleaning up empty directories...")
        category_cid = client.get_dir_id(f"/影音/{category}")
        if category_cid:
            items = client.list_files_all(dir_id=category_cid)
            empty_dirs = [
                item
                for item in items
                if "fid" not in item  # is directory
                and not client.list_files(dir_id=str(item.get("cid", "")), limit=1)
            ]
            for d in empty_dirs:
                name = d.get("n", "")
                cid = str(d.get("cid", ""))
                try:
                    client.delete([cid])
                    click.echo(f"  Deleted empty dir: {name}")
                except Exception as e:
                    click.echo(f"  Failed to delete {name}: {e}")
            click.echo(f"Cleaned up {len(empty_dirs)} empty directories")


@main.command("upload-nfo")
@click.argument("category")
def upload_nfo(category):
    """Upload NFO/poster from local scrape_output to 115 directories.

    Matches scrape_output dirs to 115 cloud directories and uploads
    .nfo, .jpg, .png files. Skips dirs that already have NFO files.

    CATEGORY: AV, 电影, or 剧目
    """
    import sys

    scrape_dir = Path.cwd() / ".cache" / "scrape_output" / category
    if not scrape_dir.exists():
        click.echo(f"No scrape output for '{category}'. Run batch-scrape first.", err=True)
        return

    client = _get_115_client()
    if not client:
        return

    category_path = f"影音/{category}"
    category_cid = client.get_dir_id("/" + category_path)
    if not category_cid:
        click.echo(f"Category dir not found: {category_path}", err=True)
        return

    # List all subdirs on 115 under category
    print(f"  Listing 115 dirs under {category_path}...", file=sys.stderr, flush=True)
    cloud_dirs = client.list_files_all(dir_id=category_cid)
    dir_map = {}  # dirname → cid
    for item in cloud_dirs:
        if "fid" not in item:  # is directory
            dir_map[item.get("n", "")] = str(item.get("cid", ""))

    # Check which dirs already have NFO using tree cache (0 API calls)
    tree_entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    dirs_with_nfo = set()
    for e in tree_entries:
        if e["is_nfo"] and category_path in e["parent"]:
            # Extract leaf dir name from parent path like "影音/电影/Title (Year)"
            leaf = e["parent"].split("/")[-1] if "/" in e["parent"] else e["parent"]
            dirs_with_nfo.add(leaf)

    # Collect upload tasks: (local_file, target_cid, remote_name)
    tasks = []
    skipped = 0
    already_has = 0
    for out_dir in sorted(scrape_dir.iterdir()):
        if not out_dir.is_dir():
            continue
        files_to_upload = [f for f in out_dir.iterdir() if f.suffix in (".nfo", ".jpg", ".png")]
        if not files_to_upload:
            continue

        # Match to 115 dir: scrape_output dir is named like "影音_电影_Title (Year)"
        # The 115 dir name is just "Title (Year)"
        parts = out_dir.name.split("_", 2)  # ["影音", "电影", "Title (Year)"]
        target_name = parts[-1] if len(parts) >= 3 else out_dir.name

        target_cid = dir_map.get(target_name)
        if not target_cid:
            skipped += 1
            continue

        if target_name in dirs_with_nfo:
            already_has += 1
            continue

        for f in files_to_upload:
            remote_name = f.name
            # For AV, rename NFO to match video: "番号.nfo" instead of long original name
            if category == "AV" and f.suffix == ".nfo":
                remote_name = f"{target_name}.nfo"
            tasks.append((f, target_cid, remote_name))

    click.echo(
        f"Found {len(tasks)} files to upload "
        f"({already_has} dirs already have NFO, {skipped} dirs not matched on 115)"
    )

    if not tasks:
        click.echo("Nothing to upload.")
        return

    uploaded = 0
    errors = 0
    for i, (local_file, target_cid, remote_name) in enumerate(tasks, 1):
        pct = 100 * i // len(tasks)
        print(
            f"\r  [{i}/{len(tasks)} {pct}%] {remote_name[:50]}...",
            end="",
            file=sys.stderr,
            flush=True,
        )
        try:
            client.upload_file(local_file, target_cid, remote_name)
            uploaded += 1
        except Exception as e:
            errors += 1
            print(f" error: {e}", file=sys.stderr)

    print("", file=sys.stderr)
    click.echo(f"\nDone: {uploaded} uploaded, {errors} errors, {skipped} dirs skipped")


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
    cases_path = Path(__file__).resolve().parent.parent.parent / "tests" / "scrape_cases.json"
    cases = load_cases(cases_path) if cases_path.exists() else []

    # Analyze each video
    click.echo(f"\nFound {len(videos)} video files.\n")
    click.echo(
        f"| {'#':>3} | {'File':<50} | {'NFO':^5} | {'Type':<6} | {'Title':<30} | {'Source':<8} | {'Action':<12} |"
    )
    click.echo(f"|{'-' * 5}|{'-' * 52}|{'-' * 7}|{'-' * 8}|{'-' * 32}|{'-' * 10}|{'-' * 14}|")

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
@click.option("--host", default="127.0.0.1")
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
@click.argument("path")
@click.option("--output", "-o", required=True, help="Output directory for .strm files")
@click.option("--host", default=None, help="Proxy host (default: from config.yaml)")
@click.option("--port", default=None, type=int, help="Proxy port (default: from config.yaml)")
def strm(path, output, host, port):
    """Generate .strm files for Jellyfin from the tree cache.

    PATH: 115 cloud path like /影音/电影

    Reads tree_cache.txt and generates one .strm file per video file,
    mirroring the 115 directory structure under OUTPUT. Each .strm file
    contains a redirect URL through the strm-proxy.

    Example:
        media115 strm /影音/电影 --output ./strm/
    """
    from urllib.parse import quote

    config = _load_config()
    proxy_host = host or config.get("strm_proxy", {}).get("host", "localhost")
    proxy_port = port or config.get("strm_proxy", {}).get("port", 9000)
    base_url = f"http://{proxy_host}:{proxy_port}"

    tree_path = media_cache.tree_cache_path()
    if not tree_path.exists():
        click.echo("No tree cache. Run 'media115 export-tree' first.", err=True)
        return

    # Normalize the filter path (strip leading/trailing slashes)
    filter_path = path.strip("/")

    entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    videos = [e for e in entries if e["is_video"]]

    # Filter to videos under the given path
    matched = [
        v for v in videos if v["path"].startswith(filter_path + "/") or v["path"] == filter_path
    ]

    if not matched:
        click.echo(f"No video files found under '{path}' in tree cache.")
        click.echo("Run 'media115 export-tree' to refresh the cache.")
        return

    out_dir = Path(output)
    out_dir.mkdir(parents=True, exist_ok=True)

    created = 0
    for item in matched:
        full_115_path = item["path"]
        # Path relative to the filter path, for local directory structure
        if full_115_path.startswith(filter_path + "/"):
            rel_path = full_115_path[len(filter_path) + 1 :]
        else:
            rel_path = full_115_path

        # Create .strm file mirroring directory structure
        strm_file = out_dir / (rel_path + ".strm")
        strm_file.parent.mkdir(parents=True, exist_ok=True)

        # URL-encode the full 115 path for the redirect endpoint
        encoded_path = quote(full_115_path, safe="")
        strm_url = f"{base_url}/redirect/{encoded_path}"

        strm_file.write_text(strm_url + "\n")
        created += 1

    click.echo(f"Generated {created} .strm files in {out_dir}")
    click.echo(f"Proxy URL base: {base_url}/redirect/...")
    click.echo(f"\nMake sure the strm-proxy is running: media115 serve --port {proxy_port}")


@main.command()
@click.argument("file_path", type=click.Path(exists=True))
@click.option("--remote-dir", default="0", help="115 remote directory ID")
def upload(file_path, remote_dir):
    """Upload a file to 115 (cookie mode, OSS upload)."""
    path = Path(file_path)
    file_size = path.stat().st_size
    click.echo(f"Uploading {path.name} ({file_size:,} bytes)...")

    client = _get_115_client()
    if not client:
        return

    try:
        result = client.upload_file(path, remote_dir)
        if result:
            click.echo("  Upload success!")
        else:
            click.echo("  Upload failed (no response)")
    except NotImplementedError:
        click.echo("  Error: upload requires cookie mode. Run 'auth' first.", err=True)
    except Exception as e:
        click.echo(f"  Upload failed: {e}", err=True)


@main.command()
@click.argument("query")
@click.option("--source", default="tmdb", type=click.Choice(["tmdb", "bangumi", "javbus"]))
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
        "Warning: No 115 credentials. Use 'media115 auth' to login or set CLOUD_115_COOKIES.",
        err=True,
    )
    return None


def _resolve_dir(client, path: str) -> str:
    """Resolve path or dir_id to a dir_id."""
    if path.isdigit():
        return path
    return client.resolve_path(path)


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

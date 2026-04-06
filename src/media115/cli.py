"""CLI entry point for media115."""

from __future__ import annotations

import os
import sys
from pathlib import Path

import click
import yaml

from media115 import cache as media_cache
from media115.utils import split_ext as _split_ext
from media115.utils import trunc as _trunc


def _cases_path() -> Path | None:
    """找到 scrape_cases.json 的路径。

    查找顺序：
    1. cwd/tests/scrape_cases.json（repo 内开发）
    2. ~/.cache/media115/scrape_cases.json（用户自定义规则）
    3. 包内 tests/scrape_cases.json（installed via pip install -e .）
    """
    # 1. cwd（repo 内开发）
    cwd_path = Path.cwd() / "tests" / "scrape_cases.json"
    if cwd_path.exists():
        return cwd_path
    # 2. XDG 缓存（用户自定义）
    from media115.cache import _cache_root
    xdg_path = _cache_root() / "scrape_cases.json"
    if xdg_path.exists():
        return xdg_path
    # 3. 包内 tests/（installed via pip install -e .）
    pkg_path = Path(__file__).resolve().parent.parent.parent / "tests" / "scrape_cases.json"
    if pkg_path.exists():
        return pkg_path
    return None


def _load_env():
    from media115.cache import _config_root
    paths = [Path.cwd() / ".env", _config_root() / ".env"]
    for env_path in paths:
        if not env_path.exists():
            continue
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
    """Load config.yaml, checking cwd first then XDG config dir.

    Search order:
      1. <cwd>/config.yaml
      2. ~/.config/media115/config.yaml  (or $XDG_CONFIG_HOME/media115/config.yaml)
    Missing keys fall back to _DEFAULT_CONFIG.
    """
    from media115.cache import _config_root
    paths = [Path.cwd() / "config.yaml", _config_root() / "config.yaml"]
    for config_path in paths:
        if config_path.exists():
            user_config = yaml.safe_load(config_path.read_text()) or {}
            # Shallow merge: top-level keys from user override defaults
            merged = dict(_DEFAULT_CONFIG)
            for key, val in user_config.items():
                if isinstance(val, dict) and key in merged and isinstance(merged[key], dict):
                    merged[key] = {**merged[key], **val}
                else:
                    merged[key] = val
            return merged
    return dict(_DEFAULT_CONFIG)


def _env_write_path() -> Path:
    """Determine the .env write path.

    If a .env already exists in cwd, write there (project-local workflow).
    Otherwise write to the XDG config dir so installed tools can find it.
    """
    cwd_env = Path.cwd() / ".env"
    if cwd_env.exists():
        return cwd_env
    from media115.cache import _config_root
    xdg_env = _config_root() / ".env"
    xdg_env.parent.mkdir(parents=True, exist_ok=True)
    return xdg_env


@click.group()
def main():
    """media115: Media library management with 115 cloud."""
    _load_env()
    from media115.log import setup_logging
    setup_logging()


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
        qr_session_path = media_cache._cache_dir() / "qr_session.json"
        qr_session_path.write_text(_json.dumps(session))
        click.echo(f"QR_URL={qr_url}")
        return

    if wait_qr:
        # Blocking: poll for scan result, save cookies
        import time as _time

        import httpx as _httpx

        qr_session_path = media_cache._cache_dir() / "qr_session.json"
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
        env_path = _env_write_path()
        client.save_cookies_to_env(env_path)
        qr_session_path.unlink(missing_ok=True)
        click.echo(f"Login success! Cookies saved to {env_path}")
        return

    if renew:
        if not existing:
            click.echo("No existing cookies to renew. Run 'media115 auth' first.", err=True)
            return
        if existing.renew_cookies(app=app):
            env_path = _env_write_path()
            existing.save_cookies_to_env(env_path)
            click.echo("Cookies renewed and saved.")
        else:
            click.echo("Cookie renewal failed. Run 'media115 auth' to re-login.", err=True)
        return

    if not force and existing and existing.check_login():
        click.echo("Already logged in to 115.")
        return

    client = Cloud115Client.qr_login(app=app)
    env_path = _env_write_path()
    client.save_cookies_to_env(env_path)
    click.echo(f"Cookies saved to {env_path}")


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

    # Write resolved path → dir_id into PathResolver cache
    if not path.isdigit():
        from media115.fs import PathResolver, _normalize

        resolver = PathResolver(client)
        resolver._write_path_index(_normalize(path), dir_id)

    click.echo("\nNext: run 'scan-tree <category>' to preview what needs scraping.")


@main.command("scan-tree")
@click.argument("category", default="")
@click.option("--all", "show_all", is_flag=True, help="Show all files, not just actionable ones")
def scan_tree(category, show_all):
    """Scan media files from cached directory tree (no API calls).

    Run 'media115 sync' first. Then: scan-tree [AV|电影|剧目|里番|写真]
    Default: only show files needing action. Use --all to show everything.
    """
    from media115.scraper.analyzer import (
        analyze_filename,
        load_cases,
        match_against_cases,
    )

    tree_path = media_cache.tree_cache_path()
    if not tree_path.exists():
        click.echo("No tree cache. Run 'media115 sync /影音' first.", err=True)
        return

    entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    videos = [e for e in entries if e["is_video"]]
    nfo_set = {e["parent"] + "/" + _split_ext(e["n"])[0] for e in entries if e["is_nfo"]}

    if category:
        videos = [
            v for v in videos if v["path"].startswith(category) or f"/{category}/" in v["path"]
        ]

    cases_path = _cases_path()
    cases = load_cases(cases_path) if cases_path else []

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
    default=None,
    help="Output directory for NFO + posters (default: ~/.cache/media115/scrape_output)",
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

    videos = [v for v in videos if f"/{category}/" in f"/{v['path']}/"]

    if not force:
        nfo_names = {e["parent"] + "/" + _split_ext(e["n"])[0] for e in entries if e["is_nfo"]}
        # Skip files that already have NFO
        videos = [v for v in videos if v["parent"] + "/" + _split_ext(v["n"])[0] not in nfo_names]

    if max_count > 0:
        videos = videos[:max_count]

    if not videos:
        click.echo(f"No files to scrape in category '{category}'.")
        return

    if output is None:
        out_dir = media_cache._cache_dir("scrape_output") / category
    else:
        out_dir = Path(output) / category
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

            entry = {"file": name, "_file_year": analysis.year, **result}
            # Extract match year from result if available
            match_year = result.get("year") or result.get("match_year")
            if match_year:
                entry["_match_year"] = match_year
            results.append(entry)
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

    # Summary table for agent review
    ok = sum(1 for r in results if r["status"] == "ok")
    fail = sum(1 for r in results if r["status"] in ("not_found", "error"))
    skip = sum(1 for r in results if r["status"] == "skip")

    click.echo(f"\nDone: {ok} scraped, {fail} failed, {skip} skipped")
    click.echo()

    # Output review table
    if results:
        click.echo(f"{'#':>4} | {'Status':<6} | {'File':<45} | {'Match':<30} | Note")
        click.echo(f"{'':->4}-+-{'':->6}-+-{'':->45}-+-{'':->30}-+------")
        for i, r in enumerate(results, 1):
            status = r.get("status", "?")
            fname = _trunc(r.get("file", ""), 45)
            match_name = r.get("match", "")
            source_id = r.get("tmdb_id", r.get("number", ""))
            file_year = r.get("_file_year")
            match_year = r.get("_match_year")

            if status == "ok":
                flag = "✓"
                match_str = _trunc(f"{match_name} ({source_id})", 30)
                # Check year mismatch
                if file_year and match_year and str(file_year) != str(match_year):
                    note = f"⚠ 年份不匹配: 文件={file_year} 匹配={match_year}"
                elif not match_year:
                    note = "⚠ 匹配结果无年份"
                else:
                    note = ""
            elif status == "not_found":
                flag = "✗"
                match_str = "—"
                note = f"query={r.get('query', '?')}"
            elif status == "error":
                flag = "✗"
                match_str = "—"
                note = _trunc(str(r.get("error", "")), 40)
            else:
                flag = "—"
                match_str = "—"
                note = r.get("reason", "")

            click.echo(f"{i:>4} | {flag:<6} | {fname:<45} | {match_str:<30} | {note}")

        # Highlight items needing review
        needs_review = [
            r for r in results
            if r.get("status") in ("not_found", "error")
            or (r.get("_file_year") and r.get("_match_year")
                and str(r["_file_year"]) != str(r["_match_year"]))
            or (r.get("status") == "ok" and not r.get("_match_year"))
        ]
        if needs_review:
            click.echo(f"\n⚠ {len(needs_review)} 个结果需要 agent 审查（年份不匹配或未找到）")

    # Save scrape log
    import json as _json
    import time as _time

    log_dir = media_cache._cache_dir("logs")
    log_file = log_dir / f"scrape_{category}_{int(_time.time())}.json"
    log_file.write_text(_json.dumps(results, ensure_ascii=False, indent=2))
    click.echo(f"\nLog saved to {log_file}")

    if ok > 0:
        click.echo(f"Next: run 'organize {category}' to preview rename plan.")


def _upload_missing_nfo(category: str, skipped_ops: list[dict]):
    """为已命名正确但缺 NFO 的文件补传 NFO/海报。

    检查 skip 的文件是否在 tree cache 中有 NFO，如果没有且本地有 scrape_output，就上传。
    """
    from media115.log import get_logger
    from media115.organizer import _upload_scrape_output

    logger = get_logger()

    # 从 tree cache 找到已有 NFO 的目录
    entries = media_cache.parse_tree_cache(VIDEO_EXTS)
    category_path = f"影音/{category}"
    nfo_parents = {
        e["parent"] for e in entries if e["is_nfo"] and f"/{category}/" in f"/{e['path']}/"
    }

    # 找缺 NFO 的 skip 文件
    missing = [
        op for op in skipped_ops
        if op.get("reason") == "already correct" and op["parent"] not in nfo_parents
    ]

    if not missing:
        return

    client = _get_115_client()
    if not client:
        return

    click.echo(f"\n补传 NFO: {len(missing)} 个文件缺少 NFO")

    uploaded = 0
    for i, op in enumerate(missing, 1):
        parent = op["parent"]
        parent_leaf = parent.split("/")[-1] if "/" in parent else parent

        # 解析目标 cid
        dir_cid = client.get_dir_id("/" + parent)
        if not dir_cid:
            logger.warning("  补传跳过 %s: 目录不存在", parent)
            continue

        print(f"\r  [{i}/{len(missing)}] {_trunc(parent_leaf, 50)}", end="", file=sys.stderr, flush=True)
        _upload_scrape_output(client, op, dir_cid, op.get("file"))
        uploaded += 1

    print("", file=sys.stderr)
    logger.info("补传完成: %d 个目录", uploaded)
    click.echo(f"补传完成: {uploaded} 个目录")


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
        click.echo("No tree cache. Run 'media115 sync /影音' first.", err=True)
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
        # 但可能有已命名正确却缺 NFO 的文件，检查并补传
        if execute:
            _upload_missing_nfo(category, skips)
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

    log_dir = media_cache._cache_dir("logs")
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

    # 补传已命名正确但缺 NFO 的文件
    _upload_missing_nfo(category, skips)

    if ok > 0:
        click.echo(
            "\nTree cache is now stale. Run 'media115 sync /影音' to refresh.",
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
    cases_path = _cases_path()
    cases = load_cases(cases_path) if cases_path else []

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
        click.echo("No tree cache. Run 'media115 sync /影音' first.", err=True)
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
        click.echo("Run 'media115 sync /影音' to refresh the cache.")
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

    cookies = os.environ.get("CLOUD_115_COOKIES", "")
    if cookies:
        return Cloud115Client.from_cookies(cookies)

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


from media115 import fs_cli as _fs_cli
_fs_cli.register(main)

if __name__ == "__main__":
    main()

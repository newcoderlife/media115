"""File organizer: SHA1 hashing, rapid upload, STRM generation, 115 rename."""

import hashlib
import re
from pathlib import Path

CHUNK_SIZE = 1024 * 1024  # 1MB
PRE_SHA1_SIZE = 128 * 1024 * 1024  # 128MB


def compute_sha1(file_path: Path) -> str:
    h = hashlib.sha1()
    with open(file_path, "rb") as f:
        while chunk := f.read(CHUNK_SIZE):
            h.update(chunk)
    return h.hexdigest()


def compute_pre_sha1(file_path: Path) -> str:
    file_size = file_path.stat().st_size
    if file_size <= PRE_SHA1_SIZE:
        return compute_sha1(file_path)

    h = hashlib.sha1()
    bytes_read = 0
    with open(file_path, "rb") as f:
        while bytes_read < PRE_SHA1_SIZE:
            to_read = min(CHUNK_SIZE, PRE_SHA1_SIZE - bytes_read)
            chunk = f.read(to_read)
            if not chunk:
                break
            h.update(chunk)
            bytes_read += len(chunk)
    return h.hexdigest()


def generate_strm(strm_path: Path, pick_code: str, proxy_base: str):
    url = f"{proxy_base.rstrip('/')}/play/{pick_code}"
    strm_path.write_text(url + "\n")


def process_file_to_strm(
    video_path: Path,
    cloud_client,
    remote_dir_id: str,
    proxy_base: str,
) -> dict:
    if video_path.suffix == ".strm":
        return {"success": True, "skipped": True}

    sha1 = compute_sha1(video_path)
    pre_sha1 = compute_pre_sha1(video_path)
    file_size = video_path.stat().st_size

    result = cloud_client.rapid_upload(
        dir_id=remote_dir_id,
        filename=video_path.name,
        file_size=file_size,
        sha1=sha1,
        pre_sha1=pre_sha1,
    )

    if result.get("status") != 2:
        return {
            "success": False,
            "reason": "rapid_upload_failed",
            "status": result.get("status"),
        }

    pick_code = result["data"]["pick_code"]
    strm_path = video_path.with_suffix(".strm")
    generate_strm(strm_path, pick_code, proxy_base)
    video_path.unlink()

    return {"success": True, "pick_code": pick_code, "strm_path": str(strm_path)}


# ── 115 Cloud directory organization ─────────────────────────────────


def plan_movie_rename(folder_name: str, metadata: dict | None) -> str | None:
    """Generate Jellyfin-standard folder name for a movie.

    Input:  'A.Better.Tomorrow.II.1987.BluRay.2160p...'
    Output: '英雄本色2 (1987)' (if metadata available)
    Or:     'A Better Tomorrow II (1987)' (parsed from filename)
    """
    if metadata:
        title = metadata.get("title", "")
        year = metadata.get("year")
        if title and year:
            clean = _sanitize(title)
            return f"{clean} ({year})"

    # Fallback: parse from folder name
    m = re.match(r"^(.+?)[.\s](\d{4})[.\s]", folder_name)
    if m:
        title = m.group(1).replace(".", " ").strip()
        year = m.group(2)
        return f"{title} ({year})"

    return None


def plan_tv_rename(folder_name: str, metadata: dict | None) -> str | None:
    """Generate Jellyfin-standard folder name for a TV show.

    Input:  '沧元图.The.Demon.Hunter.S01.2026...'
    Output: '沧元图 (2026)'
    """
    if metadata:
        title = metadata.get("title", "")
        year = (metadata.get("aired", "") or metadata.get("premiered", ""))[:4]
        if title and year:
            clean = _sanitize(title)
            return f"{clean} ({year})"

    # Fallback: parse
    m = re.match(r"^(.+?)[.\s]S\d{2}", folder_name, re.IGNORECASE)
    if m:
        title = m.group(1).replace(".", " ").strip()
        # Try to find year
        year_m = re.search(r"\.(\d{4})\.", folder_name)
        year = year_m.group(1) if year_m else ""
        if year:
            return f"{title} ({year})"
        return title

    return None


def plan_av_rename(folder_name: str) -> str | None:
    """Generate clean AV folder name.

    Input:  'DANDY-992.2026.2160p.DMM.WEB-DL...'
    Output: 'DANDY-992'
    """
    m = re.match(r"^([A-Z]{2,10}-\d{3,5})", folder_name, re.IGNORECASE)
    if m:
        return m.group(1).upper()
    return None


def plan_episode_rename(filename: str, show_title: str) -> str | None:
    """Generate Jellyfin-standard episode filename.

    Input:  'The.Demon.Hunter.S01E67.2026.2160p...'
    Output: '沧元图 S01E67.mkv'
    """
    m = re.search(r"(S\d{2}E\d{2,3})", filename, re.IGNORECASE)
    if m:
        ep_tag = m.group(1).upper()
        ext_m = re.search(r"(\.\w{2,4})$", filename)
        ext = ext_m.group(1) if ext_m else ".mkv"
        clean = _sanitize(show_title)
        return f"{clean} {ep_tag}{ext}"
    return None


def build_organize_plan(
    category: str, tree_entries: list[dict], cache_module
) -> list[dict]:
    """Build a rename/move plan from tree entries + cached metadata. Zero API calls.

    Returns list of operations:
    [{"file": "original.mkv", "path": "影音/AV/...", "parent": "...",
      "new_folder": "DANDY-992", "new_name": "DANDY-992.mkv", "action": "rename"}]
    """
    from media115.scraper.analyzer import analyze_filename

    ops = []
    # Group videos by parent directory
    videos = [e for e in tree_entries if e["is_video"] and category in e["path"]]

    for item in videos:
        name = item["n"]
        parent = item["parent"]
        analysis = analyze_filename(name)
        op = {
            "file": name,
            "path": item["path"],
            "parent": parent,
            "type": analysis.media_type,
            "action": "skip",
            "new_folder": None,
            "new_name": None,
            "reason": "",
        }

        if analysis.media_type == "movie":
            meta = _find_tmdb_cache(cache_module, analysis.title, analysis.year)
            new_folder = plan_movie_rename(
                parent.split("/")[-1] if "/" in parent else parent, meta
            )
            if new_folder and new_folder != parent.split("/")[-1]:
                ext_m = re.search(r"(\.\w{2,4})$", name)
                ext = ext_m.group(1) if ext_m else ".mkv"
                op["new_folder"] = new_folder
                op["new_name"] = f"{new_folder}{ext}"
                op["action"] = "rename"

        elif analysis.media_type == "av":
            new_folder = plan_av_rename(
                parent.split("/")[-1] if "/" in parent else name
            )
            if new_folder:
                ext_m = re.search(r"(\.\w{2,4})$", name)
                ext = ext_m.group(1) if ext_m else ".mkv"
                # Keep original filename for multi-part (Part1, Part2)
                op["new_folder"] = new_folder
                op["action"] = (
                    "rename"
                    if new_folder != (parent.split("/")[-1] if "/" in parent else "")
                    else "skip"
                )

        elif analysis.media_type == "tv":
            meta = _find_tmdb_tv_cache(cache_module, analysis.title)
            show_title = meta.get("title", analysis.title) if meta else analysis.title
            new_folder = plan_tv_rename(
                parent.split("/")[-1] if "/" in parent else parent, meta
            )
            new_name = plan_episode_rename(name, show_title)
            if new_folder or new_name:
                op["new_folder"] = new_folder
                op["new_name"] = new_name
                op["action"] = "rename"

        if op["action"] == "skip":
            op["reason"] = "no change needed or unrecognized"

        ops.append(op)

    return ops


def _find_tmdb_cache(cache_module, title: str, year: int | None) -> dict | None:
    """Search tmdb cache for a movie by title/year match."""
    import os

    cache_dir = Path(os.getcwd()) / ".cache" / "scrape" / "tmdb"
    if not cache_dir.exists():
        return None
    for f in cache_dir.glob("movie_*.json"):
        import json

        try:
            data = json.loads(f.read_text())
            detail = data.get("detail", {})
            if detail.get("title") and title.lower() in detail["title"].lower():
                return detail
            if (
                detail.get("original_title")
                and title.lower() in detail["original_title"].lower()
            ):
                return detail
        except Exception:
            continue
    return None


def _find_tmdb_tv_cache(cache_module, title: str) -> dict | None:
    """Search tmdb cache for a TV show by title match."""
    import os

    cache_dir = Path(os.getcwd()) / ".cache" / "scrape" / "tmdb"
    if not cache_dir.exists():
        return None
    for f in cache_dir.glob("tv_*.json"):
        import json

        try:
            data = json.loads(f.read_text())
            detail = data.get("detail", {})
            if (
                detail.get("name")
                and title.lower().replace(".", " ") in detail["name"].lower()
            ):
                return detail
        except Exception:
            continue
    return None


def execute_organize_plan(ops: list[dict], client) -> list[dict]:
    """Execute rename/move operations on 115. Uses search to find fids.

    Each file: search(filename) → fid → rename → move if needed.
    """
    results = []
    # Cache created directories: new_folder_name → cid
    dir_cache: dict[str, str] = {}

    for op in ops:
        if op["action"] == "skip":
            results.append({**op, "status": "skipped"})
            continue

        try:
            # Find the file on 115 by searching its name
            search_results = client.search(op["file"])
            fid = None
            for sr in search_results:
                if sr.get("n") == op["file"] and "fid" in sr:
                    fid = sr["fid"]
                    break

            if not fid:
                results.append({**op, "status": "not_found"})
                continue

            # Rename file if needed
            if op.get("new_name") and op["new_name"] != op["file"]:
                client.rename(fid, op["new_name"])

            # Rename parent folder if needed
            if op.get("new_folder"):
                parent_name = (
                    op["parent"].split("/")[-1] if "/" in op["parent"] else op["parent"]
                )
                if op["new_folder"] != parent_name and parent_name not in dir_cache:
                    # Find parent folder's cid
                    parent_results = client.search(parent_name)
                    for pr in parent_results:
                        if pr.get("n") == parent_name and "fid" not in pr:
                            client.rename(str(pr.get("cid", "")), op["new_folder"])
                            dir_cache[parent_name] = op["new_folder"]
                            break

            results.append({**op, "status": "ok"})
        except Exception as e:
            results.append({**op, "status": "error", "error": str(e)})

    return results


def _sanitize(name: str) -> str:
    """Remove characters not allowed in filenames."""
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()

"""Artwork (poster/fanart) download utilities."""

from __future__ import annotations

from pathlib import Path

import httpx

TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p"


def download_image(url: str, output_path: Path, timeout: int = 30, force: bool = False):
    if output_path.exists() and not force:
        return
    output_path.parent.mkdir(parents=True, exist_ok=True)
    resp = httpx.get(url, timeout=timeout)
    resp.raise_for_status()
    output_path.write_bytes(resp.content)


def save_poster(
    image_path: str, output_dir: Path, size: str = "w500", filename: str = "poster.jpg"
):
    url = f"{TMDB_IMAGE_BASE}/{size}{image_path}"
    download_image(url, output_dir / filename)

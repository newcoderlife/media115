"""Shared utility functions."""


from __future__ import annotations

import os
from pathlib import Path


def split_ext(filename: str) -> tuple[str, str]:
    """Split filename into stem and extension."""
    dot = filename.rfind(".")
    if dot == -1:
        return filename, ""
    return filename[:dot], filename[dot:]


def stem(filename: str) -> str:
    """Get filename without extension."""
    return split_ext(filename)[0]


def trunc(s: str, maxlen: int) -> str:
    """Truncate string with ellipsis."""
    if len(s) <= maxlen:
        return s
    return s[: maxlen - 2] + ".."


def load_env(env_path: Path | None = None):
    """Load .env file into os.environ (doesn't override existing)."""

    path = env_path or Path.cwd() / ".env"
    if not path.exists():
        return
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, val = line.split("=", 1)
        val = val.strip().strip("'\"")
        os.environ.setdefault(key.strip(), val)

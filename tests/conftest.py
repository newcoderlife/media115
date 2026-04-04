"""Shared test configuration. Loads .env if present."""

from pathlib import Path

from media115.utils import load_env

load_env(Path(__file__).parent.parent / ".env")

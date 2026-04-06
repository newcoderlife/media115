"""Shared test configuration. Loads .env if present."""

import pytest
from pathlib import Path

from media115.utils import load_env

load_env(Path(__file__).parent.parent / ".env")


@pytest.fixture(autouse=True)
def _isolate_cache(tmp_path, monkeypatch):
    """所有测试使用隔离的缓存目录。"""
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
    monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path / "config"))

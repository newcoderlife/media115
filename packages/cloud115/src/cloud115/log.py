"""cloud115 结构化日志。

Logger: "cloud115"
文件: ~/.cache/cloud115/logs/cloud115.log (RotatingFileHandler, 10MB, 5 备份)
控制台: stderr, INFO, 只输出 message
"""
from __future__ import annotations

import logging
import os
from logging.handlers import RotatingFileHandler
from pathlib import Path


def _log_dir() -> Path:
    base = os.environ.get("XDG_CACHE_HOME", "")
    if not base:
        base = str(Path.home() / ".cache")
    d = Path(base) / "cloud115" / "logs"
    d.mkdir(parents=True, exist_ok=True)
    return d


def setup_logging() -> logging.Logger:
    """初始化 cloud115 logger。幂等。"""
    logger = logging.getLogger("cloud115")
    if logger.handlers:
        return logger

    logger.setLevel(logging.DEBUG)

    fh = RotatingFileHandler(
        _log_dir() / "cloud115.log",
        maxBytes=10 * 1024 * 1024,
        backupCount=5,
        encoding="utf-8",
    )
    fh.setLevel(logging.DEBUG)
    fh.setFormatter(logging.Formatter(
        "%(asctime)s | %(levelname)-7s | %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    ))
    logger.addHandler(fh)

    sh = logging.StreamHandler()
    sh.setLevel(logging.INFO)
    sh.setFormatter(logging.Formatter("%(message)s"))
    logger.addHandler(sh)

    return logger


def get_logger() -> logging.Logger:
    logger = logging.getLogger("cloud115")
    if not logger.handlers:
        setup_logging()
    return logger

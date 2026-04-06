"""media115 日志配置。

文件日志（DEBUG）：~/.cache/media115/logs/media115.log
控制台日志（INFO）：stderr，只输出 message
进度输出不走 logging，继续用 print(\r..., stderr)
"""
import logging
from logging.handlers import RotatingFileHandler
from pathlib import Path


def setup_logging() -> logging.Logger:
    """初始化并返回 media115 logger。幂等，多次调用安全。"""
    logger = logging.getLogger("media115")
    if logger.handlers:
        return logger  # 已初始化

    logger.setLevel(logging.DEBUG)

    # 文件 handler：DEBUG+，带时间戳
    from media115.cache import _cache_dir
    log_dir = _cache_dir("logs")
    fh = RotatingFileHandler(
        log_dir / "media115.log",
        maxBytes=10 * 1024 * 1024,  # 10 MB
        backupCount=5,
        encoding="utf-8",
    )
    fh.setLevel(logging.DEBUG)
    fh.setFormatter(logging.Formatter(
        "%(asctime)s | %(levelname)-7s | %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    ))
    logger.addHandler(fh)

    # 控制台 handler：INFO+，只输出 message（不带时间戳，不干扰进度输出）
    sh = logging.StreamHandler()  # 默认 stderr
    sh.setLevel(logging.INFO)
    sh.setFormatter(logging.Formatter("%(message)s"))
    logger.addHandler(sh)

    return logger


def get_logger() -> logging.Logger:
    """获取 media115 logger（如果未初始化则自动初始化）。"""
    logger = logging.getLogger("media115")
    if not logger.handlers:
        setup_logging()
    return logger

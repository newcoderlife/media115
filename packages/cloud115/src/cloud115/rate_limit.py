"""QPS/QPM 限流器，状态持久化到 SQLite。"""
from __future__ import annotations

import threading
import time

from cloud115.log import get_logger


class RateLimiter:
    """限流器。状态通过 FileCache 的 rate_limit 表持久化。

    cache=None 时禁用持久化（纯进程内限流）。
    """

    def __init__(self, name, qps=0.5, qpm=20, cache=None):
        self._name = name
        self._qps = qps
        self._qpm = qpm
        self._cache = cache
        self._lock = threading.Lock()
        self.request_count = 0

    def acquire(self):
        with self._lock:
            self._wait_for_slot()
            self.request_count += 1

    def _wait_for_slot(self):
        if self._cache is None:
            return

        while True:
            now = time.time()
            state = self._cache.get_rate_limit(self._name)

            # cooldown
            cooldown_until = state.get("cooldown_until", 0)
            if now < cooldown_until:
                remaining = int(cooldown_until - now)
                mins, secs = divmod(remaining, 60)
                raise RuntimeError(
                    "Rate limit cooldown: %dm%02ds remaining" % (mins, secs)
                )

            # QPS
            last_req = state.get("last_request", 0)
            min_interval = 1.0 / self._qps
            wait = min_interval - (now - last_req)
            if wait > 0:
                get_logger().debug(
                    "THROTTLE   wait %.1fs qps=%.1f last_req=%.1fs_ago",
                    wait, self._qps, now - last_req,
                )
                time.sleep(wait)
                now = time.time()

            # QPM
            minute_start = state.get("minute_start", 0)
            minute_count = state.get("minute_count", 0)

            if now - minute_start > 60:
                minute_start = now
                minute_count = 0

            if minute_count >= self._qpm:
                wait = 60 - (now - minute_start)
                if wait > 0:
                    time.sleep(wait)
                    continue

            # record
            self._cache.set_rate_limit(
                self._name,
                last_request=now,
                minute_start=minute_start,
                minute_count=minute_count + 1,
            )
            break

    def set_cooldown(self, seconds):
        if self._cache:
            until = time.time() + seconds
            self._cache.set_rate_limit(self._name, cooldown_until=until)
            get_logger().warning(
                "COOLDOWN   %ds until=%.0f", int(seconds), until
            )

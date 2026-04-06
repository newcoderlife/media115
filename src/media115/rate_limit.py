"""Reusable rate limiter with QPS + QPM control and cross-process persistence."""

from __future__ import annotations

import json
import sys
import threading
import time
from pathlib import Path


def _read_state(state_path: Path) -> dict:
    """Read rate limit state from a dedicated state file (not .env)."""
    if not state_path.exists():
        return {}
    try:
        return json.loads(state_path.read_text())
    except (json.JSONDecodeError, ValueError):
        return {}


def _write_state(state_path: Path, state: dict):
    """Write rate limit state atomically."""
    state_path.write_text(json.dumps(state))


class RateLimiter:
    """Rate limiter with QPS + QPM control and cross-process persistence.

    State persisted to a JSON file:
    - cooldown_until: timestamp, 429 triggers a global cooldown
    - last_request: timestamp of last API request
    - minute_start: start of current minute window
    - minute_count: requests in current minute window

    Pass state_path=None to disable persistence (in-process only).
    """

    def __init__(self, qps: float = 0.5, qpm: int = 20, state_path: Path | None = None):
        self._qps = qps
        self._qpm = qpm
        self._state_path = state_path
        self._lock = threading.Lock()
        self.request_count = 0

    def acquire(self):
        with self._lock:
            self._wait_for_slot()
            self.request_count += 1

    def _wait_for_slot(self):
        if not self._state_path:
            return

        while True:
            now = time.time()
            state = _read_state(self._state_path)

            # Check cooldown (429 ban)
            cooldown_until = state.get("cooldown_until", 0)
            if now < cooldown_until:
                remaining = int(cooldown_until - now)
                mins, secs = divmod(remaining, 60)
                until_str = time.strftime("%H:%M", time.localtime(cooldown_until))
                raise RuntimeError(
                    f"Rate limit cooldown: {mins}m{secs:02d}s remaining (until {until_str})"
                )

            # Check QPS
            last_req = state.get("last_request", 0)
            min_interval = 1.0 / self._qps
            wait = min_interval - (now - last_req)
            if wait > 0:
                time.sleep(wait)
                now = time.time()

            # Check QPM
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

            # All checks passed — record this request
            state["last_request"] = now
            state["minute_start"] = minute_start
            state["minute_count"] = minute_count + 1
            _write_state(self._state_path, state)
            break

    def set_cooldown(self, seconds: float):
        """Set a global cooldown, persisted for cross-process enforcement."""
        if self._state_path:
            until = time.time() + seconds
            state = _read_state(self._state_path)
            state["cooldown_until"] = until
            _write_state(self._state_path, state)
            mins, secs = divmod(int(seconds), 60)
            hours, mins = divmod(mins, 60)
            until_str = time.strftime("%H:%M", time.localtime(until))
            dur = f"{hours}h" if hours else f"{mins}m{secs:02d}s"
            print(
                f"  429 received: entering {dur} cooldown (until {until_str})",
                file=sys.stderr,
                flush=True,
            )

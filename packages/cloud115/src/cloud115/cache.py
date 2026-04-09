"""SQLite-backed file/directory cache for cloud115.

Single-file database at ``~/.cache/cloud115/cache.db`` (WAL mode).

Tables
------
- **path_index** – maps cloud paths to cids.
- **dir_meta**  – per-directory freshness timestamp.
- **dir_entry** – individual entries inside a directory.
- **rate_limit** – per-endpoint rate-limit counters.
"""
from __future__ import annotations

import os
import sqlite3
import time
from pathlib import Path


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def _default_db_path() -> Path:
    base = os.environ.get("XDG_CACHE_HOME", "")
    if not base:
        base = str(Path.home() / ".cache")
    d = Path(base) / "cloud115"
    d.mkdir(parents=True, exist_ok=True)
    return d / "cache.db"


_RATE_LIMIT_FIELDS = frozenset({"cooldown_until", "last_request", "minute_start", "minute_count"})

_SCHEMA_VERSION = 3  # Bump when schema changes

_SCHEMA = """\
CREATE TABLE IF NOT EXISTS path_index (
    path       TEXT PRIMARY KEY,
    cid        TEXT NOT NULL,
    ts         INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dir_meta (
    cid  TEXT PRIMARY KEY,
    ts   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dir_entry (
    parent_cid TEXT NOT NULL,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    node_id    TEXT NOT NULL,
    size       INTEGER,
    pick_code  TEXT,
    PRIMARY KEY (parent_cid, node_id)
);

CREATE INDEX IF NOT EXISTS idx_entry_parent ON dir_entry(parent_cid);
CREATE INDEX IF NOT EXISTS idx_entry_node   ON dir_entry(node_id);
CREATE INDEX IF NOT EXISTS idx_entry_name   ON dir_entry(parent_cid, name);

CREATE TABLE IF NOT EXISTS rate_limit (
    name          TEXT PRIMARY KEY,
    cooldown_until REAL DEFAULT 0,
    last_request   REAL DEFAULT 0,
    minute_start   REAL DEFAULT 0,
    minute_count   INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tree_entry (
    path       TEXT NOT NULL,
    name       TEXT NOT NULL,
    parent     TEXT NOT NULL,
    is_video   INTEGER NOT NULL DEFAULT 0,
    is_nfo     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (path)
);

CREATE INDEX IF NOT EXISTS idx_tree_parent ON tree_entry(parent);

CREATE TABLE IF NOT EXISTS snapshot_meta (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);
"""


# ---------------------------------------------------------------------------
# FileCache
# ---------------------------------------------------------------------------

class FileCache:
    """Thin SQLite wrapper — every public method commits immediately."""

    def __init__(self, db_path: Path | str | None = None) -> None:
        if db_path is None:
            db_path = _default_db_path()
        self._db_path = Path(db_path)
        self._conn = sqlite3.connect(str(self._db_path), check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")

        # Schema migration: if version < _SCHEMA_VERSION, drop cache tables and recreate.
        # Cache data is expendable — it rebuilds from API calls.
        version = self._conn.execute("PRAGMA user_version").fetchone()[0]
        if version < _SCHEMA_VERSION:
            for table in ("dir_entry", "dir_meta", "path_index", "tree_entry", "snapshot_meta"):
                self._conn.execute(f"DROP TABLE IF EXISTS {table}")  # noqa: S608
            for idx in ("idx_entry_parent", "idx_entry_node", "idx_entry_name", "idx_tree_parent"):
                self._conn.execute(f"DROP INDEX IF EXISTS {idx}")  # noqa: S608
            self._conn.execute(f"PRAGMA user_version = {_SCHEMA_VERSION}")
            self._conn.commit()

        self._conn.executescript(_SCHEMA)
        self._conn.commit()

    # ---- path_index ------------------------------------------------------

    def get_path(self, path: str) -> tuple[str, int] | None:
        """Return ``(cid, ts)`` for *path*, or ``None``."""
        row = self._conn.execute(
            "SELECT cid, ts FROM path_index WHERE path = ?", (path,)
        ).fetchone()
        if row is None:
            return None
        return (row["cid"], row["ts"])

    def set_path(self, path: str, cid: str) -> None:
        """Insert or replace a path mapping with ``ts = now``."""
        self._conn.execute(
            "INSERT OR REPLACE INTO path_index (path, cid, ts) VALUES (?, ?, ?)",
            (path, cid, int(time.time())),
        )
        self._conn.commit()

    def delete_path_prefix(self, prefix: str) -> None:
        """Delete *prefix* itself **and** every path under ``prefix/``."""
        self._conn.execute(
            "DELETE FROM path_index WHERE path = ? OR path LIKE ?",
            (prefix, prefix.rstrip("/") + "/%"),
        )
        self._conn.commit()

    # ---- dir listing (row-level) -----------------------------------------

    def get_dir_ts(self, cid: str) -> int | None:
        """Return the freshness timestamp for directory *cid*, or ``None``."""
        row = self._conn.execute(
            "SELECT ts FROM dir_meta WHERE cid = ?", (cid,)
        ).fetchone()
        if row is None:
            return None
        return row["ts"]

    def set_dir_listing(self, cid: str, entries: list[dict]) -> None:
        """Atomically replace the cached listing for directory *cid*."""
        with self._conn:
            self._conn.execute(
                "DELETE FROM dir_entry WHERE parent_cid = ?", (cid,)
            )
            self._conn.executemany(
                "INSERT OR REPLACE INTO dir_entry "
                "(parent_cid, name, type, node_id, size, pick_code) "
                "VALUES (?, ?, ?, ?, ?, ?)",
                [
                    (
                        cid,
                        e["name"],
                        e["type"],
                        e["node_id"],
                        e.get("size"),
                        e.get("pick_code"),
                    )
                    for e in entries
                ],
            )
            self._conn.execute(
                "INSERT OR REPLACE INTO dir_meta (cid, ts) VALUES (?, ?)",
                (cid, int(time.time())),
            )

    def get_dir_entries(self, cid: str) -> list[dict]:
        """Return cached entries for *cid* (folders first, then by name)."""
        rows = self._conn.execute(
            "SELECT name, type, node_id, size, pick_code "
            "FROM dir_entry WHERE parent_cid = ? "
            "ORDER BY CASE WHEN type='dir' THEN 0 ELSE 1 END, name",
            (cid,),
        ).fetchall()
        return [dict(r) for r in rows]

    def find_entry(self, parent_cid: str, name: str) -> dict | None:
        """Look up a single child by *name* inside *parent_cid*.

        Returns the first match. When multiple same-name entries exist (different
        node_ids) use :meth:`find_entries` to retrieve all of them.
        """
        row = self._conn.execute(
            "SELECT name, type, node_id, size, pick_code "
            "FROM dir_entry WHERE parent_cid = ? AND name = ? LIMIT 1",
            (parent_cid, name),
        ).fetchone()
        if row is None:
            return None
        return dict(row)

    def find_entries(self, parent_cid: str, name: str) -> list[dict]:
        """Find ALL entries matching *name* inside *parent_cid*.

        Useful for dedup scenarios where the same filename has multiple node_ids.
        """
        rows = self._conn.execute(
            "SELECT name, type, node_id, size, pick_code "
            "FROM dir_entry WHERE parent_cid = ? AND name = ?",
            (parent_cid, name),
        ).fetchall()
        return [{"name": r[0], "type": r[1], "node_id": r[2], "size": r[3], "pick_code": r[4]} for r in rows]

    # ---- write-through ---------------------------------------------------

    def add_entry(self, parent_cid: str, entry: dict) -> None:
        """Upsert a single entry into the cached listing."""
        self._conn.execute(
            "INSERT OR REPLACE INTO dir_entry "
            "(parent_cid, name, type, node_id, size, pick_code) "
            "VALUES (?, ?, ?, ?, ?, ?)",
            (
                parent_cid,
                entry["name"],
                entry["type"],
                entry["node_id"],
                entry.get("size"),
                entry.get("pick_code"),
            ),
        )
        self._conn.commit()

    def remove_entry(self, parent_cid: str, name: str) -> None:
        """Delete all entries with *name* from the cached listing (backward compat).

        With the id-first PK a directory can contain multiple same-name files.
        This method removes all of them. Use :meth:`remove_entry_by_id` for
        targeted single-entry removal.
        """
        self._conn.execute(
            "DELETE FROM dir_entry WHERE parent_cid = ? AND name = ?",
            (parent_cid, name),
        )
        self._conn.commit()

    def remove_entry_by_id(self, parent_cid: str, node_id: str) -> None:
        """Delete a specific entry identified by *node_id* within *parent_cid*."""
        self._conn.execute(
            "DELETE FROM dir_entry WHERE parent_cid = ? AND node_id = ?",
            (parent_cid, node_id),
        )
        self._conn.commit()

    def move_entry(self, old_parent_cid: str, node_id: str, new_parent_cid: str) -> None:
        """Move an entry to a different parent directory (atomic UPDATE)."""
        self._conn.execute(
            "UPDATE dir_entry SET parent_cid = ? WHERE parent_cid = ? AND node_id = ?",
            (new_parent_cid, old_parent_cid, node_id),
        )
        self._conn.commit()

    def rename_entry(self, parent_cid: str, node_id: str, new_name: str) -> None:
        """Rename an entry identified by *node_id* within *parent_cid*."""
        self._conn.execute(
            "UPDATE dir_entry SET name = ? "
            "WHERE parent_cid = ? AND node_id = ?",
            (new_name, parent_cid, node_id),
        )
        self._conn.commit()

    def invalidate_dir(self, cid: str) -> None:
        """Remove **both** meta and all entries for directory *cid*."""
        with self._conn:
            self._conn.execute("DELETE FROM dir_meta  WHERE cid = ?", (cid,))
            self._conn.execute(
                "DELETE FROM dir_entry WHERE parent_cid = ?", (cid,)
            )

    def delete_dir_meta(self, cid: str) -> None:
        """Mark directory *cid* as stale (remove meta, keep entries)."""
        self._conn.execute("DELETE FROM dir_meta WHERE cid = ?", (cid,))
        self._conn.commit()

    # ---- rate_limit ------------------------------------------------------

    def get_rate_limit(self, name: str) -> dict:
        """Return rate-limit state for *name* (defaults all to 0)."""
        row = self._conn.execute(
            "SELECT cooldown_until, last_request, minute_start, minute_count "
            "FROM rate_limit WHERE name = ?",
            (name,),
        ).fetchone()
        if row is None:
            return {
                "cooldown_until": 0.0,
                "last_request": 0.0,
                "minute_start": 0.0,
                "minute_count": 0,
            }
        return {
            "cooldown_until": row["cooldown_until"],
            "last_request": row["last_request"],
            "minute_start": row["minute_start"],
            "minute_count": row["minute_count"],
        }

    def set_rate_limit(self, name: str, **kwargs: float | int) -> None:
        """Upsert rate-limit fields, merging *kwargs* with current values."""
        unknown = set(kwargs) - _RATE_LIMIT_FIELDS
        if unknown:
            raise ValueError("Unknown rate_limit fields: %s" % unknown)
        current = self.get_rate_limit(name)
        current.update(kwargs)
        self._conn.execute(
            "INSERT OR REPLACE INTO rate_limit "
            "(name, cooldown_until, last_request, minute_start, minute_count) "
            "VALUES (?, ?, ?, ?, ?)",
            (
                name,
                current["cooldown_until"],
                current["last_request"],
                current["minute_start"],
                current["minute_count"],
            ),
        )
        self._conn.commit()

    def increment_minute_count(self, name: str) -> None:
        """Atomically bump *minute_count* for *name* (creates row if needed)."""
        self._conn.execute(
            "INSERT INTO rate_limit (name, minute_count) VALUES (?, 1) "
            "ON CONFLICT(name) DO UPDATE SET minute_count = minute_count + 1",
            (name,),
        )
        self._conn.commit()

    # ---- management ------------------------------------------------------

    def clear_metadata(self) -> None:
        """Clear all cache data. Keeps rate_limit."""
        with self._conn:
            self._conn.execute("DELETE FROM path_index")
            self._conn.execute("DELETE FROM dir_meta")
            self._conn.execute("DELETE FROM dir_entry")
            self._conn.execute("DELETE FROM tree_entry")
            self._conn.execute("DELETE FROM snapshot_meta")

    def clear(self) -> None:
        """Clear everything including rate_limit."""
        with self._conn:
            for table in ("path_index", "dir_meta", "dir_entry", "rate_limit", "tree_entry", "snapshot_meta"):
                self._conn.execute(f"DELETE FROM {table}")  # noqa: S608

    def try_acquire_slot(self, name: str, now: float, min_interval: float, qpm: int) -> bool:
        """原子尝试获取限流 slot。单条 SQL 完成检查+更新。
        SQLite 写锁保证跨进程互斥。成功返回 True，失败返回 False。
        """
        self._conn.execute(
            "INSERT OR IGNORE INTO rate_limit (name) VALUES (?)", (name,)
        )
        cursor = self._conn.execute("""
            UPDATE rate_limit SET
                minute_count = CASE
                    WHEN ? - minute_start > 60 THEN 1
                    ELSE minute_count + 1
                END,
                minute_start = CASE
                    WHEN ? - minute_start > 60 THEN ?
                    ELSE minute_start
                END,
                last_request = ?
            WHERE name = ?
              AND (cooldown_until <= ? OR cooldown_until = 0)
              AND (? - last_request >= ? OR last_request = 0)
              AND (CASE WHEN ? - minute_start > 60 THEN 0 ELSE minute_count END) < ?
        """, (now, now, now, now, name, now, now, min_interval, now, qpm))
        self._conn.commit()
        return cursor.rowcount > 0

    def stale_dir_count(self, listing_ttl: int) -> int:
        """返回过期目录数。"""
        cutoff = int(time.time()) - listing_ttl
        return self._conn.execute(
            "SELECT COUNT(*) FROM dir_meta WHERE ts < ?", (cutoff,)
        ).fetchone()[0]

    # ---- tree_entry ------------------------------------------------------

    def set_tree(self, entries: list[dict], root_path: str = "", exported_at=None) -> None:
        """Replace all tree entries atomically. Each entry: {path, n, parent, is_video, is_nfo}."""
        import time as _time
        with self._conn:
            self._conn.execute("DELETE FROM tree_entry")
            self._conn.executemany(
                "INSERT OR REPLACE INTO tree_entry (path, name, parent, is_video, is_nfo) VALUES (?, ?, ?, ?, ?)",
                [
                    (e["path"], e["n"], e["parent"], int(e["is_video"]), int(e["is_nfo"]))
                    for e in entries
                ],
            )
            # Record metadata
            self._conn.execute(
                "INSERT OR REPLACE INTO snapshot_meta (key, value) VALUES (?, ?)",
                ("root_path", root_path),
            )
            self._conn.execute(
                "INSERT OR REPLACE INTO snapshot_meta (key, value) VALUES (?, ?)",
                ("exported_at", str(exported_at or int(_time.time()))),
            )
            self._conn.execute(
                "INSERT OR REPLACE INTO snapshot_meta (key, value) VALUES (?, ?)",
                ("entry_count", str(len(entries))),
            )

    def get_snapshot_meta(self) -> dict:
        """Return snapshot metadata as a key→value dict."""
        rows = self._conn.execute(
            "SELECT key, value FROM snapshot_meta"
        ).fetchall()
        return {r[0]: r[1] for r in rows}

    def get_tree_entries(self, category: str = "") -> list[dict]:
        """Get tree entries, optionally filtered by category path prefix."""
        if category:
            rows = self._conn.execute(
                "SELECT path, name, parent, is_video, is_nfo FROM tree_entry "
                "WHERE path LIKE ? OR path = ?",
                (category + "/%", category),
            ).fetchall()
        else:
            rows = self._conn.execute(
                "SELECT path, name, parent, is_video, is_nfo FROM tree_entry"
            ).fetchall()
        return [
            {
                "path": r[0],
                "n": r[1],
                "parent": r[2],
                "is_video": bool(r[3]),
                "is_nfo": bool(r[4]),
            }
            for r in rows
        ]

    def tree_stats(self) -> dict:
        """Return tree entry counts."""
        total = self._conn.execute("SELECT COUNT(*) FROM tree_entry").fetchone()[0]
        videos = self._conn.execute(
            "SELECT COUNT(*) FROM tree_entry WHERE is_video = 1"
        ).fetchone()[0]
        nfos = self._conn.execute(
            "SELECT COUNT(*) FROM tree_entry WHERE is_nfo = 1"
        ).fetchone()[0]
        return {"total": total, "videos": videos, "nfos": nfos}

    def clear_tree(self) -> None:
        """Clear tree entries only."""
        with self._conn:
            self._conn.execute("DELETE FROM tree_entry")

    def close(self) -> None:
        """Close the database connection."""
        if self._conn is not None:
            self._conn.close()
            self._conn = None

    def __enter__(self) -> FileCache:
        """Enter context manager."""
        return self

    def __exit__(self, *args) -> None:
        """Exit context manager and close the connection."""
        self.close()

    def stats(self) -> dict:
        """Return a summary of cache contents and database size."""
        path_count = self._conn.execute(
            "SELECT COUNT(*) FROM path_index"
        ).fetchone()[0]
        dir_count = self._conn.execute(
            "SELECT COUNT(*) FROM dir_meta"
        ).fetchone()[0]
        entry_count = self._conn.execute(
            "SELECT COUNT(*) FROM dir_entry"
        ).fetchone()[0]
        tree_entries = self._conn.execute(
            "SELECT COUNT(*) FROM tree_entry"
        ).fetchone()[0]
        try:
            db_size = self._db_path.stat().st_size
        except OSError:
            db_size = 0

        rate_states = {}
        for row in self._conn.execute(
            "SELECT name, cooldown_until, last_request, minute_count FROM rate_limit"
        ).fetchall():
            rate_states[row[0]] = {
                "cooldown_until": row[1],
                "last_request": row[2],
                "minute_count": row[3],
            }

        return {
            "path_count": path_count,
            "dir_count": dir_count,
            "entry_count": entry_count,
            "tree_entries": tree_entries,
            "db_size_bytes": db_size,
            "rate_limit": rate_states,
        }

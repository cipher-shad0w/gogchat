"""Disk-based cache for the gchat TUI application.

Caches API responses (spaces list, messages per space, members per space,
unread states) to allow instant UI rendering with background refresh.
Cache files are stored in a platform-aware directory and use atomic writes
to prevent corruption.
"""

import json
import logging
import os
import platform
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

_MEMBERS_TTL_SECONDS: float = 3600.0  # 1 hour


class Cache:
    """Disk-based cache for Google Chat API responses."""

    def __init__(self) -> None:
        """Determine cache directory path.

        Directories are NOT created here; they are lazily created on first write.
        """
        self._dir: Path = self._cache_dir()

    @staticmethod
    def _cache_dir() -> Path:
        """Return the platform-aware cache directory path.

        - macOS (Darwin): ~/Library/Caches/gogchat/
        - Linux/other: $XDG_CACHE_HOME/gogchat/ if set, else ~/.cache/gogchat/
        """
        system = platform.system()
        if system == "Darwin":
            return Path.home() / "Library" / "Caches" / "gogchat"
        # Linux and everything else
        xdg = os.environ.get("XDG_CACHE_HOME")
        if xdg:
            return Path(xdg) / "gogchat"
        return Path.home() / ".cache" / "gogchat"

    @staticmethod
    def _space_id(space_name: str) -> str:
        """Extract the space ID from a fully-qualified space name.

        Example: ``spaces/AAAA`` -> ``AAAA``.
        """
        return space_name.split("/")[-1]

    def _read(self, path: Path) -> dict | None:
        """Read and parse a cache file.

        Returns the parsed JSON wrapper dict (with ``timestamp`` and ``data``
        keys) or ``None`` if the file is missing or corrupt.
        """
        try:
            data = path.read_text(encoding="utf-8")
            return json.loads(data)
        except FileNotFoundError:
            return None
        except (json.JSONDecodeError, OSError) as exc:
            logger.warning("Failed to read cache file %s: %s", path, exc)
            return None

    def _write(self, path: Path, data: Any) -> None:
        """Write *data* to a cache file using an atomic rename.

        Parent directories are created as needed.  The payload is wrapped in::

            {"timestamp": "<ISO 8601>", "data": <data>}

        Errors are logged but never raised.
        """
        wrapper = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "data": data,
        }
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            # Write to a temp file in the same directory, then atomically rename.
            fd, tmp_path = tempfile.mkstemp(
                dir=path.parent, suffix=".tmp", prefix=".cache_"
            )
            try:
                with os.fdopen(fd, "w", encoding="utf-8") as fh:
                    json.dump(wrapper, fh, ensure_ascii=False)
                os.replace(tmp_path, path)
            except BaseException:
                # Clean up the temp file on any failure.
                try:
                    os.unlink(tmp_path)
                except OSError:
                    pass
                raise
        except OSError as exc:
            logger.error("Failed to write cache file %s: %s", path, exc)

    def _is_expired(self, path: Path, max_age_seconds: float) -> bool:
        """Return ``True`` if *path* is missing or older than *max_age_seconds*."""
        wrapper = self._read(path)
        if wrapper is None:
            return True
        ts_str = wrapper.get("timestamp")
        if not ts_str:
            return True
        try:
            ts = datetime.fromisoformat(ts_str)
            age = (datetime.now(timezone.utc) - ts).total_seconds()
            return age > max_age_seconds
        except (ValueError, TypeError) as exc:
            logger.warning("Invalid timestamp in %s: %s", path, exc)
            return True

    # -- spaces ---------------------------------------------------------------

    def get_spaces(self) -> list[dict] | None:
        """Read the cached spaces list, or ``None`` if not cached."""
        wrapper = self._read(self._dir / "spaces.json")
        if wrapper is None:
            return None
        return wrapper.get("data")

    def set_spaces(self, spaces: list[dict]) -> None:
        """Write the spaces list to the cache."""
        self._write(self._dir / "spaces.json", spaces)

    # -- messages -------------------------------------------------------------

    def get_messages(self, space_name: str) -> list[dict] | None:
        """Read cached messages for *space_name*, or ``None`` if not cached."""
        path = self._dir / "messages" / f"{self._space_id(space_name)}.json"
        wrapper = self._read(path)
        if wrapper is None:
            return None
        return wrapper.get("data")

    def set_messages(self, space_name: str, messages: list[dict]) -> None:
        """Write messages for *space_name* to the cache."""
        path = self._dir / "messages" / f"{self._space_id(space_name)}.json"
        self._write(path, messages)

    # -- members --------------------------------------------------------------

    def get_members(self, space_name: str) -> list[dict] | None:
        """Read cached members for *space_name*.

        Returns ``None`` if not cached **or** if the cache entry has expired
        (TTL: 1 hour).
        """
        path = self._dir / "members" / f"{self._space_id(space_name)}.json"
        if self._is_expired(path, _MEMBERS_TTL_SECONDS):
            return None
        wrapper = self._read(path)
        if wrapper is None:
            return None
        return wrapper.get("data")

    def set_members(self, space_name: str, members: list[dict]) -> None:
        """Write members for *space_name* to the cache."""
        path = self._dir / "members" / f"{self._space_id(space_name)}.json"
        self._write(path, members)

    # -- unread states --------------------------------------------------------

    def get_unread_states(self) -> dict[str, str] | None:
        """Read the cached unread states map, or ``None`` if not cached."""
        wrapper = self._read(self._dir / "unread.json")
        if wrapper is None:
            return None
        return wrapper.get("data")

    def set_unread_states(self, states: dict[str, str]) -> None:
        """Write the unread states map to the cache."""
        self._write(self._dir / "unread.json", states)

    # -- invalidation ---------------------------------------------------------

    def invalidate_messages(self, space_name: str) -> None:
        """Delete the cached messages file for *space_name*."""
        path = self._dir / "messages" / f"{self._space_id(space_name)}.json"
        try:
            path.unlink(missing_ok=True)
        except OSError as exc:
            logger.error("Failed to invalidate messages cache %s: %s", path, exc)

    def invalidate_all(self) -> None:
        """Delete the entire cache directory (hard refresh)."""
        import shutil

        try:
            shutil.rmtree(self._dir, ignore_errors=True)
        except OSError as exc:
            logger.error("Failed to remove cache directory %s: %s", self._dir, exc)


# -- singleton ----------------------------------------------------------------

_instance: Cache | None = None


def get_cache() -> Cache:
    """Return the lazily-created singleton :class:`Cache` instance."""
    global _instance
    if _instance is None:
        _instance = Cache()
    return _instance

"""Base provider interface for scraping metadata."""
from __future__ import annotations

from abc import ABC, abstractmethod
from pathlib import Path


class BaseProvider(ABC):
    """Base class for all metadata providers."""

    name: str = ""
    supported_types: set[str] = set()
    priority: int = 0

    @abstractmethod
    def search(self, query: str, **kwargs) -> list[dict]:
        """Search for metadata. Returns list of candidate matches."""

    @abstractmethod
    def get_detail(self, provider_id: str, **kwargs) -> dict:
        """Get detailed metadata by provider-specific ID."""

    @abstractmethod
    def scrape(self, query: str, filename: str, out_dir: Path, **kwargs) -> dict:
        """Full scrape: search → match → generate NFO + artwork.

        Returns {"status": "ok", "match": title, ...} or {"status": "not_found"}.
        This is the main entry point that registry calls.
        """

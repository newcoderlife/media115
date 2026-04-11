"""Provider registry with priority-based fallback."""
from __future__ import annotations

from pathlib import Path

from media115.scraper.provider import BaseProvider


# Registry: media_type → list of providers sorted by priority
_PROVIDERS: dict[str, list[BaseProvider]] = {}


def register(provider: BaseProvider):
    """Register a provider for its supported types."""
    for media_type in provider.supported_types:
        if media_type not in _PROVIDERS:
            _PROVIDERS[media_type] = []
        _PROVIDERS[media_type].append(provider)
        _PROVIDERS[media_type].sort(key=lambda p: p.priority)


def get_providers(media_type: str) -> list[BaseProvider]:
    """Get registered providers for a media type, sorted by priority."""
    return _PROVIDERS.get(media_type, [])


def scrape(media_type: str, query: str, filename: str, out_dir: Path, **kwargs) -> dict:
    """Try providers in priority order. First success wins."""
    from media115.log import get_logger
    logger = get_logger()

    providers = get_providers(media_type)
    if not providers:
        return {"status": "error", "error": f"No providers for type: {media_type}"}

    for provider in providers:
        try:
            logger.debug("SCRAPE     trying %s for %s '%s'", provider.name, media_type, query[:40])
            result = provider.scrape(query, filename, out_dir, **kwargs)
            if result and result.get("status") == "ok":
                result["_source"] = provider.name
                return result
            logger.debug("SCRAPE     %s returned %s", provider.name, result.get("status", "?"))
        except Exception as e:
            logger.warning("SCRAPE     %s failed: %s", provider.name, e)
            continue

    return {"status": "not_found", "query": query}

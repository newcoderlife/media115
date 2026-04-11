"""Auto-register all providers."""
from __future__ import annotations

from media115.scraper import registry
from media115.scraper.providers.tmdb_provider import TMDBMovieProvider, TMDBTVProvider
from media115.scraper.providers.bangumi_provider import BangumiProvider
from media115.scraper.providers.av_provider import Jav321Provider, JavFreeProvider

# Register providers
# Movie
registry.register(TMDBMovieProvider())

# TV: TMDB primary
registry.register(TMDBTVProvider())

# Anime: Bangumi primary, TMDB fallback
_tmdb_tv_anime = TMDBTVProvider()
_tmdb_tv_anime.priority = 1  # fallback for anime
# Note: TMDBTVProvider supports both "tv" and "anime"
# For "tv", TMDB is priority 0
# For "anime", Bangumi is priority 0, TMDB is priority 1
registry.register(BangumiProvider())

# AV
registry.register(Jav321Provider())
registry.register(JavFreeProvider())

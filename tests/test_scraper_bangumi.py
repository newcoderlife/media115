"""Bangumi API scraper tests. Hits real Bangumi API (read-only, free)."""

import os
import pytest

pytestmark = pytest.mark.live

from media115.scraper.bangumi import BangumiClient


@pytest.fixture
def bgm():
    token = os.environ.get("BANGUMI_ACCESS_TOKEN")
    return BangumiClient(access_token=token)


class TestBangumiSearch:
    def test_search_attack_on_titan(self, bgm):
        results = bgm.search("进击的巨人", subject_type=2)  # type=2 is anime
        assert len(results) > 0
        # Should find the main entry
        names = [r["name"] for r in results]
        assert any("進撃の巨人" in n or "进击的巨人" in n for n in names)

    def test_search_bocchi(self, bgm):
        results = bgm.search("孤独摇滚", subject_type=2)
        assert len(results) > 0

    def test_search_no_results(self, bgm):
        results = bgm.search("xyznonexistent12345", subject_type=2)
        assert len(results) == 0


class TestBangumiSubject:
    def test_get_subject(self, bgm):
        # Bocchi the Rock! = bgm id 328609
        subject = bgm.subject(328609)
        assert subject["id"] == 328609
        assert subject["name"] is not None
        assert subject["name_cn"] is not None
        assert "images" in subject

    def test_get_subject_has_summary(self, bgm):
        subject = bgm.subject(328609)
        assert "summary" in subject


class TestBangumiEpisodes:
    def test_get_episodes(self, bgm):
        episodes = bgm.episodes(328609)  # Bocchi the Rock!
        assert len(episodes) > 0
        # Should have episode numbers
        assert "ep" in episodes[0] or "sort" in episodes[0]

    def test_get_episodes_with_type(self, bgm):
        # type=0 is main episodes
        episodes = bgm.episodes(328609, episode_type=0)
        assert len(episodes) > 0


class TestBangumiSubjectPersons:
    def test_get_persons(self, bgm):
        persons = bgm.subject_persons(328609)
        assert len(persons) > 0

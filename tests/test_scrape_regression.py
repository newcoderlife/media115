"""Regression tests: auto-loaded from scrape_cases.json.

Every user correction becomes a test case. If the analyzer or rules
change, these tests catch regressions — ensuring previously-corrected
cases still produce the correct result.
"""

import json
import pytest
from pathlib import Path

from media115.scraper.analyzer import (
    analyze_filename,
    load_cases,
    match_against_cases,
)

CASES_PATH = Path(__file__).parent / "scrape_cases.json"


def _load_regression_cases():
    if not CASES_PATH.exists():
        return []
    return json.loads(CASES_PATH.read_text())


_cases = _load_regression_cases()


@pytest.mark.parametrize(
    "case",
    _cases,
    ids=[c["id"] for c in _cases],
)
def test_regression(case):
    """Each case in scrape_cases.json is a regression test.

    The analyzer should either:
    1. Match the filename against the case database (confidence=high), or
    2. Produce a heuristic result that matches the expected fields.
    """
    filename = case["filename"]
    expect = case["expect"]

    # First try: match against the case database itself
    all_cases = load_cases(CASES_PATH)
    rule_result = match_against_cases(filename, all_cases)

    if rule_result:
        # Matched a known rule — verify the rule output is correct
        result = rule_result
    else:
        # No rule match — fall back to heuristic
        result = analyze_filename(filename)

    # Verify expected fields
    if "type" in expect:
        assert result.media_type == expect["type"], (
            f"Expected type={expect['type']}, got {result.media_type} for {filename}"
        )

    if "title" in expect:
        assert expect["title"] in result.title or result.title in expect["title"], (
            f"Expected title containing '{expect['title']}', got '{result.title}' "
            f"for {filename}"
        )

    if "year" in expect:
        assert result.year == expect["year"], (
            f"Expected year={expect['year']}, got {result.year} for {filename}"
        )

    if "season" in expect:
        assert result.season == expect["season"], (
            f"Expected season={expect['season']}, got {result.season} for {filename}"
        )

    if "episode" in expect:
        assert result.episode == expect["episode"], (
            f"Expected episode={expect['episode']}, got {result.episode} for {filename}"
        )

    if "source" in expect:
        assert result.source == expect["source"], (
            f"Expected source={expect['source']}, got {result.source} for {filename}"
        )

    if "source_id" in expect:
        assert result.source_id == expect["source_id"], (
            f"Expected source_id={expect['source_id']}, got {result.source_id} "
            f"for {filename}"
        )

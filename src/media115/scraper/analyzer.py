"""Media file analyzer: classify files and extract metadata from filenames.

This module provides the logic that LLM skills call. It handles:
1. Loading correction rules from scrape_cases.json
2. Matching filenames against known rules first
3. Falling back to heuristic analysis for unknown files
"""

import datetime
import json
import re
from pathlib import Path
from dataclasses import dataclass


@dataclass
class AnalysisResult:
    filename: str
    media_type: str  # movie, tv, anime, av, unknown
    title: str = ""
    year: int | None = None
    season: int | None = None
    episode: int | None = None
    source: str = ""  # tmdb, bangumi, javbus
    source_id: str = ""
    confidence: str = "low"  # high (from rules), medium (heuristic), low (guess)
    note: str = ""


def load_cases(cases_path: Path) -> list[dict]:
    if not cases_path.exists():
        return []
    return json.loads(cases_path.read_text())


def match_against_cases(filename: str, cases: list[dict]) -> AnalysisResult | None:
    """Check if filename matches any known correction case."""
    for case in cases:
        case_fn = case["filename"]
        # Exact match
        if filename == case_fn:
            return _case_to_result(filename, case)
        # Basename match (ignore path)
        if Path(filename).name == Path(case_fn).name:
            return _case_to_result(filename, case)
    return None


def analyze_filename(filename: str) -> AnalysisResult:
    """Heuristic analysis of a media filename.

    This is the fallback when no rule matches. LLM skills can do better
    than these heuristics — this exists so the system works without LLM too.
    """
    name = Path(filename).stem

    # AV:番号模式 (ABC-123, FC2-PPV-1234567, 012345_678)
    if re.search(r"^[A-Z]{2,6}-\d{3,5}$", name, re.IGNORECASE):
        number = name.upper()
        return AnalysisResult(
            filename=filename,
            media_type="av",
            title=number,
            source="javbus",
            confidence="medium",
        )
    if re.search(r"FC2[-_]?PPV[-_]?\d+", name, re.IGNORECASE):
        return AnalysisResult(
            filename=filename,
            media_type="av",
            title=name,
            source="javbus",
            confidence="medium",
        )

    # TV/Anime: S01E01 pattern
    ep_match = re.search(r"S(\d{1,2})E(\d{1,3})", name, re.IGNORECASE)
    if ep_match:
        season = int(ep_match.group(1))
        episode = int(ep_match.group(2))
        # Extract title (everything before SxxExx)
        title = name[: ep_match.start()].strip().rstrip(".-_ ")
        # Guess anime vs tv: subtitle group brackets suggest anime
        if re.search(r"^\[", name):
            return AnalysisResult(
                filename=filename,
                media_type="anime",
                title=title,
                season=season,
                episode=episode,
                source="bangumi",
                confidence="medium",
            )
        return AnalysisResult(
            filename=filename,
            media_type="tv",
            title=title,
            season=season,
            episode=episode,
            source="tmdb",
            confidence="medium",
        )

    # Anime: [SubGroup] Title - Episode pattern
    anime_match = re.search(r"^\[.+?\]\s*(.+?)\s*-\s*(\d+)", name)
    if anime_match:
        title = anime_match.group(1).strip()
        episode = int(anime_match.group(2))
        return AnalysisResult(
            filename=filename,
            media_type="anime",
            title=title,
            episode=episode,
            source="bangumi",
            confidence="medium",
        )

    # Movie: Title.Year pattern
    movie_match = re.search(r"^(.+?)[.\s](\d{4})[.\s]", name)
    if movie_match:
        title = movie_match.group(1).replace(".", " ").strip()
        year = int(movie_match.group(2))
        return AnalysisResult(
            filename=filename,
            media_type="movie",
            title=title,
            year=year,
            source="tmdb",
            confidence="medium",
        )

    return AnalysisResult(
        filename=filename,
        media_type="unknown",
        confidence="low",
        note="Could not determine media type from filename",
    )


def add_case(cases_path: Path, result: AnalysisResult, correction: dict | None = None):
    """Add a verified/corrected result as a new test case."""
    cases = load_cases(cases_path)

    expect = correction or {}
    if not correction:
        expect = {
            "type": result.media_type,
            "title": result.title,
            "source": result.source,
        }
        if result.source_id:
            expect["source_id"] = result.source_id
        if result.year:
            expect["year"] = result.year
        if result.season is not None:
            expect["season"] = result.season
        if result.episode is not None:
            expect["episode"] = result.episode

    # Check if case already exists for this filename
    for existing in cases:
        if existing["filename"] == result.filename:
            existing["expect"] = expect
            cases_path.write_text(json.dumps(cases, ensure_ascii=False, indent=2))
            return

    new_case = {
        "id": f"case_{_next_case_id(cases):04d}",
        "filename": result.filename,
        "expect": expect,
        "added": datetime.date.today().isoformat(),
        "note": result.note or "",
    }
    cases.append(new_case)
    cases_path.write_text(json.dumps(cases, ensure_ascii=False, indent=2))


def _next_case_id(cases: list[dict]) -> int:
    if not cases:
        return 1
    ids = []
    for c in cases:
        parts = c.get("id", "").split("_")
        if len(parts) == 2 and parts[1].isdigit():
            ids.append(int(parts[1]))
    return max(ids, default=0) + 1


def _case_to_result(filename: str, case: dict) -> AnalysisResult:
    expect = case["expect"]
    return AnalysisResult(
        filename=filename,
        media_type=expect.get("type", "unknown"),
        title=expect.get("title", ""),
        year=expect.get("year"),
        season=expect.get("season"),
        episode=expect.get("episode"),
        source=expect.get("source", ""),
        source_id=expect.get("source_id", ""),
        confidence="high",  # From verified case
        note=case.get("note", ""),
    )

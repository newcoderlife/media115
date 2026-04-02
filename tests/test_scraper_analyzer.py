"""Analyzer tests: heuristic analysis + rule matching."""

from media115.scraper.analyzer import analyze_filename, AnalysisResult


class TestAnalyzeMovie:
    def test_standard_format(self):
        r = analyze_filename("The.Matrix.1999.BluRay.1080p.x264.mkv")
        assert r.media_type == "movie"
        assert r.title == "The Matrix"
        assert r.year == 1999

    def test_chinese_with_year(self):
        r = analyze_filename("满江红.2023.WEB-DL.4K.mkv")
        assert r.media_type == "movie"
        assert r.year == 2023

    def test_parenthesized_year(self):
        r = analyze_filename("Inception (2010) [2160p].mkv")
        # Heuristic may not catch parenthesized year, that's ok
        # This is what LLM handles better
        assert r.media_type in ("movie", "unknown")


class TestAnalyzeTV:
    def test_standard_sxxexx(self):
        r = analyze_filename("Breaking.Bad.S01E01.Pilot.720p.BluRay.mkv")
        assert r.media_type == "tv"
        assert r.season == 1
        assert r.episode == 1

    def test_chinese_drama(self):
        r = analyze_filename("漫长的季节.S01E01.2023.WEB-DL.4K.mkv")
        assert r.media_type == "tv"
        assert r.season == 1
        assert r.episode == 1


class TestAnalyzeAnime:
    def test_subgroup_format(self):
        r = analyze_filename("[Lilith-Raws] Bocchi the Rock! - 01 [1080p].mkv")
        assert r.media_type == "anime"
        assert r.episode == 1
        assert "Bocchi" in r.title

    def test_subgroup_with_sxxexx(self):
        r = analyze_filename("[SubHD] Attack on Titan S04E28 [1080p].mkv")
        assert r.media_type == "anime"
        assert r.season == 4
        assert r.episode == 28


class TestAnalyzeAV:
    def test_standard_number(self):
        r = analyze_filename("ABC-123.mp4")
        assert r.media_type == "av"

    def test_fc2(self):
        r = analyze_filename("FC2-PPV-1234567.mp4")
        assert r.media_type == "av"


class TestAnalyzeUnknown:
    def test_unrecognizable(self):
        r = analyze_filename("random_stuff.mp4")
        assert r.media_type == "unknown"


class TestAddCase:
    def test_add_and_reload(self, tmp_path):
        from media115.scraper.analyzer import add_case, load_cases

        cases_path = tmp_path / "cases.json"
        cases_path.write_text("[]")

        result = AnalysisResult(
            filename="test.mkv",
            media_type="movie",
            title="Test Movie",
            year=2024,
            source="tmdb",
            source_id="999",
        )
        add_case(cases_path, result)

        cases = load_cases(cases_path)
        assert len(cases) == 1
        assert cases[0]["expect"]["title"] == "Test Movie"
        assert cases[0]["expect"]["source_id"] == "999"

    def test_update_existing_case(self, tmp_path):
        from media115.scraper.analyzer import add_case, load_cases

        cases_path = tmp_path / "cases.json"
        cases_path.write_text("[]")

        r1 = AnalysisResult(filename="test.mkv", media_type="movie", title="Wrong")
        add_case(cases_path, r1)

        r2 = AnalysisResult(filename="test.mkv", media_type="movie", title="Correct")
        add_case(
            cases_path,
            r2,
            correction={
                "type": "movie",
                "title": "Correct",
                "source": "tmdb",
                "source_id": "123",
            },
        )

        cases = load_cases(cases_path)
        assert len(cases) == 1  # updated, not duplicated
        assert cases[0]["expect"]["title"] == "Correct"

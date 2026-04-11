from pathlib import Path


def test_registry_has_providers():
    import media115.scraper.providers  # trigger registration
    from media115.scraper.registry import get_providers
    assert len(get_providers("movie")) >= 1
    assert len(get_providers("tv")) >= 1
    assert len(get_providers("av")) >= 1
    assert len(get_providers("anime")) >= 1


def test_registry_priority_order():
    import media115.scraper.providers
    from media115.scraper.registry import get_providers
    providers = get_providers("av")
    assert providers[0].name == "jav321"
    assert providers[1].name == "javfree"


def test_scrape_by_type_not_found():
    from media115.scraper.scrape import scrape_by_type
    result = scrape_by_type("nonexistent_type", "query", "file.mkv", Path("/tmp"))
    assert result["status"] == "error"

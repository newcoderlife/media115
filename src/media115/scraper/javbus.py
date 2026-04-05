"""JavBus HTML scraper for AV metadata."""

from __future__ import annotations

from lxml import html as lxml_html

from media115.scraper.base import ThrottledClient

JAVBUS_URL = "https://www.javbus.com"
_client: ThrottledClient | None = None


def _get_client() -> ThrottledClient:
    global _client
    if _client is None:
        _client = ThrottledClient()
    return _client


def fetch_metadata(number: str) -> dict | None:
    """Fetch AV metadata from JavBus by number. May be blocked by Cloudflare."""
    try:
        resp = _get_client().get(
            f"{JAVBUS_URL}/{number}",
            headers={"Accept-Language": "zh-TW,zh;q=0.9"},
            follow_redirects=True,
        )
        if resp.status_code != 200:
            return None
        result = parse_detail_page(resp.text)
        return result if result.get("number") else None
    except Exception:
        return None


def parse_detail_page(html_str: str) -> dict:
    """Parse a JavBus detail page HTML."""

    doc = lxml_html.fromstring(html_str)
    result = {}

    title_els = doc.xpath("//h3/text()")
    result["title"] = title_els[0].strip() if title_els else ""

    number_els = doc.xpath(
        '//span[@class="header"][contains(text(), "識別碼")]/following-sibling::span[1]/text()'
    )
    if not number_els:
        number_els = doc.xpath('//span[contains(@style, "color")]/text()')
    result["number"] = number_els[0].strip() if number_els else ""

    _extract_info_field(doc, result, "release_date", "發行日期")

    runtime_text = _get_info_text(doc, "長度")
    result["runtime"] = "".join(c for c in (runtime_text or "") if c.isdigit())

    director_els = doc.xpath('//a[contains(@href, "/director/")]/text()')
    result["director"] = director_els[0].strip() if director_els else ""

    studio_els = doc.xpath('//a[contains(@href, "/studio/")]/text()')
    result["studio"] = studio_els[0].strip() if studio_els else ""

    label_els = doc.xpath('//a[contains(@href, "/label/")]/text()')
    result["label"] = label_els[0].strip() if label_els else ""

    series_els = doc.xpath('//a[contains(@href, "/series/")]/text()')
    result["series"] = series_els[0].strip() if series_els else ""

    result["actors"] = [a.strip() for a in doc.xpath('//div[@class="star-name"]/a/text()')]
    result["genres"] = [
        g.strip()
        for g in doc.xpath('//span[@class="genre"]/label/a[contains(@href, "/genre/")]/text()')
    ]

    cover_els = doc.xpath('//a[@class="bigImage"]/@href')
    result["cover_url"] = cover_els[0] if cover_els else ""
    result["screenshots"] = doc.xpath("//div[@id='sample-waterfall']/a/@href")

    return result


def _get_info_text(doc, label: str) -> str | None:
    p_els = doc.xpath(f'//span[@class="header"][contains(text(), "{label}")]/parent::p')
    if p_els:
        full_text = p_els[0].text_content()
        parts = full_text.split(":", 1) if ":" in full_text else full_text.split("：", 1)
        if len(parts) > 1:
            return parts[1].strip()
    return None


def _extract_info_field(doc, result: dict, key: str, label: str):
    text = _get_info_text(doc, label)
    result[key] = text.strip() if text else ""

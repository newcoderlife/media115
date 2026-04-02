"""JavBus HTML scraper for AV metadata."""

from lxml import html as lxml_html


def parse_detail_page(html_str: str) -> dict:
    doc = lxml_html.fromstring(html_str)

    result = {}

    # Title from h3
    title_els = doc.xpath("//h3/text()")
    result["title"] = title_els[0].strip() if title_els else ""

    # Number
    number_els = doc.xpath(
        '//span[@class="header"][contains(text(), "識別碼")]/following-sibling::span[1]/text()'
    )
    if not number_els:
        # Fallback: try color span
        number_els = doc.xpath('//span[contains(@style, "color")]/text()')
    result["number"] = number_els[0].strip() if number_els else ""

    # Release date
    _extract_info_field(doc, result, "release_date", "發行日期")

    # Runtime
    runtime_text = _get_info_text(doc, "長度")
    if runtime_text:
        # Extract digits from "120分鐘"
        digits = "".join(c for c in runtime_text if c.isdigit())
        result["runtime"] = digits
    else:
        result["runtime"] = ""

    # Director
    director_els = doc.xpath('//a[contains(@href, "/director/")]/text()')
    result["director"] = director_els[0].strip() if director_els else ""

    # Studio (製作商)
    studio_els = doc.xpath('//a[contains(@href, "/studio/")]/text()')
    result["studio"] = studio_els[0].strip() if studio_els else ""

    # Label (發行商)
    label_els = doc.xpath('//a[contains(@href, "/label/")]/text()')
    result["label"] = label_els[0].strip() if label_els else ""

    # Series
    series_els = doc.xpath('//a[contains(@href, "/series/")]/text()')
    result["series"] = series_els[0].strip() if series_els else ""

    # Actors
    actor_els = doc.xpath('//div[@class="star-name"]/a/text()')
    result["actors"] = [a.strip() for a in actor_els]

    # Genres
    genre_els = doc.xpath(
        '//span[@class="genre"]/label/a[contains(@href, "/genre/")]/text()'
    )
    result["genres"] = [g.strip() for g in genre_els]

    # Cover image
    cover_els = doc.xpath('//a[@class="bigImage"]/@href')
    result["cover_url"] = cover_els[0] if cover_els else ""

    # Screenshots
    screenshot_els = doc.xpath("//div[@id='sample-waterfall']/a/@href")
    result["screenshots"] = screenshot_els

    return result


def _get_info_text(doc, label: str) -> str | None:
    """Extract text after a header span like '發行日期:'."""
    # The text is a sibling text node of the parent <p>
    p_els = doc.xpath(f'//span[@class="header"][contains(text(), "{label}")]/parent::p')
    if p_els:
        full_text = p_els[0].text_content()
        # Remove the label part
        parts = (
            full_text.split(":", 1) if ":" in full_text else full_text.split("：", 1)
        )
        if len(parts) > 1:
            return parts[1].strip()
    return None


def _extract_info_field(doc, result: dict, key: str, label: str):
    text = _get_info_text(doc, label)
    result[key] = text.strip() if text else ""

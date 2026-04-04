"""NFO file generation (Kodi format) and parsing.

Follows the Kodi NFO standard for maximum compatibility with
Jellyfin, Infuse, and Kodi itself.
"""

from pathlib import Path

from lxml import etree


def _add(parent: etree._Element, tag: str, text, **attrib):
    """Add a child element with text content. Skips if text is None or empty."""
    if text is None:
        return None
    s = str(text)
    if not s:
        return None
    el = etree.SubElement(parent, tag, **attrib)
    el.text = s
    return el


def generate_movie_nfo(metadata: dict, output_path: Path):
    """Generate Kodi-standard movie NFO."""
    root = etree.Element("movie")

    _add(root, "title", metadata.get("title"))
    _add(root, "originaltitle", metadata.get("originaltitle"))
    _add(root, "sorttitle", metadata.get("sorttitle"))
    _add(root, "year", metadata.get("year"))
    _add(root, "premiered", metadata.get("premiered"))
    _add(root, "plot", metadata.get("plot"))
    _add(root, "outline", metadata.get("outline"))
    _add(root, "tagline", metadata.get("tagline"))
    _add(root, "runtime", metadata.get("runtime"))
    _add(root, "mpaa", metadata.get("mpaa"))

    # Ratings
    rating = metadata.get("rating")
    if rating:
        _add(root, "rating", rating)
    votes = metadata.get("votes")
    if votes:
        _add(root, "votes", votes)

    # Collection/set
    collection = metadata.get("set")
    if collection:
        set_el = etree.SubElement(root, "set")
        _add(set_el, "name", collection)

    # Studios
    for studio in _as_list(metadata.get("studios")) or _as_list(metadata.get("studio")):
        _add(root, "studio", studio)

    # Genres
    for genre in metadata.get("genres", []):
        _add(root, "genre", genre)

    # Tags
    for tag in metadata.get("tags", []):
        _add(root, "tag", tag)

    # Country
    for country in metadata.get("countries", []):
        _add(root, "country", country)

    # Credits (writers)
    for writer in metadata.get("credits", []):
        _add(root, "credits", writer)

    # Directors
    for director in metadata.get("directors", []):
        _add(root, "director", director)

    # Actors
    for actor_data in metadata.get("actors", []):
        actor_el = etree.SubElement(root, "actor")
        _add(actor_el, "name", actor_data.get("name"))
        _add(actor_el, "role", actor_data.get("role"))
        _add(actor_el, "thumb", actor_data.get("thumb"))

    # Unique IDs
    for uid_type, uid_value in metadata.get("uniqueids", {}).items():
        uid_el = etree.SubElement(root, "uniqueid", type=uid_type)
        uid_el.text = str(uid_value)

    # Thumb/fanart (image URLs for Kodi)
    thumb = metadata.get("thumb")
    if thumb:
        _add(root, "thumb", thumb, aspect="poster")
    fanart_url = metadata.get("fanart")
    if fanart_url:
        fanart_el = etree.SubElement(root, "fanart")
        _add(fanart_el, "thumb", fanart_url)

    _write_xml(root, output_path)


def generate_episode_nfo(metadata: dict, output_path: Path):
    """Generate Kodi-standard episode NFO."""
    root = etree.Element("episodedetails")

    _add(root, "title", metadata.get("title"))
    _add(root, "showtitle", metadata.get("showtitle"))
    _add(root, "season", metadata.get("season"))
    _add(root, "episode", metadata.get("episode"))
    _add(root, "plot", metadata.get("plot"))
    _add(root, "aired", metadata.get("aired"))
    _add(root, "rating", metadata.get("rating"))
    _add(root, "votes", metadata.get("votes"))
    _add(root, "runtime", metadata.get("runtime"))

    for director in metadata.get("directors", []):
        _add(root, "director", director)

    for writer in metadata.get("credits", []):
        _add(root, "credits", writer)

    for actor_data in metadata.get("actors", []):
        actor_el = etree.SubElement(root, "actor")
        _add(actor_el, "name", actor_data.get("name"))
        _add(actor_el, "role", actor_data.get("role"))

    for uid_type, uid_value in metadata.get("uniqueids", {}).items():
        uid_el = etree.SubElement(root, "uniqueid", type=uid_type)
        uid_el.text = str(uid_value)

    _write_xml(root, output_path)


def generate_tvshow_nfo(metadata: dict, output_path: Path):
    """Generate Kodi-standard tvshow NFO (for the series, not episodes)."""
    root = etree.Element("tvshow")

    _add(root, "title", metadata.get("title"))
    _add(root, "originaltitle", metadata.get("originaltitle"))
    _add(root, "year", metadata.get("year"))
    _add(root, "plot", metadata.get("plot"))
    _add(root, "rating", metadata.get("rating"))
    _add(root, "votes", metadata.get("votes"))
    _add(root, "premiered", metadata.get("premiered"))
    _add(root, "status", metadata.get("status"))

    for genre in metadata.get("genres", []):
        _add(root, "genre", genre)

    for studio in _as_list(metadata.get("studios")) or _as_list(metadata.get("studio")):
        _add(root, "studio", studio)

    for actor_data in metadata.get("actors", []):
        actor_el = etree.SubElement(root, "actor")
        _add(actor_el, "name", actor_data.get("name"))
        _add(actor_el, "role", actor_data.get("role"))

    for uid_type, uid_value in metadata.get("uniqueids", {}).items():
        uid_el = etree.SubElement(root, "uniqueid", type=uid_type)
        uid_el.text = str(uid_value)

    thumb = metadata.get("thumb")
    if thumb:
        _add(root, "thumb", thumb, aspect="poster")
    fanart_url = metadata.get("fanart")
    if fanart_url:
        fanart_el = etree.SubElement(root, "fanart")
        _add(fanart_el, "thumb", fanart_url)

    _write_xml(root, output_path)


def parse_nfo(nfo_path: Path) -> dict:
    """Parse any NFO file (movie, episode, tvshow) tolerantly."""
    content = nfo_path.read_bytes()
    parser = etree.HTMLParser(encoding="utf-8")
    doc = etree.fromstring(content, parser)

    root = None
    for tag in ("movie", "episodedetails", "tvshow"):
        found = doc.find(f".//{tag}")
        if found is not None:
            root = found
            break
    if root is None:
        root = doc

    result = {}

    for field in (
        "title",
        "originaltitle",
        "sorttitle",
        "year",
        "plot",
        "outline",
        "tagline",
        "runtime",
        "rating",
        "votes",
        "premiered",
        "mpaa",
        "studio",
        "season",
        "episode",
        "aired",
        "showtitle",
        "status",
    ):
        val = root.findtext(field)
        if val:
            result[field] = val

    # Set/collection
    set_el = root.find("set")
    if set_el is not None:
        set_name = set_el.findtext("name") or set_el.text
        if set_name:
            result["set"] = set_name.strip()

    genres = [el.text for el in root.findall("genre") if el.text]
    if genres:
        result["genres"] = genres

    tags = [el.text for el in root.findall("tag") if el.text]
    if tags:
        result["tags"] = tags

    countries = [el.text for el in root.findall("country") if el.text]
    if countries:
        result["countries"] = countries

    directors = [el.text for el in root.findall("director") if el.text]
    if directors:
        result["directors"] = directors

    credits = [el.text for el in root.findall("credits") if el.text]
    if credits:
        result["credits"] = credits

    actors = []
    for actor_el in root.findall("actor"):
        actor = {}
        name = actor_el.findtext("name")
        if name:
            actor["name"] = name
        role = actor_el.findtext("role")
        if role:
            actor["role"] = role
        thumb = actor_el.findtext("thumb")
        if thumb:
            actor["thumb"] = thumb
        if actor:
            actors.append(actor)
    if actors:
        result["actors"] = actors

    uniqueids = {}
    for uid_el in root.findall("uniqueid"):
        uid_type = uid_el.get("type")
        if uid_type and uid_el.text:
            uniqueids[uid_type] = uid_el.text
    if uniqueids:
        result["uniqueids"] = uniqueids

    return result


def _write_xml(root: etree._Element, output_path: Path):
    output_path.parent.mkdir(parents=True, exist_ok=True)
    tree = etree.ElementTree(root)
    tree.write(
        str(output_path), encoding="UTF-8", xml_declaration=True, pretty_print=True
    )


def _as_list(val) -> list:
    if val is None:
        return []
    if isinstance(val, list):
        return val
    return [val]

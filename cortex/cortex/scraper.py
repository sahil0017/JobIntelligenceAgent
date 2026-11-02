"""Job-page scraper: Playwright for JavaScript-heavy pages, plain HTTP as the fallback.

URLs come from the user, so every request (including redirects and browser sub-requests) is
restricted to public hosts on ports 80/443, which keeps the service from being pointed at
localhost or the local network. robots.txt is honoured. Pages with schema.org JobPosting data
are read from that structured block first, because it is cleaner than scraped body text.
"""

from __future__ import annotations

import ipaddress
import json
import re
import os
import socket
from dataclasses import dataclass
from pathlib import Path
from html.parser import HTMLParser
from urllib.parse import urljoin, urlparse
from urllib.robotparser import RobotFileParser

import httpx

USER_AGENT = "CareerPilotBot/0.1 (+local job-search assistant; respects robots.txt)"
MAX_BYTES = 2_000_000
MAX_TEXT = 20_000
MAX_REDIRECTS = 4
PAGE_TIMEOUT_MS = 15_000
ALLOWED_PORTS = {80, 443}
PROFILE_ARGS = ["--use-mock-keychain", "--password-store=basic", "--no-first-run"]


class ScrapeError(RuntimeError):
    """A page could not be read; the message is safe to show to the user."""


class AuthRequired(ScrapeError):
    """The site answered 401/403: the user may sign in (visible browser) and retry."""

    def __init__(self, host: str, status: int) -> None:
        super().__init__(f"{host} answered HTTP {status}; sign in to read it")
        self.host = host
        self.status = status


def profile_path() -> Path:
    return Path(os.environ.get("CORTEX_DATA_DIR") or Path(__file__).resolve().parent.parent / "data") / "browser-profile"


def profile_dir() -> Path | None:
    """Browser profile holding the user's own sign-in sessions; None until they sign in."""
    path = profile_path()
    return path if path.exists() else None


@dataclass(frozen=True)
class ScrapedPage:
    url: str
    title: str
    text: str
    method: str  # "playwright" or "http"
    structured: bool  # True when read from schema.org JobPosting data


def assert_public_url(url: str) -> str:
    parsed = urlparse(url)
    if parsed.scheme not in ("http", "https") or not parsed.hostname:
        raise ScrapeError("only http(s) page addresses can be read")
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    if port not in ALLOWED_PORTS:
        raise ScrapeError("only standard web ports (80 and 443) can be read")
    try:
        addresses = {info[4][0] for info in socket.getaddrinfo(parsed.hostname, port, proto=socket.IPPROTO_TCP)}
    except socket.gaierror:
        raise ScrapeError(f"could not resolve {parsed.hostname}") from None
    for address in addresses:
        ip = ipaddress.ip_address(address.split("%")[0])
        if not ip.is_global:
            raise ScrapeError("that address points to a private or local network, so it was not opened")
    return url


def allowed_by_robots(url: str, client: httpx.Client) -> bool:
    parsed = urlparse(url)
    robots_url = f"{parsed.scheme}://{parsed.netloc}/robots.txt"
    try:
        assert_public_url(robots_url)
        response = client.get(robots_url, timeout=5.0)
    except (ScrapeError, httpx.HTTPError):
        return True  # no readable robots.txt means no stated restriction
    if response.status_code >= 400:
        return True
    parser = RobotFileParser()
    parser.parse(response.text.splitlines())
    return parser.can_fetch(USER_AGENT, url)


class _TextExtractor(HTMLParser):
    SKIP = {"script", "style", "noscript", "svg", "nav", "footer", "header", "form", "iframe"}
    BLOCK = {"p", "div", "li", "br", "h1", "h2", "h3", "h4", "tr", "section", "article", "ul", "ol"}

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.title = ""
        self.parts: list[str] = []
        self.json_ld: list[str] = []
        self._skip = 0
        self._in_title = False
        self._in_ld = False

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag == "title":
            self._in_title = True
        elif tag == "script" and dict(attrs).get("type") == "application/ld+json":
            self._in_ld = True
            self.json_ld.append("")
        elif tag in self.SKIP:
            self._skip += 1
        elif tag in self.BLOCK:
            self.parts.append("\n")

    def handle_endtag(self, tag: str) -> None:
        if tag == "title":
            self._in_title = False
        elif tag == "script" and self._in_ld:
            self._in_ld = False
        elif tag in self.SKIP and self._skip:
            self._skip -= 1

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.title += data
        elif self._in_ld:
            self.json_ld[-1] += data
        elif not self._skip:
            self.parts.append(data)


def _clean(text: str) -> str:
    text = re.sub(r"[ \t\r\f\v]+", " ", text)
    text = re.sub(r"\n\s*\n+", "\n", text)
    return text.strip()


def _html_to_text(html: str) -> str:
    extractor = _TextExtractor()
    extractor.feed(html)
    return _clean("".join(extractor.parts))


def _find_job_posting(node: object) -> dict | None:
    if isinstance(node, list):
        for item in node:
            found = _find_job_posting(item)
            if found:
                return found
    elif isinstance(node, dict):
        kind = node.get("@type")
        if kind == "JobPosting" or (isinstance(kind, list) and "JobPosting" in kind):
            return node
        return _find_job_posting(node.get("@graph", []))
    return None


def parse_html(html: str, url: str, method: str) -> ScrapedPage:
    """Prefer structured JobPosting data; otherwise use the visible page text."""
    extractor = _TextExtractor()
    extractor.feed(html)
    title = _clean(extractor.title)
    for block in extractor.json_ld:
        try:
            posting = _find_job_posting(json.loads(block))
        except ValueError:
            continue
        if posting and posting.get("description"):
            organisation = posting.get("hiringOrganization")
            company = organisation.get("name", "") if isinstance(organisation, dict) else ""
            description = _html_to_text(str(posting["description"]))
            heading = " at ".join(part for part in (str(posting.get("title", "")), company) if part)
            return ScrapedPage(url, heading or title, (heading + "\n" + description).strip()[:MAX_TEXT], method, True)
    text = _clean("".join(extractor.parts))
    if len(text) < 80:
        raise ScrapeError("the page had no readable job text (it may need a login or block automated readers)")
    return ScrapedPage(url, title, text[:MAX_TEXT], method, False)


def _fetch_http(url: str, client: httpx.Client, parser=None):
    parser = parser or parse_html
    current = assert_public_url(url)
    for _ in range(MAX_REDIRECTS + 1):
        with client.stream("GET", current, follow_redirects=False) as response:
            if response.is_redirect:
                location = response.headers.get("location", "")
                current = assert_public_url(urljoin(current, location))
                continue
            if response.status_code in (401, 403):
                raise AuthRequired(urlparse(current).hostname or "", response.status_code)
            if response.status_code >= 400:
                raise ScrapeError(f"the site answered HTTP {response.status_code}")
            if "html" not in response.headers.get("content-type", "html").lower():
                raise ScrapeError("the address is not a web page")
            body = bytearray()
            for chunk in response.iter_bytes():
                body += chunk
                if len(body) > MAX_BYTES:
                    break
            html = bytes(body).decode(response.encoding or "utf-8", errors="replace")
            return parser(html, current, "http")
    raise ScrapeError("too many redirects")


def _fetch_playwright(url: str, parser=None):
    parser = parser or parse_html
    from playwright.sync_api import sync_playwright  # optional at import time

    with sync_playwright() as playwright:
        profile = profile_dir()
        browser = None
        if profile:  # reuse the user's own sign-in; same flags as the sign-in window so cookies decrypt
            context = playwright.chromium.launch_persistent_context(
                str(profile), headless=True, user_agent=USER_AGENT, args=PROFILE_ARGS, ignore_default_args=["--use-mock-keychain"]
            )
        else:
            browser = playwright.chromium.launch(headless=True)
            context = browser.new_context(user_agent=USER_AGENT)
        try:

            def guard(route) -> None:  # every browser request must target a public host
                request = route.request
                if request.resource_type in ("image", "media", "font"):
                    route.abort()
                    return
                try:
                    assert_public_url(request.url)
                except ScrapeError:
                    route.abort()
                    return
                route.continue_()

            context.route("**/*", guard)
            page = context.new_page()
            response = page.goto(url, wait_until="domcontentloaded", timeout=PAGE_TIMEOUT_MS)
            if response and response.status in (401, 403):
                raise AuthRequired(urlparse(url).hostname or "", response.status)
            try:
                page.wait_for_load_state("networkidle", timeout=4_000)
            except Exception:  # noqa: BLE001 - long-polling pages never go idle; the DOM is enough
                pass
            final_url = assert_public_url(page.url)
            return parser(page.content(), final_url, "playwright")
        finally:
            context.close()
            if browser:
                browser.close()


def scrape(url: str, use_browser: bool = True, parser=None):
    url = assert_public_url(url.strip())
    with httpx.Client(headers={"User-Agent": USER_AGENT}, timeout=httpx.Timeout(15.0, connect=5.0)) as client:
        if not allowed_by_robots(url, client):
            raise ScrapeError("the site's robots.txt asks automated readers not to open this page")
        if use_browser:
            try:
                return _fetch_playwright(url, parser)
            except ScrapeError:
                raise
            except Exception:  # noqa: BLE001 - browser missing or crashed: fall back to plain HTTP
                pass
        try:
            return _fetch_http(url, client, parser)
        except httpx.HTTPError as error:
            raise ScrapeError(f"could not download the page ({type(error).__name__})") from error

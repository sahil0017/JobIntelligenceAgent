"""Company career-portal search and the user's own sign-in sessions.

Search reads a careers page (JSON-LD JobPosting data first, then job-looking links) and ranks the
results against the candidate with the same embedder the copilot uses. When a portal answers
401/403 the user signs in themselves in a visible, ordinary Chromium window; the session lives in a
local browser profile that later scrapes reuse. No Google password ever reaches this service.
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import threading
from html.parser import HTMLParser
from urllib.parse import urljoin, urlparse

from . import boards
from .embeddings import Embedder
from .scraper import PROFILE_ARGS, AuthRequired, ScrapeError, assert_public_url, scrape
from .scraper import profile_dir as existing_profile, profile_path
from .skills import extract_skills

MAX_JOBS = 50
JOB_HINT = re.compile(r"/(jobs?|careers?|positions?|openings?|vacanc\w*|requisitions?|postings?|roles?)/[^/?#]+|[?&](gh_jid|jobid|job_id|reqid)=", re.I)
NOISE = re.compile(r"^(jobs?|careers?|all jobs|view all|apply( now)?|search|sign in|log ?in|home|back|next|previous|learn more|read more)$", re.I)
ATS_HOSTS = ("greenhouse.io", "lever.co", "ashbyhq.com", "workable.com", "myworkdayjobs.com", "smartrecruiters.com", "bamboohr.com", "recruitee.com")


def _site(host: str) -> str:
    parts = host.lower().split(".")
    return ".".join(parts[-2:])


class _Links(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.links: list[tuple[str, str]] = []
        self.json_ld: list[str] = []
        self._href: str | None = None
        self._text: list[str] = []
        self._ld = False

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        values = dict(attrs)
        if tag == "a" and values.get("href"):
            self._href, self._text = values["href"], []
        elif tag == "script" and values.get("type") == "application/ld+json":
            self._ld = True
            self.json_ld.append("")

    def handle_endtag(self, tag: str) -> None:
        if tag == "a" and self._href is not None:
            self.links.append((self._href, " ".join("".join(self._text).split())))
            self._href = None
        elif tag == "script":
            self._ld = False

    def handle_data(self, data: str) -> None:
        if self._ld:
            self.json_ld[-1] += data
        elif self._href is not None:
            self._text.append(data)


def _postings(node: object) -> list[dict]:
    if isinstance(node, list):
        return [p for item in node for p in _postings(item)]
    if not isinstance(node, dict):
        return []
    kind = node.get("@type")
    if kind == "JobPosting" or (isinstance(kind, list) and "JobPosting" in kind):
        return [node]
    return [p for key in ("@graph", "itemListElement", "item") for p in _postings(node.get(key))]


def extract_jobs(html: str, base_url: str) -> list[dict]:
    """Return {title, url, snippet} for job postings found on a careers page."""
    parser = _Links()
    parser.feed(html)
    host = urlparse(base_url).hostname or ""
    found: dict[str, dict] = {}

    def keep(url: str) -> bool:
        target = urlparse(url)
        if target.scheme not in ("http", "https") or not target.hostname:
            return False
        return _site(target.hostname) == _site(host) or target.hostname.endswith(ATS_HOSTS)

    for block in parser.json_ld:
        try:
            postings = _postings(json.loads(block))
        except ValueError:
            continue
        for posting in postings:
            title = str(posting.get("title", "")).strip()
            url = urljoin(base_url, str(posting.get("url") or "")) if posting.get("url") else base_url
            if title and keep(url):
                snippet = re.sub(r"<[^>]+>", " ", str(posting.get("description", "")))
                found.setdefault(url + "#" + title, {"title": title[:140], "url": url, "snippet": " ".join(snippet.split())[:240]})

    for href, text in parser.links:
        url = urljoin(base_url, href).split("#")[0]
        if not (4 <= len(text) <= 120) or NOISE.match(text) or not keep(url):
            continue
        if JOB_HINT.search(url) and url.rstrip("/") != base_url.rstrip("/"):
            found.setdefault(url, {"title": text, "url": url, "snippet": ""})
    return list(found.values())[:MAX_JOBS]


def _raw(html: str, url: str, method: str) -> tuple[str, str]:
    return html, url


def search(url: str, keywords: str, candidate_text: str, embedder: Embedder) -> dict:
    """Read a careers page and rank its jobs. Returns status ok or auth_required."""
    url = assert_public_url(url.strip())
    host = urlparse(url).hostname or ""
    jobs: list[dict] | None = None
    try:
        jobs = boards.search(url, keywords)
    except Exception:  # noqa: BLE001 - a board API change must not break page scraping
        jobs = None
    if jobs is None or not jobs:
        try:
            html, final_url = scrape(url, parser=_raw)
        except AuthRequired as error:
            return {"status": "auth_required", "host": error.host, "httpStatus": error.status, "jobs": [], "message": str(error)}
        jobs = extract_jobs(html, final_url)
    wanted = [word.lower() for word in re.split(r"[,\s]+", keywords) if word]
    if wanted:
        narrowed = [j for j in jobs if any(w in (j["title"] + " " + j["snippet"]).lower() for w in wanted)]
        jobs = narrowed or jobs
    if jobs:
        texts = [f"{j['title']}. {j['snippet']}" for j in jobs]
        vectors = embedder.embed([candidate_text or keywords or "job"] + texts)
        for job, vector in zip(jobs, vectors[1:]):
            job["score"] = embedder.calibrate(float(vectors[0] @ vector))
            job["skills"] = extract_skills(f"{job['title']} {job['snippet']}")[:6]
        jobs.sort(key=lambda j: j["score"], reverse=True)
    return {"status": "ok", "host": host, "jobs": jobs, "message": "" if jobs else "No job links were found on that page; try the company's job-list address."}


class SignIn:
    """Opens a visible Chromium window for the user to sign in; the profile persists locally."""

    def __init__(self) -> None:
        self._process: subprocess.Popen | None = None
        self._lock = threading.Lock()

    def open_window(self, url: str) -> dict:
        url = assert_public_url(url.strip())
        with self._lock:
            if self._process and self._process.poll() is None:
                return {"opened": False, "message": "A sign-in window is already open. Finish there, then close it."}
            from playwright.sync_api import sync_playwright

            with sync_playwright() as playwright:
                executable = playwright.chromium.executable_path
            profile = profile_path()
            profile.mkdir(parents=True, exist_ok=True)
            # A plain launch (no automation flags) so Google accepts the sign-in.
            self._process = subprocess.Popen(
                [executable, f"--user-data-dir={profile}", *PROFILE_ARGS, url],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        return {"opened": True, "message": "A browser window opened. Sign in with Google there, then close it and search again."}

    def status(self) -> dict:
        open_now = bool(self._process and self._process.poll() is None)
        return {"signedIn": existing_profile() is not None, "windowOpen": open_now}

    def sign_out(self) -> dict:
        if self._process and self._process.poll() is None:
            raise ScrapeError("close the sign-in window first")
        shutil.rmtree(profile_path(), ignore_errors=True)
        return self.status()

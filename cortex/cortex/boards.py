"""Public job-search JSON endpoints that big employers publish for their own career sites.

Many large careers pages (Amazon, Oracle, every Workday tenant such as NVIDIA or Salesforce) render in
JavaScript, so page scraping finds nothing. Their sites call these documented-by-use JSON endpoints, which are
fast, need no sign-in and do not require running a browser. Anything not recognised falls back to page scraping.
"""

from __future__ import annotations

import re
from urllib.parse import quote, urlparse

import httpx

from .scraper import USER_AGENT, assert_public_url

LIMIT = 40
ORACLE_HOST = "eeho.fa.us2.oraclecloud.com"
ORACLE_SITE = "CX_45001"
LOCALE = re.compile(r"^[a-z]{2}([-_][A-Za-z]{2})?$")


def _clean(text: object, size: int = 220) -> str:
    return re.sub(r"\s+", " ", re.sub(r"<[^>]+>", " ", str(text or ""))).strip()[:size]


def _amazon(query: str, client: httpx.Client) -> list[dict]:
    response = client.get(f"https://www.amazon.jobs/en/search.json?base_query={quote(query)}&result_limit={LIMIT}")
    response.raise_for_status()
    return [
        {"title": j["title"], "url": f"https://www.amazon.jobs{j['job_path']}", "snippet": _clean(f"{j.get('location', '')}. {j.get('description_short', '')}")}
        for j in response.json().get("jobs", []) if j.get("title") and j.get("job_path")
    ]


def _oracle(query: str, client: httpx.Client) -> list[dict]:
    finder = f"findReqs;siteNumber={ORACLE_SITE},limit={LIMIT},keyword={quote(query)}"
    response = client.get(f"https://{ORACLE_HOST}/hcmRestApi/resources/latest/recruitingCEJobRequisitions?onlyData=true&expand=requisitionList&finder={finder}")
    response.raise_for_status()
    rows = (response.json().get("items") or [{}])[0].get("requisitionList", [])
    return [
        {"title": r["Title"], "url": f"https://{ORACLE_HOST}/hcmUI/CandidateExperience/en/sites/{ORACLE_SITE}/job/{r['Id']}", "snippet": _clean(f"{r.get('PrimaryLocation', '')}. {r.get('ShortDescriptionStr', '')}")}
        for r in rows if r.get("Title") and r.get("Id")
    ]


def _workday(host: str, path: str, query: str, client: httpx.Client) -> list[dict]:
    parts = [p for p in path.split("/") if p and not LOCALE.match(p)]
    if not parts:
        return []
    tenant, site = host.split(".")[0], parts[0]
    response = client.post(f"https://{host}/wday/cxs/{tenant}/{site}/jobs", json={"limit": 20, "offset": 0, "searchText": query})
    response.raise_for_status()
    return [
        {"title": p["title"], "url": f"https://{host}/{site}{p['externalPath']}", "snippet": _clean(f"{p.get('locationsText', '')}. {p.get('postedOn', '')}")}
        for p in response.json().get("jobPostings", []) if p.get("title") and p.get("externalPath")
    ]


def recognised(url: str) -> bool:
    host = (urlparse(url).hostname or "").lower()
    return host.endswith("amazon.jobs") or host in {"careers.oracle.com", ORACLE_HOST} or host.endswith(".myworkdayjobs.com")


def search(url: str, query: str) -> list[dict] | None:
    """Jobs from a recognised board, or None when the address is not one (the caller then scrapes the page)."""
    parsed = urlparse(url)
    host = (parsed.hostname or "").lower()
    if not recognised(url):
        return None
    query = query.strip() or "software engineer"
    assert_public_url(url)
    with httpx.Client(timeout=20.0, headers={"User-Agent": USER_AGENT, "Accept": "application/json", "Accept-Encoding": "gzip"}, follow_redirects=False) as client:
        if host.endswith("amazon.jobs"):
            return _amazon(query, client)
        if host.endswith(".myworkdayjobs.com"):
            return _workday(host, parsed.path, query, client)
        return _oracle(query, client)

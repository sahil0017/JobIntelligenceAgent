import http.server
import threading

import pytest

from cortex import portals, scraper
from cortex.embeddings import HashingEmbedder

HTML = """<html><head><script type="application/ld+json">{"@type":"JobPosting","title":"Go Engineer","url":"https://example.com/jobs/go","description":"<p>Build Go services</p>"}</script></head>
<body><a href="/careers/jobs/123-python-developer">Python Developer</a><a href="/about">About</a>
<a href="https://evil.test/jobs/1">Elsewhere Role</a><a href="/jobs/">Jobs</a></body></html>"""


def test_extract_jobs_keeps_same_site_job_links_and_json_ld():
    jobs = portals.extract_jobs(HTML, "https://example.com/careers")
    titles = {j["title"] for j in jobs}
    assert titles == {"Go Engineer", "Python Developer"}


def test_search_reports_auth_required(monkeypatch):
    def blocked(url, parser=None, **_):
        raise scraper.AuthRequired("careers.example.com", 403)

    monkeypatch.setattr(portals, "scrape", blocked)
    monkeypatch.setattr(portals, "assert_public_url", lambda u: u)
    result = portals.search("https://careers.example.com", "", "python", HashingEmbedder())
    assert result["status"] == "auth_required" and result["host"] == "careers.example.com"


def test_search_ranks_jobs(monkeypatch):
    monkeypatch.setattr(portals, "scrape", lambda url, parser=None, **_: (HTML, "https://example.com/careers"))
    monkeypatch.setattr(portals, "assert_public_url", lambda u: u)
    result = portals.search("https://example.com/careers", "", "python developer", HashingEmbedder())
    assert result["status"] == "ok" and result["jobs"][0]["title"] == "Python Developer"


def test_http_403_raises_auth_required(monkeypatch):
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(403)
            self.end_headers()

        def log_message(self, *a):
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    monkeypatch.setattr(scraper, "assert_public_url", lambda u: u)
    import httpx

    with httpx.Client() as client, pytest.raises(scraper.AuthRequired):
        scraper._fetch_http(f"http://127.0.0.1:{server.server_port}/", client)
    server.shutdown()

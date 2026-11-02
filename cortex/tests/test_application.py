import json
import time

import pytest
from fastapi.testclient import TestClient

from cortex.embeddings import HashingEmbedder
from cortex.llm import LLMError, NoLLM
from cortex.mcp_server import Toolbox
from cortex.memory import Memory, chunk_text
from cortex.scraper import ScrapeError, assert_public_url, parse_html
from cortex.server import create_app

RESUME = """Backend Engineer
- Built payment services in Go and PostgreSQL handling 2M requests per day
- Designed Kafka event pipelines and deployed them with Docker on AWS
- Mentored four engineers and led code reviews for the platform team
"""
PROFILE = {"title": "Backend Engineer", "skills": "Go, PostgreSQL, Kafka, Docker, AWS"}
JOB = {
    "id": "j1", "company": "Acme", "role": "Senior Backend Engineer", "tags": ["Go", "Kafka"],
    "description": "Build resilient Go services on PostgreSQL and Kafka. Experience with Docker and AWS is a plus. " * 2,
}


class ScriptedLLM:
    """Answers tailor prompts from a script, then writes plain notes."""

    name = "scripted"
    remote = False

    def __init__(self, tailor_answers):
        self.tailor_answers = list(tailor_answers)

    def complete(self, system, prompt, max_tokens=700):
        if "Rewrite up to" in prompt:
            return self.tailor_answers.pop(0)
        return "Short, honest note.\n\n[Your name]"


class DownLLM:
    name = "down"
    remote = False

    def complete(self, system, prompt, max_tokens=700):
        raise LLMError("offline")


def make_client(llm=None):
    return TestClient(create_app(HashingEmbedder(), llm or NoLLM(), use_browser=False))


def settle(client, run_id, until=("awaiting_approval", "completed", "failed", "cancelled"), timeout=10):
    deadline = time.time() + timeout
    while time.time() < deadline:
        run = client.get(f"/v1/applications/{run_id}").json()
        if run["status"] in until:
            return run
        time.sleep(0.02)
    raise AssertionError(f"run stuck: {run['status']}")


def start(client, job=JOB, **extra):
    body = {"profile": PROFILE, "resumeText": RESUME, "job": job, **extra}
    response = client.post("/v1/applications", json=body)
    assert response.status_code == 202, response.text
    return response.json()["id"]


def test_application_pauses_for_approval_with_grounded_draft():
    client = make_client()
    run = settle(client, start(client))
    assert run["status"] == "awaiting_approval"
    steps = {s["id"]: s for s in run["steps"]}
    assert [steps[i]["status"] for i in ("scrape", "index", "match", "tailor", "verify", "outreach")] == ["done"] * 6
    assert steps["review"]["status"] == "running" and steps["finalize"]["status"] == "pending"
    draft = run["draft"]
    assert draft["status"] == "awaiting_review" and draft["fitScore"] >= 35
    assert draft["tailoredBullets"] and not any(b.startswith("Profile:") for b in draft["tailoredBullets"])
    resume_words = set(RESUME.lower().split())
    assert all(set(b.lower().split()) <= resume_words for b in draft["tailoredBullets"])
    assert any("No language model" in w for w in draft["warnings"])
    assert "[Your name]" in draft["coverNote"]


def test_human_edits_and_approves():
    client = make_client()
    run_id = start(client)
    settle(client, run_id)
    decision = {"approve": True, "tailoredBullets": ["Led payment services in Go"], "coverNote": "Hello!"}
    assert client.post(f"/v1/applications/{run_id}/decision", json=decision).status_code == 202
    run = settle(client, run_id, until=("completed", "failed"))
    assert run["status"] == "completed" and all(s["status"] == "done" for s in run["steps"] if s["id"] != "apply")
    assert [s["status"] for s in run["steps"] if s["id"] == "apply"] == ["pending"]
    assert run["draft"]["status"] == "approved"
    assert run["draft"]["tailoredBullets"] == ["Led payment services in Go"]
    assert run["draft"]["coverNote"] == "Hello!"
    # a finished run cannot be decided twice
    assert client.post(f"/v1/applications/{run_id}/decision", json={"approve": False}).status_code == 409


def test_rejection_keeps_draft_for_reference():
    client = make_client()
    run_id = start(client)
    settle(client, run_id)
    client.post(f"/v1/applications/{run_id}/decision", json={"approve": False})
    run = settle(client, run_id, until=("completed",))
    assert run["draft"]["status"] == "rejected"


def test_weak_fit_skips_drafting_and_review():
    client = make_client()
    job = {"id": "x", "company": "Pixel", "role": "Pastry Chef", "tags": ["Baking"], "description": "We are hiring a pastry chef to bake bread and cakes in our busy bakery. Kitchen experience, early mornings and food safety certificates required."}
    run_id = start(client, job=job)
    run = settle(client, run_id, until=("completed", "failed"))
    assert run["status"] == "completed"
    steps = {s["id"]: s["status"] for s in run["steps"]}
    assert steps["tailor"] == steps["outreach"] == steps["review"] == "skipped" and steps["finalize"] == "done"
    assert run["draft"]["status"] == "not_recommended" and run["draft"]["tailoredBullets"] == []
    # the user can still ask for a draft
    forced = settle(client, start(client, job=job, draftAnyway=True))
    assert forced["status"] == "awaiting_approval"


def test_verify_sends_invented_claims_back_to_the_tailor():
    invented = "- Ran Kubernetes clusters serving 99.99% uptime for payments\n- Built payment services in Go and PostgreSQL"
    honest = "- Built payment services in Go and PostgreSQL handling 2M requests per day"
    client = make_client(ScriptedLLM([invented, honest]))
    run = settle(client, start(client))
    assert run["status"] == "awaiting_approval"
    tailor = next(s for s in run["steps"] if s["id"] == "tailor")
    assert tailor["detail"].startswith("attempt 2")
    assert any("sending back to the tailor" in t["message"] for t in run["trace"])
    assert not any("Kubernetes" in b for b in run["draft"]["tailoredBullets"])


def test_unsupported_claims_are_removed_when_the_model_keeps_inventing():
    invented = "- Ran Kubernetes clusters serving 99.99% uptime for payments\n- Built payment services in Go and PostgreSQL"
    client = make_client(ScriptedLLM([invented] * 3))
    run = settle(client, start(client))
    assert run["draft"]["tailoredBullets"] == ["Built payment services in Go and PostgreSQL"]
    assert any("unsupported claim" in w for w in run["draft"]["warnings"])


def test_unreachable_model_falls_back_to_resume_passages():
    client = make_client(DownLLM())
    run = settle(client, start(client))
    assert run["status"] == "awaiting_approval"
    assert run["draft"]["writer"].startswith("template")


def test_private_page_addresses_are_refused_and_description_is_used():
    client = make_client()
    run = settle(client, start(client, url="http://127.0.0.1/api/profile"))
    scrape = next(s for s in run["steps"] if s["id"] == "scrape")
    assert scrape["status"] == "done" and "unreadable" in scrape["detail"]
    assert any("private or local network" in w for w in run["draft"]["warnings"])


def test_missing_resume_fails_the_run_with_a_clear_message():
    client = make_client()
    response = client.post("/v1/applications", json={"profile": {}, "resumeText": "", "job": JOB})
    run = settle(client, response.json()["id"])
    assert run["status"] == "failed" and "résumé" in run["error"]
    assert next(s for s in run["steps"] if s["id"] == "index")["status"] == "failed"


def test_cancel_while_waiting_for_approval():
    client = make_client()
    run_id = start(client)
    settle(client, run_id)
    assert client.post(f"/v1/applications/{run_id}/cancel").status_code == 202
    run = client.get(f"/v1/applications/{run_id}").json()
    assert run["status"] == "cancelled"
    assert client.post(f"/v1/applications/{run_id}/decision", json={"approve": True}).status_code == 409


def test_unknown_run_is_404_and_pipeline_lists_graph_edges():
    client = make_client()
    assert client.get("/v1/applications/nope").status_code == 404
    steps = client.get("/v1/application-pipeline").json()
    assert [s["id"] for s in steps][-3:] == ["review", "finalize", "apply"]
    assert next(s for s in steps if s["id"] == "finalize")["after"] == ["review", "match"]


def test_scraper_rejects_unsafe_addresses():
    for url in ("file:///etc/passwd", "ftp://example.com/x", "http://localhost/x", "http://10.0.0.5/x", "http://169.254.169.254/latest", "http://127.0.0.1:8890/"):
        with pytest.raises(ScrapeError):
            assert_public_url(url)


def test_scraper_prefers_structured_job_posting():
    posting = {"@type": "JobPosting", "title": "Go Engineer", "hiringOrganization": {"name": "Acme"}, "description": "<p>Build <b>Go</b> services.</p><ul><li>Kafka</li></ul>"}
    html = f"<html><title>Careers</title><script type='application/ld+json'>{json.dumps(posting)}</script><body>cookie banner</body></html>"
    page = parse_html(html, "https://acme.test/jobs/1", "http")
    assert page.structured and page.title == "Go Engineer at Acme"
    assert "Build Go services." in page.text and "cookie" not in page.text


def test_scraper_extracts_visible_text_and_skips_chrome():
    body = "<nav>Home Jobs</nav><main><h1>Platform Engineer</h1><p>" + "We build reliable systems with Go and Kafka. " * 4 + "</p></main><script>evil()</script>"
    page = parse_html(f"<html><title>Jobs</title><body>{body}</body></html>", "https://x.test", "http")
    assert "Platform Engineer" in page.text and "evil" not in page.text and "Home Jobs" not in page.text
    with pytest.raises(ScrapeError):
        parse_html("<html><body>Login</body></html>", "https://x.test", "http")


def test_memory_retrieves_the_most_relevant_resume_passage():
    memory = Memory(HashingEmbedder())
    chunks = chunk_text(RESUME)
    assert len(chunks) >= 3
    name = memory.index(chunks)
    assert memory.index(chunks) == name  # idempotent
    top = memory.search(name, "Kafka event streaming pipelines", k=2)
    assert "Kafka" in top[0].text and top[0].score >= top[1].score


def test_mcp_toolbox_matches_and_refuses_unknown_profile_fields():
    box = Toolbox(platform_url="http://127.0.0.1:1")
    result = box.match_job(JOB["description"], RESUME, "Go, Kafka")
    assert result["fitScore"] > 35 and "Go" in result["matchedSkills"]
    assert box.parse_job_description("We use Go and Kafka")["skills"] == ["Go", "Kafka"]
    with pytest.raises(ValueError):
        box.update_profile(email="x@y.z")
    assert not hasattr(box, "approve_application")


def test_chunking_splits_flattened_pdf_text_and_drops_contact_details():
    from cortex.memory import chunk_text

    flat = (
        "SAHIL Mobile: +91 - 7022299648 • Email: a@b.com LinkedIn: https://x.com/in/y "
        "• Built payment pipelines in Go and Kafka handling 2M events a day. "
        "• Reduced latency by 40% using PostgreSQL indexing and caching."
    )
    chunks = chunk_text(flat)
    assert len(chunks) == 2
    assert not any("@" in chunk or "http" in chunk for chunk in chunks)


class SearchingLLM(ScriptedLLM):
    def __init__(self):
        super().__init__([])
        self.queries = []

    def complete(self, system, prompt, max_tokens=700):
        if "Rewrite up to" in prompt:
            raise LLMError("no tailoring in this test")
        return super().complete(system, prompt, max_tokens)

    def research(self, query, max_tokens=500):
        self.queries.append(query)
        return "Acme builds payment infrastructure."


def test_research_agent_sends_only_company_and_role_and_feeds_the_draft():
    llm = SearchingLLM()
    client = make_client(llm)
    run = settle(client, start(client))
    research = next(s for s in run["steps"] if s["id"] == "research")
    assert research["status"] == "done" and research["agent"] == "Research agent"
    assert run["draft"]["research"] == "Acme builds payment infrastructure."
    assert len(llm.queries) == 1 and "Acme" in llm.queries[0]
    assert "Go" not in llm.queries[0].split() and "2M" not in llm.queries[0]


def test_research_is_skipped_without_a_web_search_model():
    client = make_client()
    run = settle(client, start(client))
    research = next(s for s in run["steps"] if s["id"] == "research")
    assert research["status"] == "done" and "skipped" in research["detail"]
    assert run["draft"]["research"] == ""


def test_openai_web_search_request_shape_and_parsing(monkeypatch):
    import httpx

    from cortex.llm import OpenAILLM

    seen = {}

    def fake_post(url, headers, json, timeout):
        seen.update(url=url, json=json, timeout=timeout)
        body = {"output": [{"type": "web_search_call"}, {"type": "message", "content": [{"type": "output_text", "text": "Acme does X."}]}]}
        return httpx.Response(200, json=body, request=httpx.Request("POST", url))

    monkeypatch.setattr(httpx, "post", fake_post)
    text = OpenAILLM("test-key", "gpt-4o-mini").research("Acme?")
    assert text == "Acme does X."
    assert seen["url"].endswith("/responses") and seen["json"]["tools"] == [{"type": "web_search"}]


class TeamLLM(ScriptedLLM):
    remote = True

    def __init__(self):
        super().__init__([])

    def complete(self, system, prompt, max_tokens=700):
        if "Rewrite up to" in prompt:
            raise LLMError("template bullets in this test")
        if "positioning" in prompt or "strongest angles" in prompt:
            return "Lead with Go services."
        if "strict recruiter" in prompt:
            return "Tighten the opening."
        return "Short, honest note.\n\n[Your name]"

    def research(self, query, max_tokens=500):
        return "Acme builds payments."


def test_specialist_agents_run_and_fill_the_draft():
    client = make_client(TeamLLM())
    run = settle(client, start(client))
    steps = {s["id"]: s for s in run["steps"]}
    for agent in ("research", "strategy", "tailor", "verify", "outreach", "critic"):
        assert steps[agent]["status"] == "done", agent
    assert run["draft"]["strategy"] == "Lead with Go services."
    assert run["draft"]["critique"] == "Tighten the opening."
    assert run["draft"]["research"] == "Acme builds payments."


def test_key_expiry_countdown_only_for_hosted_models(monkeypatch):
    from datetime import date, timedelta

    soon = (date.today() + timedelta(days=5)).isoformat()
    monkeypatch.setenv("CORTEX_KEY_EXPIRES", soon)
    assert make_client(TeamLLM()).get("/health").json()["keyExpiresInDays"] in (4, 5)
    assert make_client().get("/health").json()["keyExpiresInDays"] is None


def test_apply_only_after_approval_and_never_submits(monkeypatch):
    from cortex import application

    opened = {}

    def fake_open(url, applicant, on_ready, on_error):
        opened["url"], opened["name"], opened["cover"] = url, applicant.name, applicant.cover_note
        on_ready(application.ApplyResult(filled=["email"]))

    monkeypatch.setattr(application, "open_and_prefill", fake_open)
    client = make_client()
    run_id = start(client, url="https://example.com/job")
    settle(client, run_id)
    assert client.post(f"/v1/applications/{run_id}/apply", json={}).status_code == 409  # not approved yet
    client.post(f"/v1/applications/{run_id}/decision", json={"approve": True, "coverNote": "Hi"})
    settle(client, run_id, until=("completed",))
    response = client.post(f"/v1/applications/{run_id}/apply", json={"name": "Ada Lovelace", "email": "a@b.co"})
    assert response.status_code == 202
    time.sleep(0.3)
    run = client.get(f"/v1/applications/{run_id}").json()
    apply_step = next(s for s in run["steps"] if s["id"] == "apply")
    assert apply_step["status"] == "done" and "press Submit yourself" in apply_step["detail"]
    assert opened == {"url": "https://example.com/job", "name": "Ada Lovelace", "cover": "Hi"}

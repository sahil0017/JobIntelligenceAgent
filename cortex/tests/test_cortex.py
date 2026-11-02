import time

import pytest
from fastapi.testclient import TestClient

from cortex.embeddings import HashingEmbedder
from cortex.llm import NoLLM
from cortex.orchestrator import Orchestrator, Step, Workflow
from cortex.server import create_app

PROFILE = {"title": "Backend Engineer", "skills": "Go, PostgreSQL, Kafka, Docker, AWS", "roles": "Backend Engineer"}
JOBS = [
    {"id": "a", "company": "Acme", "role": "Senior Backend Engineer", "tags": ["Go", "Kafka", "PostgreSQL", "Kubernetes"]},
    {"id": "b", "company": "Pixel", "role": "iOS Designer", "tags": ["Swift", "Figma"], "description": "Design mobile interfaces."},
]


def wait(client, run_id, timeout=5):
    deadline = time.time() + timeout
    while time.time() < deadline:
        run = client.get(f"/v1/analyses/{run_id}").json()
        if run["status"] != "running":
            return run
        time.sleep(0.02)
    raise AssertionError("run did not finish")


@pytest.fixture()
def client():
    return TestClient(create_app(HashingEmbedder(), NoLLM(), use_browser=False))


def test_health_and_pipeline(client):
    health = client.get("/health").json()
    assert health == {
        "status": "ok", "engine": "hashing-ngrams", "deepLearning": False, "llm": "none (templates)",
        "llmRemote": False, "scraper": "http", "memory": "chromadb", "tracing": False, "keyExpiresInDays": None,
    }
    steps = client.get("/v1/pipeline").json()
    assert [s["id"] for s in steps] == ["prepare", "extract", "embed", "match", "gaps", "rank", "summarise"]


def test_analysis_ranks_relevant_job_first_and_reports_gaps(client):
    run = client.post("/v1/analyses", json={"profile": PROFILE, "resumeText": "", "jobs": JOBS}).json()
    finished = wait(client, run["id"])
    assert finished["status"] == "completed", finished
    assert all(step["status"] == "done" for step in finished["steps"])
    result = finished["result"]
    top, other = result["jobs"]
    assert top["id"] == "a" and top["rank"] == 1 and other["rank"] == 2
    assert top["fitScore"] > other["fitScore"]
    assert top["skills"]["missing"] == ["Kubernetes"]
    assert other["skillCoverage"] is None  # too few recognised skills to trust a coverage number
    assert result["learningPriorities"][0] == {"skill": "Kubernetes", "jobs": 1}
    assert any(entry["stage"] == "embed" for entry in finished["trace"])


def test_empty_candidate_fails_with_clear_error_and_skips_the_rest(client):
    run = client.post("/v1/analyses", json={"profile": {}, "resumeText": "", "jobs": JOBS}).json()
    finished = wait(client, run["id"])
    assert finished["status"] == "failed"
    assert "profile skills or upload a resume" in finished["error"]
    statuses = [step["status"] for step in finished["steps"]]
    assert statuses[0] == "failed" and set(statuses[1:]) == {"skipped"}


def test_validation_and_not_found(client):
    assert client.post("/v1/analyses", json={"profile": PROFILE, "jobs": []}).status_code == 422
    assert client.get("/v1/analyses/missing").status_code == 404
    assert client.post("/v1/analyses/missing/cancel").status_code == 404


def test_cancel_stops_between_steps():
    def slow(ctx):
        time.sleep(0.2)

    workflow = Workflow([Step("one", "One", "", "prepare", slow), Step("two", "Two", "", "model", slow)])
    orchestrator = Orchestrator(workflow)
    run = orchestrator.submit({})
    orchestrator.cancel(run.id)
    deadline = time.time() + 3
    while orchestrator.get(run.id).status == "running" and time.time() < deadline:
        time.sleep(0.02)
    finished = orchestrator.get(run.id)
    assert finished.status == "cancelled"
    assert finished.steps[1].status == "skipped"


def test_duplicate_step_ids_rejected():
    step = Step("x", "X", "", "prepare", lambda ctx: None)
    with pytest.raises(ValueError):
        Workflow([step, step])

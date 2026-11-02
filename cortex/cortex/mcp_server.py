"""Model Context Protocol server: exposes CareerPilot's job-search actions as typed tools.

Run over stdio so an MCP client (Claude Desktop, VS Code, an agent framework) can call them:

    cortex/.venv/bin/python -m cortex.mcp_server

There is deliberately no "approve" or "send" tool. An external agent can draft an application and
read the draft, but only the person in the console can approve it.
"""

from __future__ import annotations

import os
from typing import Any

import httpx
from mcp.server.fastmcp import FastMCP

from .application import ApplicationRuns
from .embeddings import build_embedder
from .llm import build_llm
from .memory import Memory, chunk_text
from .pipeline import MIN_JOB_SKILLS, SEMANTIC_WEIGHT, verdict_for
from .schemas import ApplicationRequest, CandidateProfile, JobInput
from .skills import extract_skills

PROFILE_FIELDS = ("title", "skills", "roles", "experience")


class Toolbox:
    """The tool implementations, separate from the MCP wiring so they are easy to test."""

    def __init__(self, platform_url: str | None = None):
        self.embedder = build_embedder()
        self.memory = Memory(self.embedder)
        self.runs = ApplicationRuns(self.embedder, build_llm(), self.memory, use_browser=os.environ.get("CORTEX_SCRAPER") != "http")
        self.platform_url = (platform_url or os.environ.get("CAREERPILOT_API_URL", "http://127.0.0.1:8787")).rstrip("/")

    def parse_job_description(self, text: str) -> dict[str, Any]:
        skills = extract_skills(text)
        return {"skills": skills, "wordCount": len(text.split())}

    def match_job(self, job_text: str, resume_text: str, profile_skills: str = "") -> dict[str, Any]:
        vectors = self.embedder.embed([f"{profile_skills} {resume_text}".strip(), job_text])
        semantic = self.embedder.calibrate(float(vectors[0] @ vectors[1]))
        have = {s.lower() for s in extract_skills(f"{profile_skills} {resume_text}")}
        wanted = extract_skills(job_text)
        matched = [s for s in wanted if s.lower() in have]
        coverage = round(100 * len(matched) / len(wanted)) if wanted else 0
        fit = round(SEMANTIC_WEIGHT * semantic + (1 - SEMANTIC_WEIGHT) * coverage) if len(wanted) >= MIN_JOB_SKILLS else semantic
        return {
            "fitScore": fit, "verdict": verdict_for(fit), "semanticScore": semantic, "skillCoverage": coverage,
            "matchedSkills": matched, "missingSkills": [s for s in wanted if s.lower() not in have], "engine": self.embedder.name,
        }

    def search_resume(self, resume_text: str, query: str, limit: int = 5) -> list[dict[str, Any]]:
        chunks = chunk_text(resume_text)
        if not chunks:
            return []
        name = self.memory.index(chunks)
        return [{"text": e.text, "score": e.score} for e in self.memory.search(name, query, k=max(1, min(limit, 10)))]

    def draft_application(self, role: str, company: str, resume_text: str, job_text: str = "", url: str = "", skills: str = "") -> dict[str, Any]:
        request = ApplicationRequest(
            profile=CandidateProfile(skills=skills), resume_text=resume_text, url=url,
            job=JobInput(id="mcp", role=role, company=company, description=job_text),
        )
        run = self.runs.submit(request)
        return {"runId": run.id, "status": run.status, "note": "Poll get_application. A person must approve the draft in the CareerPilot console."}

    def get_application(self, run_id: str) -> dict[str, Any]:
        run = self.runs.get(run_id)
        return run.model_dump(by_alias=True, mode="json")

    def update_profile(self, **changes: str) -> dict[str, Any]:
        unknown = set(changes) - set(PROFILE_FIELDS)
        if unknown:
            raise ValueError(f"unsupported profile fields: {', '.join(sorted(unknown))}")
        with httpx.Client(base_url=self.platform_url, timeout=10.0) as client:
            current = client.get("/api/profile")
            current.raise_for_status()
            merged = {**current.json(), **{k: v for k, v in changes.items() if v is not None}}
            saved = client.put("/api/profile", json=merged)
            saved.raise_for_status()
            return saved.json()


def build_server(toolbox: Toolbox | None = None) -> FastMCP:
    box = toolbox or Toolbox()
    server = FastMCP("careerpilot")

    @server.tool()
    def parse_job_description(text: str) -> dict[str, Any]:
        """Extract the skills a job description asks for."""
        return box.parse_job_description(text)

    @server.tool()
    def match_job(job_text: str, resume_text: str, profile_skills: str = "") -> dict[str, Any]:
        """Score how well a résumé fits a job: fit score, verdict, matched and missing skills."""
        return box.match_job(job_text, resume_text, profile_skills)

    @server.tool()
    def search_resume(resume_text: str, query: str, limit: int = 5) -> list[dict[str, Any]]:
        """Find the résumé passages most relevant to a query (semantic search)."""
        return box.search_resume(resume_text, query, limit)

    @server.tool()
    def draft_application(role: str, company: str, resume_text: str, job_text: str = "", url: str = "", skills: str = "") -> dict[str, Any]:
        """Start drafting a tailored application. Returns a run id; a human must approve the draft."""
        return box.draft_application(role, company, resume_text, job_text, url, skills)

    @server.tool()
    def get_application(run_id: str) -> dict[str, Any]:
        """Read the status, steps and draft of an application run."""
        return box.get_application(run_id)

    @server.tool()
    def update_profile(title: str | None = None, skills: str | None = None, roles: str | None = None, experience: str | None = None) -> dict[str, Any]:
        """Update fields of the saved CareerPilot profile (needs the platform running)."""
        return box.update_profile(title=title, skills=skills, roles=roles, experience=experience)

    return server


if __name__ == "__main__":
    build_server().run()

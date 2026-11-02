"""Wire types shared with the Go platform. JSON uses camelCase to match the rest of the API."""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel


class Wire(BaseModel):
    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True)


class CandidateProfile(Wire):
    title: str = Field(default="", max_length=500)
    skills: str = Field(default="", max_length=5000)
    roles: str = Field(default="", max_length=2000)
    experience: str = Field(default="", max_length=200)


class JobInput(Wire):
    id: str = Field(max_length=200)
    company: str = Field(default="", max_length=300)
    role: str = Field(max_length=500)
    location: str = Field(default="", max_length=500)
    tags: list[str] = Field(default_factory=list, max_length=40)
    description: str = Field(default="", max_length=20000)


class AnalysisRequest(Wire):
    profile: CandidateProfile = Field(default_factory=CandidateProfile)
    resume_text: str = Field(default="", max_length=60000)
    jobs: list[JobInput] = Field(min_length=1, max_length=100)


class SkillGap(Wire):
    matched: list[str]
    missing: list[str]
    coverage: int


class JobAnalysis(Wire):
    id: str
    company: str
    role: str
    rank: int
    fit_score: int
    semantic_score: int
    skill_coverage: int | None
    skills: SkillGap
    verdict: Literal["strong", "good", "stretch", "weak"]


class LearningPriority(Wire):
    skill: str
    jobs: int


class AnalysisResult(Wire):
    engine: str
    jobs: list[JobAnalysis]
    candidate_skills: list[str]
    learning_priorities: list[LearningPriority]
    summary: str


StepStatus = Literal["pending", "running", "done", "skipped", "failed"]
RunStatus = Literal["running", "awaiting_approval", "completed", "failed", "cancelled"]


class RunStep(Wire):
    id: str
    title: str
    description: str
    status: StepStatus = "pending"
    detail: str = ""
    duration_ms: int = 0
    kind: str = ""
    agent: str = ""
    after: list[str] = Field(default_factory=list)  # edges of the workflow graph, for the live graph view


class TraceEntry(Wire):
    time: str
    stage: str
    message: str


class ApplicationRequest(Wire):
    profile: CandidateProfile = Field(default_factory=CandidateProfile)
    resume_text: str = Field(default="", max_length=60000)
    job: JobInput
    url: str = Field(default="", max_length=2000)
    draft_anyway: bool = False


class ApplyRequest(Wire):
    name: str = Field(default="", max_length=200)
    email: str = Field(default="", max_length=200)
    phone: str = Field(default="", max_length=60)
    location: str = Field(default="", max_length=200)


class QuickApplyRequest(ApplyRequest):
    url: str = Field(max_length=2000)


class QuickApplyResult(Wire):
    status: str
    filled: list[str] = Field(default_factory=list)
    message: str = ""


class PortalSearchRequest(Wire):
    url: str = Field(max_length=2000)
    keywords: str = Field(default="", max_length=300)
    profile: CandidateProfile = Field(default_factory=CandidateProfile)
    resume_text: str = Field(default="", max_length=60000)


class PortalSignInRequest(Wire):
    url: str = Field(max_length=2000)


class PortalJob(Wire):
    title: str
    url: str
    snippet: str = ""
    score: int = 0
    skills: list[str] = Field(default_factory=list)


class PortalResult(Wire):
    status: Literal["ok", "auth_required"]
    host: str = ""
    http_status: int = 0
    message: str = ""
    jobs: list[PortalJob] = Field(default_factory=list)


class PortalSession(Wire):
    signed_in: bool
    window_open: bool = False
    message: str = ""


class Draft(Wire):
    status: Literal["awaiting_review", "approved", "rejected", "not_recommended"] = "awaiting_review"
    company: str = ""
    role: str = ""
    fit_score: int = 0
    verdict: str = ""
    skills: SkillGap | None = None
    tailored_bullets: list[str] = Field(default_factory=list)
    cover_note: str = ""
    outreach_message: str = ""
    warnings: list[str] = Field(default_factory=list)
    sources: list[str] = Field(default_factory=list)  # résumé passages the draft is grounded in
    page_title: str = ""
    page_method: str = ""
    writer: str = ""
    research: str = ""
    strategy: str = ""
    critique: str = ""


class Decision(Wire):
    approve: bool
    tailored_bullets: list[str] | None = Field(default=None, max_length=12)
    cover_note: str | None = Field(default=None, max_length=4000)
    outreach_message: str | None = Field(default=None, max_length=3000)


class Run(Wire):
    id: str
    kind: Literal["analysis", "application"] = "analysis"
    title: str = ""
    status: RunStatus = "running"
    started_at: str
    finished_at: str | None = None
    job_count: int = 0
    steps: list[RunStep]
    trace: list[TraceEntry] = Field(default_factory=list)
    result: AnalysisResult | None = None
    draft: Draft | None = None
    error: str = ""


class PipelineStep(Wire):
    id: str
    title: str
    description: str
    kind: str
    agent: str = ""
    after: list[str] = Field(default_factory=list)


class Health(Wire):
    status: str
    engine: str
    deep_learning: bool
    llm: str = "none"
    llm_remote: bool = False
    scraper: str = "http"
    memory: str = "chromadb"
    tracing: bool = False
    key_expires_in_days: int | None = None

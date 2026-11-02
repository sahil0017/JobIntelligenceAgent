"""The job-fit analysis workflow: prepare, embed, match, find skill gaps, rank, summarise."""

from __future__ import annotations

from collections import Counter

from .embeddings import Embedder
from .orchestrator import Step, StepContext, Workflow
from .schemas import (
    AnalysisRequest,
    AnalysisResult,
    JobAnalysis,
    LearningPriority,
    SkillGap,
)
from .skills import extract_skills

# Skill coverage is only trusted when a posting names enough skills to compare against.
MIN_JOB_SKILLS = 2
SEMANTIC_WEIGHT = 0.6


def verdict_for(score: int) -> str:
    if score >= 75:
        return "strong"
    if score >= 55:
        return "good"
    if score >= 35:
        return "stretch"
    return "weak"


def build_workflow(embedder: Embedder) -> Workflow:
    def prepare(ctx: StepContext) -> str:
        request: AnalysisRequest = ctx.state["request"]
        candidate = " ".join(
            part
            for part in (
                request.profile.title,
                request.profile.roles,
                request.profile.skills,
                request.resume_text,
            )
            if part
        ).strip()
        if not candidate:
            raise ValueError("add profile skills or upload a resume before running an analysis")
        ctx.state["candidate_text"] = candidate
        ctx.state["job_texts"] = [
            " ".join([job.role, job.company, job.location, " ".join(job.tags), job.description]).strip()
            for job in request.jobs
        ]
        ctx.log("prepare", f"candidate document has {len(candidate.split())} words")
        return f"{len(request.jobs)} jobs, resume {'included' if request.resume_text else 'not provided'}"

    def extract(ctx: StepContext) -> str:
        request: AnalysisRequest = ctx.state["request"]
        profile_text = " ".join([request.profile.skills, request.profile.title, request.profile.roles])
        candidate_skills = extract_skills(profile_text + " " + request.resume_text)
        ctx.state["candidate_skills"] = candidate_skills
        ctx.state["job_skills"] = [extract_skills(text) for text in ctx.state["job_texts"]]
        ctx.log("extract", "candidate skills: " + (", ".join(candidate_skills) or "none recognised"))
        return f"{len(candidate_skills)} candidate skills recognised"

    def embed(ctx: StepContext) -> str:
        ctx.checkpoint()
        ctx.log("embed", f"encoding with {embedder.name}")
        vectors = embedder.embed([ctx.state["candidate_text"], *ctx.state["job_texts"]])
        ctx.state["candidate_vector"] = vectors[0]
        ctx.state["job_vectors"] = vectors[1:]
        return f"{embedder.name}, {vectors.shape[1]} dimensions"

    def match(ctx: StepContext) -> str:
        ctx.checkpoint()
        similarities = ctx.state["job_vectors"] @ ctx.state["candidate_vector"]
        ctx.state["semantic"] = [embedder.calibrate(float(value)) for value in similarities]
        ctx.log("match", f"cosine range {similarities.min():.2f} to {similarities.max():.2f}")
        return f"scored {len(similarities)} jobs by cosine similarity"

    def gaps(ctx: StepContext) -> str:
        have = {skill.lower() for skill in ctx.state["candidate_skills"]}
        results: list[SkillGap | None] = []
        for skills in ctx.state["job_skills"]:
            matched = [skill for skill in skills if skill.lower() in have]
            missing = [skill for skill in skills if skill.lower() not in have]
            coverage = round(100 * len(matched) / len(skills)) if skills else 0
            results.append(SkillGap(matched=matched, missing=missing, coverage=coverage))
        ctx.state["gaps"] = results
        comparable = sum(1 for skills in ctx.state["job_skills"] if len(skills) >= MIN_JOB_SKILLS)
        ctx.log("gaps", f"{comparable} postings name enough skills for a coverage comparison")
        return f"{comparable} of {len(results)} postings comparable"

    def rank(ctx: StepContext) -> str:
        request: AnalysisRequest = ctx.state["request"]
        rows: list[JobAnalysis] = []
        for index, job in enumerate(request.jobs):
            semantic = ctx.state["semantic"][index]
            gap: SkillGap = ctx.state["gaps"][index]
            comparable = len(ctx.state["job_skills"][index]) >= MIN_JOB_SKILLS
            fit = round(SEMANTIC_WEIGHT * semantic + (1 - SEMANTIC_WEIGHT) * gap.coverage) if comparable else semantic
            rows.append(
                JobAnalysis(
                    id=job.id,
                    company=job.company,
                    role=job.role,
                    rank=0,
                    fit_score=fit,
                    semantic_score=semantic,
                    skill_coverage=gap.coverage if comparable else None,
                    skills=gap,
                    verdict=verdict_for(fit),  # type: ignore[arg-type]
                )
            )
        rows.sort(key=lambda row: (-row.fit_score, row.role.lower()))
        for position, row in enumerate(rows, start=1):
            row.rank = position
        ctx.state["ranked"] = rows
        ctx.log("rank", f"top fit {rows[0].fit_score}: {rows[0].role} at {rows[0].company}")
        return f"best fit {rows[0].fit_score}%"

    def summarise(ctx: StepContext) -> str:
        rows: list[JobAnalysis] = ctx.state["ranked"]
        missing = Counter(skill for row in rows[:20] for skill in row.skills.missing)
        priorities = [LearningPriority(skill=skill, jobs=count) for skill, count in missing.most_common(6)]
        strong = sum(1 for row in rows if row.verdict in ("strong", "good"))
        parts = [f"{strong} of {len(rows)} roles are a strong or good fit."]
        if priorities:
            top = ", ".join(f"{p.skill} ({p.jobs})" for p in priorities[:3])
            parts.append(f"Most requested skills you are missing: {top}.")
        ctx.state["result"] = AnalysisResult(
            engine=embedder.name,
            jobs=rows,
            candidate_skills=ctx.state["candidate_skills"],
            learning_priorities=priorities,
            summary=" ".join(parts),
        )
        return f"{strong} strong or good fits, {len(priorities)} learning priorities"

    return Workflow(
        [
            Step("prepare", "Prepare inputs", "Combine your profile and résumé into one candidate document and one document per job.", "prepare", prepare),
            Step("extract", "Extract skills", "Recognise skills in your profile and each posting from a curated vocabulary.", "prepare", extract),
            Step("embed", "Embed documents", "Encode every document into a vector with the configured language model.", "model", embed),
            Step("match", "Semantic matching", "Score each job by cosine similarity between its vector and yours.", "model", match),
            Step("gaps", "Skill gap analysis", "Compare skills you have against the skills each job asks for.", "reason", gaps),
            Step("rank", "Rank opportunities", "Blend semantic similarity and skill coverage into one fit score.", "reason", rank),
            Step("summarise", "Summarise findings", "Aggregate missing skills across the best roles into learning priorities.", "reason", summarise),
        ]
    )


def to_state(request: AnalysisRequest) -> dict:
    return {"request": request}

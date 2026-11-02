"""HTTP interface of the Cortex service. The Go platform is its only intended client."""

from __future__ import annotations

import os
from datetime import date, datetime, timezone

from fastapi import FastAPI, HTTPException

from .apply import Applicant, open_quick
from .application import ApplicationRuns, describe_pipeline
from .embeddings import Embedder, build_embedder
from .llm import LLM, build_llm
from .memory import Memory
from .orchestrator import Orchestrator
from . import portals
from .pipeline import build_workflow, to_state
from .schemas import AnalysisRequest, ApplicationRequest, ApplyRequest, Decision, Health, PipelineStep, PortalResult, PortalSearchRequest, PortalSession, PortalSignInRequest, QuickApplyRequest, QuickApplyResult, Run
from .scraper import ScrapeError
from .tracing import setup_tracing


def create_app(
    embedder: Embedder | None = None,
    llm: LLM | None = None,
    memory: Memory | None = None,
    use_browser: bool | None = None,
    enable_tracing: bool = False,
) -> FastAPI:
    engine = embedder or build_embedder()
    model = llm or build_llm()
    store = memory or Memory(engine)
    browser = use_browser if use_browser is not None else os.environ.get("CORTEX_SCRAPER", "playwright") != "http"
    tracing = setup_tracing() if enable_tracing else False
    workflow = build_workflow(engine)
    orchestrator = Orchestrator(workflow)
    applications = ApplicationRuns(engine, model, store, use_browser=browser)
    app = FastAPI(title="CareerPilot Cortex", version="0.2.0", docs_url=None, redoc_url=None)

    def not_found(kind: str) -> HTTPException:
        return HTTPException(status_code=404, detail=f"{kind} run not found")

    def key_days_left() -> int | None:
        """Days until the hosted-model key expires, from CORTEX_KEY_EXPIRES (YYYY-MM-DD); None if unset."""
        raw = os.environ.get("CORTEX_KEY_EXPIRES", "").strip()
        if not raw or not model.remote:
            return None
        try:
            return (date.fromisoformat(raw[:10]) - datetime.now(timezone.utc).date()).days
        except ValueError:
            return None

    @app.get("/health", response_model=Health, response_model_by_alias=True)
    def health() -> Health:
        return Health(
            key_expires_in_days=key_days_left(),
            status="ok", engine=engine.name, deep_learning=engine.deep_learning, llm=model.name,
            llm_remote=model.remote, scraper="playwright" if browser else "http", tracing=tracing,
        )

    @app.get("/v1/pipeline", response_model=list[PipelineStep], response_model_by_alias=True)
    def pipeline() -> list[PipelineStep]:
        return workflow.describe()

    @app.post("/v1/analyses", response_model=Run, response_model_by_alias=True, status_code=202)
    def start(request: AnalysisRequest) -> Run:
        return orchestrator.submit(to_state(request), job_count=len(request.jobs))

    @app.get("/v1/analyses", response_model=list[Run], response_model_by_alias=True)
    def list_runs() -> list[Run]:
        return orchestrator.list()

    @app.get("/v1/analyses/{run_id}", response_model=Run, response_model_by_alias=True)
    def get_run(run_id: str) -> Run:
        try:
            return orchestrator.get(run_id)
        except KeyError:
            raise not_found("analysis") from None

    @app.post("/v1/analyses/{run_id}/cancel", status_code=202)
    def cancel(run_id: str) -> dict[str, str]:
        try:
            orchestrator.cancel(run_id)
        except KeyError:
            raise not_found("analysis") from None
        return {"status": "cancelling"}

    @app.get("/v1/application-pipeline", response_model=list[PipelineStep], response_model_by_alias=True)
    def application_pipeline() -> list[PipelineStep]:
        return describe_pipeline()

    @app.post("/v1/applications", response_model=Run, response_model_by_alias=True, status_code=202)
    def start_application(request: ApplicationRequest) -> Run:
        return applications.submit(request)

    @app.get("/v1/applications", response_model=list[Run], response_model_by_alias=True)
    def list_applications() -> list[Run]:
        return applications.list()

    @app.get("/v1/applications/{run_id}", response_model=Run, response_model_by_alias=True)
    def get_application(run_id: str) -> Run:
        try:
            return applications.get(run_id)
        except KeyError:
            raise not_found("application") from None

    @app.post("/v1/applications/{run_id}/decision", response_model=Run, response_model_by_alias=True, status_code=202)
    def decide_application(run_id: str, decision: Decision) -> Run:
        try:
            return applications.decide(run_id, decision)
        except KeyError:
            raise not_found("application") from None
        except ValueError as error:
            raise HTTPException(status_code=409, detail=str(error)) from None

    @app.post("/v1/applications/{run_id}/apply", response_model=Run, response_model_by_alias=True, status_code=202)
    def apply_application(run_id: str, request: ApplyRequest) -> Run:
        try:
            return applications.apply(run_id, Applicant(request.name, request.email, request.phone, request.location))
        except KeyError:
            raise not_found("application") from None
        except ValueError as error:
            raise HTTPException(status_code=409, detail=str(error)) from None

    @app.post("/v1/apply", response_model=QuickApplyResult, response_model_by_alias=True)
    def quick_apply(request: QuickApplyRequest) -> QuickApplyResult:
        applicant = Applicant(request.name, request.email, request.phone, request.location)
        try:
            return QuickApplyResult(**open_quick(request.url, applicant))
        except ScrapeError as error:
            raise HTTPException(status_code=422, detail=str(error)) from None
        except ValueError as error:
            raise HTTPException(status_code=409, detail=str(error)) from None

    @app.post("/v1/applications/{run_id}/cancel", status_code=202)
    def cancel_application(run_id: str) -> dict[str, str]:
        try:
            applications.cancel(run_id)
        except KeyError:
            raise not_found("application") from None
        return {"status": "cancelling"}

    signin = portals.SignIn()

    def session(message: str = "") -> PortalSession:
        state = signin.status()
        return PortalSession(signed_in=state["signedIn"], window_open=state["windowOpen"], message=message)

    @app.post("/v1/portals/search", response_model=PortalResult, response_model_by_alias=True)
    def portal_search(request: PortalSearchRequest) -> PortalResult:
        profile = request.profile
        candidate = " ".join(p for p in (profile.title, profile.roles, profile.skills, request.resume_text) if p)
        try:
            return PortalResult(**portals.search(request.url, request.keywords, candidate, engine))
        except ScrapeError as error:
            raise HTTPException(status_code=422, detail=str(error)) from None

    @app.get("/v1/portals/session", response_model=PortalSession, response_model_by_alias=True)
    def portal_session() -> PortalSession:
        return session()

    @app.post("/v1/portals/signin", response_model=PortalSession, response_model_by_alias=True)
    def portal_signin(request: PortalSignInRequest) -> PortalSession:
        try:
            result = signin.open_window(request.url)
        except ScrapeError as error:
            raise HTTPException(status_code=422, detail=str(error)) from None
        except Exception:  # noqa: BLE001 - browser binary missing
            raise HTTPException(status_code=503, detail="could not open a browser window; run npm run setup:ai") from None
        return session(result["message"])

    @app.delete("/v1/portals/session", response_model=PortalSession, response_model_by_alias=True)
    def portal_sign_out() -> PortalSession:
        try:
            signin.sign_out()
        except ScrapeError as error:
            raise HTTPException(status_code=409, detail=str(error)) from None
        return session("Signed out; the saved browser sign-in was deleted.")

    return app


app = create_app(enable_tracing=True)

"""A small agent orchestrator: ordered steps over shared state, with full observability.

Every step records its status, duration and a human-readable detail, and may emit trace
events. The Processing workflow page in the console renders exactly this record, so anything
a step reports is visible to the user. Steps can be cancelled between boundaries.
"""

from __future__ import annotations

import threading
import time
import uuid
from collections import OrderedDict
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Callable

from .schemas import AnalysisResult, PipelineStep, Run, RunStep, TraceEntry

MAX_TRACE = 500
MAX_RUNS = 50


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


class Cancelled(Exception):
    """Raised inside a step when the run was cancelled."""


class StepContext:
    """What a step can see and do: shared state, tracing, and cancellation checks."""

    def __init__(self, state: dict[str, Any], emit: Callable[[str, str], None], cancelled: threading.Event):
        self.state = state
        self._emit = emit
        self._cancelled = cancelled

    def log(self, stage: str, message: str) -> None:
        self._emit(stage, message)

    def checkpoint(self) -> None:
        if self._cancelled.is_set():
            raise Cancelled()


@dataclass(frozen=True)
class Step:
    id: str
    title: str
    description: str
    kind: str  # "prepare", "model" or "reason": shown as the step's category in the UI
    run: Callable[[StepContext], str | None]


class Workflow:
    def __init__(self, steps: list[Step]):
        ids = [step.id for step in steps]
        if len(set(ids)) != len(ids):
            raise ValueError("workflow step ids must be unique")
        self.steps = steps

    def describe(self) -> list[PipelineStep]:
        return [
            PipelineStep(id=s.id, title=s.title, description=s.description, kind=s.kind, after=[self.steps[i - 1].id] if i else [])
            for i, s in enumerate(self.steps)
        ]


class _Execution:
    def __init__(self, run: Run):
        self.run = run
        self.cancelled = threading.Event()


class Orchestrator:
    """Runs workflows on a small worker pool and keeps a bounded history of runs."""

    def __init__(self, workflow: Workflow, workers: int = 2):
        self.workflow = workflow
        self._pool = ThreadPoolExecutor(max_workers=workers, thread_name_prefix="cortex-run")
        self._lock = threading.Lock()
        self._executions: OrderedDict[str, _Execution] = OrderedDict()

    def submit(self, state: dict[str, Any], job_count: int = 0) -> Run:
        run = Run(
            id=uuid.uuid4().hex[:12],
            started_at=_now(),
            job_count=job_count,
            steps=[
                RunStep(
                    id=s.id,
                    title=s.title,
                    description=s.description,
                    kind=s.kind,
                    after=[self.workflow.steps[i - 1].id] if i else [],
                )
                for i, s in enumerate(self.workflow.steps)
            ],
        )
        execution = _Execution(run)
        with self._lock:
            self._executions[run.id] = execution
            while len(self._executions) > MAX_RUNS:
                self._executions.popitem(last=False)
        self._pool.submit(self._execute, execution, state)
        return self.get(run.id)

    def get(self, run_id: str) -> Run:
        with self._lock:
            execution = self._executions.get(run_id)
            if execution is None:
                raise KeyError(run_id)
            return execution.run.model_copy(deep=True)

    def list(self) -> list[Run]:
        with self._lock:
            return [e.run.model_copy(deep=True) for e in reversed(self._executions.values())]

    def cancel(self, run_id: str) -> None:
        with self._lock:
            execution = self._executions.get(run_id)
            if execution is None:
                raise KeyError(run_id)
            execution.cancelled.set()

    def _emit(self, execution: _Execution, stage: str, message: str) -> None:
        with self._lock:
            trace = execution.run.trace
            if len(trace) < MAX_TRACE:
                trace.append(TraceEntry(time=_now(), stage=stage, message=message))

    def _update(self, execution: _Execution, index: int, **fields: Any) -> None:
        with self._lock:
            step = execution.run.steps[index]
            for name, value in fields.items():
                setattr(step, name, value)

    def _finish(self, execution: _Execution, status: str, error: str = "") -> None:
        with self._lock:
            execution.run.status = status  # type: ignore[assignment]
            execution.run.error = error
            execution.run.finished_at = _now()

    def _execute(self, execution: _Execution, state: dict[str, Any]) -> None:
        context = StepContext(state, lambda stage, message: self._emit(execution, stage, message), execution.cancelled)
        self._emit(execution, "run", f"started {len(self.workflow.steps)}-step workflow")
        for index, step in enumerate(self.workflow.steps):
            if execution.cancelled.is_set():
                self._skip_rest(execution, index)
                self._finish(execution, "cancelled", "run cancelled")
                return
            self._update(execution, index, status="running")
            self._emit(execution, step.id, f"{step.title}: started")
            started = time.perf_counter()
            try:
                detail = step.run(context) or ""
            except Cancelled:
                self._update(execution, index, status="skipped", detail="cancelled")
                self._skip_rest(execution, index + 1)
                self._finish(execution, "cancelled", "run cancelled")
                return
            except Exception as error:  # a failing step must never take the service down
                elapsed = int((time.perf_counter() - started) * 1000)
                self._update(execution, index, status="failed", detail=str(error)[:300], duration_ms=elapsed)
                self._emit(execution, step.id, f"{step.title}: failed: {error}")
                self._skip_rest(execution, index + 1)
                self._finish(execution, "failed", f"{step.title} failed: {error}"[:400])
                return
            elapsed = int((time.perf_counter() - started) * 1000)
            self._update(execution, index, status="done", detail=detail, duration_ms=elapsed)
            self._emit(execution, step.id, f"{step.title}: done in {elapsed} ms" + (f" ({detail})" if detail else ""))
        result = state.get("result")
        with self._lock:
            if isinstance(result, AnalysisResult):
                execution.run.result = result
        self._finish(execution, "completed")

    def _skip_rest(self, execution: _Execution, start: int) -> None:
        for index in range(start, len(self.workflow.steps)):
            self._update(execution, index, status="skipped")

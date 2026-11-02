import { CheckCircle2, Circle, LoaderCircle, MinusCircle, Square, Workflow, XCircle } from "lucide-react";
import { useEffect, useState } from "react";
import { apiRequest } from "../../lib/api";
import { AIRun, PipelineStep, RunStep, StepStatus } from "./types";
import { RunGraph } from "./RunGraph";
import { formatDuration, useRun } from "./useRun";

const kindLabel: Record<string, string> = { prepare: "Prepare", model: "Model", reason: "Reason", human: "You" };

function StepIcon({ status }: { status: StepStatus }) {
  switch (status) {
    case "running": return <LoaderCircle className="spin" size={18} />;
    case "done": return <CheckCircle2 size={18} />;
    case "failed": return <XCircle size={18} />;
    case "skipped": return <MinusCircle size={18} />;
    default: return <Circle size={18} />;
  }
}

function useRunList(refreshKey: string | null): AIRun[] | null {
  const [runs, setRuns] = useState<AIRun[] | null>(null);
  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const [analyses, applications] = await Promise.all([
          apiRequest<AIRun[]>("/api/ai/analyses"),
          apiRequest<AIRun[]>("/api/ai/applications").catch(() => [] as AIRun[]),
        ]);
        if (cancelled) return;
        const list = [...analyses, ...applications].sort((a, b) => b.startedAt.localeCompare(a.startedAt));
        setRuns(list);
        timer = window.setTimeout(() => void tick(), list.some((run) => run.status === "running") ? 1000 : 4000);
      } catch {
        if (!cancelled) {
          setRuns([]);
          timer = window.setTimeout(() => void tick(), 5000);
        }
      }
    };
    void tick();
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [refreshKey]);
  return runs;
}

function usePipeline(): PipelineStep[] {
  const [steps, setSteps] = useState<PipelineStep[]>([]);
  useEffect(() => {
    let cancelled = false;
    apiRequest<PipelineStep[]>("/api/ai/pipeline").then((value) => !cancelled && setSteps(value)).catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);
  return steps;
}

export function ProcessingWorkflow({ selectedRunId, onSelectRun, onOpenAnalysis }: {
  selectedRunId: string | null;
  onSelectRun: (id: string) => void;
  onOpenAnalysis: () => void;
}) {
  const runs = useRunList(selectedRunId);
  const pipeline = usePipeline();
  const activeId = selectedRunId ?? runs?.[0]?.id ?? null;
  const activeKind = runs?.find((item) => item.id === activeId)?.kind ?? "analysis";
  const apiPath = activeKind === "application" ? "applications" : "analyses";
  const { run } = useRun(activeId, apiPath);
  const [message, setMessage] = useState("");

  const cancel = async () => {
    if (!run) return;
    try {
      await apiRequest(`/api/ai/${apiPath}/${encodeURIComponent(run.id)}/cancel`, { method: "POST" });
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not cancel the run.");
    }
  };

  const kinds = new Map(pipeline.map((step) => [step.id, step.kind]));
  const steps: (RunStep | (PipelineStep & { status: StepStatus; detail: string; durationMs: number }))[] =
    run ? run.steps : pipeline.map((step) => ({ ...step, status: "pending" as const, detail: "", durationMs: 0 }));
  const slowest = Math.max(1, ...steps.map((step) => step.durationMs));
  const totalMs = steps.reduce((sum, step) => sum + step.durationMs, 0);

  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> OBSERVABILITY</div><h1>Processing workflow</h1><p className="page-subtitle">Every step the AI takes, how long it took, and what it found. Nothing happens off-screen.</p></div>
      <button className="button button-secondary" onClick={onOpenAnalysis}>Back to AI analysis</button></div>

    {runs && runs.length > 0 && <div className="panel ai-run-picker">
      <label className="form-field"><span>Run</span>
        <select value={activeId ?? ""} onChange={(event) => onSelectRun(event.target.value)}>
          {runs.map((item) => <option key={item.id} value={item.id}>{new Date(item.startedAt).toLocaleTimeString()} · {item.kind === "application" ? `Application: ${item.title ?? ""}` : `${item.jobCount} jobs`} · {item.status}</option>)}
        </select>
      </label>
      {run?.status === "running" && <button className="button button-secondary" onClick={() => void cancel()}><Square size={14} /> Stop run</button>}
    </div>}
    {message && <div className="workflow-error" role="alert">{message}</div>}

    {steps.length === 0
      ? <div className="panel history-empty"><Workflow size={18} /><span>The AI service is offline or has no pipeline to show. Start it with npm run dev:ai.</span></div>
      : <div className="panel workflow-steps">
        <div className="workflow-run-head"><strong>{run ? `Run ${run.id}` : "Pipeline"}{totalMs > 0 ? ` · ${formatDuration(totalMs)}` : ""}</strong>{run ? <span className={`workflow-status ${run.status}`}>{run.status}</span> : <span className="ai-note">No runs yet. This is what will happen.</span>}</div>
        {steps.some((step) => step.after) && <RunGraph nodes={steps} waitingOn={run?.status === "awaiting_approval" ? "review" : undefined} />}
        {steps.map((step, index) => <div key={step.id} className={`workflow-step ${step.status}`}>
          <div className="workflow-step-icon"><StepIcon status={step.status} />{index < steps.length - 1 && <span className="workflow-step-line" />}</div>
          <div className="workflow-step-copy">
            <strong>{step.title} <span className={`ai-kind ${step.kind ?? kinds.get(step.id) ?? ""}`}>{kindLabel[step.kind ?? kinds.get(step.id) ?? "prepare"] ?? "Step"}</span>{step.agent && <span className="ai-graph-agent">{step.agent}</span>}</strong>
            <span>{step.detail || step.description}</span>
            {step.durationMs > 0 && <div className="ai-duration"><span style={{ width: `${Math.max(4, (step.durationMs / slowest) * 100)}%` }} /></div>}
          </div>
          <time>{formatDuration(step.durationMs)}</time>
        </div>)}
        {run?.error && <div className="workflow-error">{run.error}</div>}
      </div>}

    {run && run.trace.length > 0 && <div className="panel workflow-trace"><div className="section-kicker">TRACE LOG</div>
      <div className="workflow-trace-list">{run.trace.map((entry, index) => <div key={index}><time>{new Date(entry.time).toLocaleTimeString()}</time><b>{entry.stage}</b><span>{entry.message}</span></div>)}</div></div>}
  </>;
}

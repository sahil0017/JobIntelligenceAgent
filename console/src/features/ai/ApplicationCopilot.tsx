import { Check, CloudUpload, LoaderCircle, Send, ShieldCheck, Square, TriangleAlert, X } from "lucide-react";
import { useEffect, useState } from "react";
import { apiRequest } from "../../lib/api";
import { Job } from "../../types";
import { AIRun, Draft } from "./types";
import { RunGraph } from "./RunGraph";
import { useAIStatus, useRun } from "./useRun";

const statusLabel: Record<string, string> = {
  running: "Working",
  awaiting_approval: "Needs your approval",
  completed: "Finished",
  failed: "Failed",
  cancelled: "Stopped",
};

export function useApplicationRuns(refreshKey: unknown): AIRun[] {
  const [runs, setRuns] = useState<AIRun[]>([]);
  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const list = await apiRequest<AIRun[]>("/api/ai/applications");
        if (cancelled) return;
        setRuns(list);
        timer = window.setTimeout(() => void tick(), list.some((run) => run.status === "running") ? 1000 : 5000);
      } catch {
        if (!cancelled) timer = window.setTimeout(() => void tick(), 8000);
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

function DraftReview({ run, onDecided }: { run: AIRun; onDecided: () => void }) {
  const draft = run.draft as Draft;
  const [bullets, setBullets] = useState(draft.tailoredBullets);
  const [cover, setCover] = useState(draft.coverNote);
  const [outreach, setOutreach] = useState(draft.outreachMessage);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const pending = run.status === "awaiting_approval";
  const applyStep = run.steps.find((step) => step.id === "apply");
  const canApply = run.status === "completed" && draft.status === "approved" && applyStep !== undefined;

  useEffect(() => {
    if (applyStep?.status !== "running") return undefined;
    const timer = window.setTimeout(onDecided, 1500); // the form opens in the background; poll until it reports back
    return () => window.clearTimeout(timer);
  }, [applyStep?.status, applyStep?.detail]);

  const apply = async () => {
    setBusy(true);
    setMessage("");
    try {
      await apiRequest(`/api/ai/applications/${encodeURIComponent(run.id)}/apply`, { method: "POST" });
      onDecided();
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not open the application form.");
    } finally {
      setBusy(false);
    }
  };

  const decide = async (approve: boolean) => {
    setBusy(true);
    setMessage("");
    try {
      await apiRequest(`/api/ai/applications/${encodeURIComponent(run.id)}/decision`, {
        method: "POST",
        body: JSON.stringify({ approve, tailoredBullets: bullets.filter((line) => line.trim()), coverNote: cover, outreachMessage: outreach }),
      });
      onDecided();
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not record your decision.");
    } finally {
      setBusy(false);
    }
  };

  return <div className="panel ai-draft">
    <div className="workflow-run-head"><strong>{draft.role}{draft.company ? ` · ${draft.company}` : ""}</strong>
      <span className={`workflow-status ${run.status}`}>{statusLabel[run.status] ?? run.status}</span></div>
    <p className="ai-note">Fit {draft.fitScore}% · {draft.verdict}{draft.writer ? ` · written by ${draft.writer}` : ""}{draft.pageTitle ? ` · read “${draft.pageTitle}” (${draft.pageMethod})` : ""}</p>
    {draft.warnings.map((warning) => <div key={warning} className="workflow-error"><TriangleAlert size={14} /> {warning}</div>)}
    {draft.status === "not_recommended"
      ? <p>This role looks like a weak fit, so no draft was written. You can ask for one anyway from the form above.</p>
      : <>
        {draft.skills && <div className="ai-skill-row">
          {draft.skills.matched.map((skill) => <span key={skill} className="ai-chip matched">{skill}</span>)}
          {draft.skills.missing.map((skill) => <span key={skill} className="ai-chip missing">{skill}</span>)}
        </div>}
        {draft.strategy && <details className="ai-sources" open><summary>Positioning plan (strategy agent)</summary><p>{draft.strategy}</p></details>}
        {draft.critique && <details className="ai-sources" open><summary>Reviewer notes (critic agent)</summary><p>{draft.critique}</p></details>}
        {draft.research && <details className="ai-sources" open><summary>Company research from the live web</summary><p>{draft.research}</p></details>}
        <div className="section-kicker">TAILORED RÉSUMÉ BULLETS</div>
        {bullets.map((bullet, index) => <textarea key={index} className="ai-edit" rows={2} value={bullet} disabled={!pending}
          aria-label={`Bullet ${index + 1}`} onChange={(event) => setBullets(bullets.map((item, i) => (i === index ? event.target.value : item)))} />)}
        <div className="section-kicker">COVER NOTE</div>
        <textarea className="ai-edit" rows={4} value={cover} disabled={!pending} onChange={(event) => setCover(event.target.value)} />
        <div className="section-kicker">OUTREACH MESSAGE</div>
        <textarea className="ai-edit" rows={3} value={outreach} disabled={!pending} onChange={(event) => setOutreach(event.target.value)} />
        {draft.sources.length > 0 && <details className="ai-sources"><summary>Résumé passages this is based on ({draft.sources.length})</summary>
          <ul>{draft.sources.map((source) => <li key={source}>{source}</li>)}</ul></details>}
      </>}
    {message && <div className="workflow-error" role="alert">{message}</div>}
    {pending && <div className="ai-actions">
      <button className="button button-primary" disabled={busy} onClick={() => void decide(true)}><Check size={14} /> Approve draft</button>
      <button className="button button-secondary" disabled={busy} onClick={() => void decide(false)}><X size={14} /> Reject</button>
    </div>}
    {canApply && <div className="panel ai-apply">
      <div><strong>Apply in the company portal</strong>
        <span>{applyStep?.status === "done" || applyStep?.status === "failed" ? applyStep.detail : "Opens the application form in a browser window and pre-fills your details and cover note. You review it, attach your résumé and press Submit yourself."}</span></div>
      <button className="button button-primary" disabled={busy || applyStep?.status === "running"} onClick={() => void apply()}>
        {applyStep?.status === "running" ? <LoaderCircle className="spin" size={14} /> : <Send size={14} />} {applyStep?.status === "done" ? "Open again" : "Apply in portal"}
      </button>
    </div>}
    <p className="ai-note"><ShieldCheck size={13} /> Nothing is submitted for you. Approving saves your decision; applying only pre-fills the form and you press Submit.</p>
  </div>;
}

export function ApplicationCopilot({ jobs, runId, onRunStarted, onOpenWorkflow }: {
  jobs: Job[];
  runId: string | null;
  onRunStarted: (id: string) => void;
  onOpenWorkflow: () => void;
}) {
  const status = useAIStatus();
  const { run, error: pollError, reload } = useRun(runId, "applications");
  const [jobId, setJobId] = useState("");
  const [url, setUrl] = useState("");
  const [draftAnyway, setDraftAnyway] = useState(false);
  const [starting, setStarting] = useState(false);
  const [message, setMessage] = useState("");
  const [version, setVersion] = useState(0);
  const queue = useApplicationRuns(version).filter((item) => item.status === "awaiting_approval");
  const job = jobs.find((item) => item.id === jobId) ?? jobs[0];

  useEffect(() => {
    if (job?.url && !url) setUrl(job.url);
  }, [job?.id]);

  const start = async () => {
    if (!job) return;
    setStarting(true);
    setMessage("");
    try {
      const started = await apiRequest<AIRun>("/api/ai/applications", {
        method: "POST",
        body: JSON.stringify({ job: { id: job.id, company: job.company, role: job.role, location: job.location, tags: job.tags }, url: url.trim(), draftAnyway }),
      });
      onRunStarted(started.id);
      setVersion((value) => value + 1);
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not start the copilot.");
    } finally {
      setStarting(false);
    }
  };

  const cancel = async () => {
    if (!run) return;
    try {
      await apiRequest(`/api/ai/applications/${encodeURIComponent(run.id)}/cancel`, { method: "POST" });
      reload();
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not stop the run.");
    }
  };

  const offline = status !== null && !status.available;
  const active = run?.status === "running";

  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> AGENTIC AI</div><h1>Application copilot</h1>
      <p className="page-subtitle">A team of agents reads the job page, matches it to your résumé, drafts tailored text, checks it for invented claims, then waits for you.</p></div>
      <button className="button button-secondary" onClick={onOpenWorkflow}>See all workflows</button></div>

    {offline && <div className="panel history-empty"><TriangleAlert size={18} /><span>{status?.message ?? "The AI service is offline."} Start it with npm run dev:ai.</span></div>}
    {status?.keyExpiresInDays != null && status.keyExpiresInDays <= 14 && <div className="workflow-error"><TriangleAlert size={14} /> {status.keyExpiresInDays < 0 ? "Your OpenAI key has expired." : `Your OpenAI key expires in ${status.keyExpiresInDays} day${status.keyExpiresInDays === 1 ? "" : "s"}.`} Create a new one in the OpenAI dashboard and update cortex/.env, then restart the AI service.</div>}
    {status?.llmRemote && <div className="workflow-error"><TriangleAlert size={14} /> Your résumé text is sent to a hosted model ({status.llm}). Set CORTEX_LLM=ollama to keep it on this machine.</div>}

    {queue.length > 0 && <div className="panel ai-queue"><div className="section-kicker">WAITING FOR YOU ({queue.length})</div>
      {queue.map((item) => <button key={item.id} className="text-button" onClick={() => onRunStarted(item.id)}>{item.title || item.id}</button>)}</div>}

    <div className="panel ai-launch">
      <label className="form-field"><span>Job</span>
        <select value={job?.id ?? ""} onChange={(event) => { setJobId(event.target.value); setUrl(""); }} disabled={jobs.length === 0}>
          {jobs.length === 0 && <option>No jobs yet. Run a search first</option>}
          {jobs.map((item) => <option key={item.id} value={item.id}>{item.role} · {item.company}</option>)}
        </select></label>
      <label className="form-field"><span>Job page (optional)</span>
        <input type="url" placeholder="https://company.com/careers/role" value={url} onChange={(event) => setUrl(event.target.value)} /></label>
      <label className="ai-check"><input type="checkbox" checked={draftAnyway} onChange={(event) => setDraftAnyway(event.target.checked)} /> Draft even if it looks like a weak fit</label>
      <div className="ai-actions">
        <button className="button button-primary" disabled={!job || starting || active || offline} onClick={() => void start()}>
          {starting || active ? <LoaderCircle className="spin" size={14} /> : <Send size={14} />} Draft application</button>
        {active && <button className="button button-secondary" onClick={() => void cancel()}><Square size={14} /> Stop</button>}
      </div>
      {message && <div className="workflow-error" role="alert">{message}</div>}
    </div>

    {pollError && <div className="workflow-error" role="alert">{pollError}</div>}
    {run && <div className="panel"><div className="workflow-run-head"><strong><CloudUpload size={14} /> Live agent graph</strong>
      <span className={`workflow-status ${run.status}`}>{statusLabel[run.status] ?? run.status}</span></div>
      <RunGraph nodes={run.steps} waitingOn={run.status === "awaiting_approval" ? "review" : undefined} />
      {run.error && <div className="workflow-error">{run.error}</div>}</div>}
    {run?.draft && <DraftReview key={`${run.id}-${run.draft.status}`} run={run} onDecided={() => { reload(); setVersion((value) => value + 1); }} />}
  </>;
}

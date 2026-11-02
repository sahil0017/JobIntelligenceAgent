import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowUpRight, Circle, CheckCircle2, LoaderCircle, MinusCircle, Play, Plus, Settings2, Sparkles, Square, XCircle } from "lucide-react";

export type AgentJob = {
  id: string;
  company: string;
  role: string;
  location: string;
  workMode: string;
  salary: string;
  posted: string;
  source: string;
  url?: string;
  score: number;
  tags: string[];
  status: string;
  logo: string;
  logoTone: string;
};

type StepStatus = "pending" | "running" | "done" | "skipped" | "failed";
type RunStep = { id: string; title: string; description: string; status: StepStatus; detail: string; durationMs: number };
type TraceEntry = { time: string; stage: string; message: string };
type RunResult = {
  jobs: AgentJob[];
  sourceErrors: { url?: string; error: string; category?: string }[];
  checkedSources: number;
  filteredLowMatch: number;
};
type AgentRun = {
  id: string;
  status: "running" | "completed" | "failed" | "cancelled";
  country: string;
  state: string;
  area: string;
  startedAt: string;
  steps: RunStep[];
  trace: TraceEntry[];
  result?: RunResult;
  review: string;
  error: string;
};
type AgentConfig = {
  model: string;
  ollamaUrl: string;
  maxRounds: number;
  targetMatches: number;
  maxPages: number;
  minScore: number;
  llmReview: boolean;
};

const reasonLabels: Record<string, string> = {
  auth_required: "need a login (skipped)",
  closed: "closed or removed",
  robots: "blocked by robots.txt",
  redirect: "redirected elsewhere",
  no_data: "had no structured postings",
  error: "could not be read",
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, headers: { "Content-Type": "application/json", ...init?.headers } });
  const text = await response.text();
  let payload: { error?: string } | undefined;
  try {
    payload = text ? JSON.parse(text) : undefined;
  } catch {
    throw new Error(`Request to ${path} failed (${response.status}) with a non-JSON response.`);
  }
  if (!response.ok) throw new Error(payload?.error ?? `Request to ${path} failed (${response.status}).`);
  return payload as T;
}

function StepIcon({ status }: { status: StepStatus }) {
  switch (status) {
    case "running": return <LoaderCircle className="spin" size={18} />;
    case "done": return <CheckCircle2 size={18} />;
    case "failed": return <XCircle size={18} />;
    case "skipped": return <MinusCircle size={18} />;
    default: return <Circle size={18} />;
  }
}

function duration(ms: number) {
  return ms > 0 ? `${(ms / 1000).toFixed(1)}s` : "";
}

export function AgentWorkflow({ defaultCountry, defaultState, defaultArea, onAddJobs }: {
  defaultCountry: string;
  defaultState: string;
  defaultArea: string;
  onAddJobs: (jobs: AgentJob[]) => void;
}) {
  const [country, setCountry] = useState(defaultCountry || "India");
  const [state, setState] = useState(defaultState);
  const [area, setArea] = useState(defaultArea);
  const [run, setRun] = useState<AgentRun | null>(null);
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [showConfig, setShowConfig] = useState(false);
  const [message, setMessage] = useState("");
  const [starting, setStarting] = useState(false);
  const addedRunId = useRef("");

  const running = run?.status === "running";

  useEffect(() => {
    void (async () => {
      try {
        setConfig(await request<AgentConfig>("/api/agent/config"));
        const { runs } = await request<{ runs: AgentRun[] }>("/api/agent/runs");
        if (runs.length) setRun(await request<AgentRun>(`/api/agent/runs/${runs[0].id}`));
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load the agent.");
      }
    })();
  }, []);

  const runId = run?.id;
  useEffect(() => {
    if (!runId || !running) return undefined;
    const timer = window.setInterval(() => {
      request<AgentRun>(`/api/agent/runs/${runId}`).then(setRun).catch((error) => setMessage(error instanceof Error ? error.message : "Lost contact with the agent."));
    }, 1500);
    return () => window.clearInterval(timer);
  }, [runId, running]);

  const start = useCallback(async () => {
    setMessage("");
    setStarting(true);
    try {
      addedRunId.current = "";
      setRun(await request<AgentRun>("/api/agent/runs", { method: "POST", body: JSON.stringify({ country: country.trim(), state: state.trim(), area: area.trim() }) }));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not start the agent.");
    } finally {
      setStarting(false);
    }
  }, [country, state, area]);

  const cancel = async () => {
    if (!run) return;
    try {
      await request(`/api/agent/runs/${run.id}/cancel`, { method: "POST" });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not cancel the run.");
    }
  };

  const saveConfig = async () => {
    if (!config) return;
    setMessage("");
    try {
      setConfig(await request<AgentConfig>("/api/agent/config", { method: "PUT", body: JSON.stringify(config) }));
      setMessage("Agent settings saved.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not save agent settings.");
    }
  };

  const setNumber = (key: "maxRounds" | "targetMatches" | "maxPages" | "minScore", value: string) =>
    setConfig((current) => current && { ...current, [key]: Number(value) });

  const reasons = new Map<string, number>();
  run?.result?.sourceErrors.forEach(({ category }) => reasons.set(category ?? "error", (reasons.get(category ?? "error") ?? 0) + 1));
  const jobs = run?.result?.jobs ?? [];

  const addJobs = () => {
    if (!run || addedRunId.current === run.id) return;
    addedRunId.current = run.id;
    onAddJobs(jobs);
    setMessage(`Added ${jobs.length} job${jobs.length === 1 ? "" : "s"} to Job matches.`);
  };

  return <section className="agent-workflow">
    <div className="section-heading-row activity-heading"><div><div className="section-kicker">AGENTIC WORKFLOW</div><h2>Resume-matched job search</h2></div>
      <button className="button button-secondary" onClick={() => setShowConfig((value) => !value)}><Settings2 size={15} /> Agent settings</button></div>
    <div className="panel workflow-form">
      <label className="form-field"><span>Country</span><input value={country} onChange={(event) => setCountry(event.target.value)} /></label>
      <label className="form-field"><span>State</span><input value={state} onChange={(event) => setState(event.target.value)} /></label>
      <label className="form-field"><span>City or area</span><input value={area} onChange={(event) => setArea(event.target.value)} placeholder="e.g. Bengaluru" /></label>
      {running
        ? <button className="button button-secondary" onClick={() => void cancel()}><Square size={14} /> Stop run</button>
        : <button className="button button-primary" disabled={starting || !country.trim() || !area.trim()} onClick={() => void start()}>{starting ? <LoaderCircle className="spin" size={15} /> : <Play size={15} />} Run agent</button>}
    </div>
    {message && <div className="workflow-message" role="status">{message}</div>}
    {showConfig && config && <div className="panel workflow-config">
      <label className="form-field"><span>Ollama model</span><input value={config.model} onChange={(event) => setConfig({ ...config, model: event.target.value })} /></label>
      <label className="form-field"><span>Ollama URL</span><input value={config.ollamaUrl} onChange={(event) => setConfig({ ...config, ollamaUrl: event.target.value })} /></label>
      <label className="form-field"><span>Search rounds (1–6)</span><input type="number" min={1} max={6} value={config.maxRounds} onChange={(event) => setNumber("maxRounds", event.target.value)} /></label>
      <label className="form-field"><span>Target matches (1–50)</span><input type="number" min={1} max={50} value={config.targetMatches} onChange={(event) => setNumber("targetMatches", event.target.value)} /></label>
      <label className="form-field"><span>Page budget (5–60)</span><input type="number" min={5} max={60} value={config.maxPages} onChange={(event) => setNumber("maxPages", event.target.value)} /></label>
      <label className="form-field"><span>Minimum match score (1–100)</span><input type="number" min={1} max={100} value={config.minScore} onChange={(event) => setNumber("minScore", event.target.value)} /></label>
      <label className="workflow-toggle"><input type="checkbox" checked={config.llmReview} onChange={(event) => setConfig({ ...config, llmReview: event.target.checked })} /> Summarize best matches with the local model (sends job summaries, never your resume)</label>
      <button className="button button-primary" onClick={() => void saveConfig()}>Save agent settings</button>
    </div>}
    {run ? <>
      <div className="panel workflow-steps">
        <div className="workflow-run-head"><strong>{run.area}{run.state ? `, ${run.state}` : ""}, {run.country}</strong><span className={`workflow-status ${run.status}`}>{run.status}</span></div>
        {run.steps.map((step, index) => <div key={step.id} className={`workflow-step ${step.status}`}>
          <div className="workflow-step-icon"><StepIcon status={step.status} />{index < run.steps.length - 1 && <span className="workflow-step-line" />}</div>
          <div className="workflow-step-copy"><strong>{step.title}</strong><span>{step.detail || step.description}</span></div>
          <time>{duration(step.durationMs)}</time>
        </div>)}
        {run.error && <div className="workflow-error">{run.error}</div>}
      </div>
      {run.review && <div className="panel workflow-review"><div className="section-kicker"><Sparkles size={13} /> LOCAL MODEL REVIEW</div><p>{run.review}</p></div>}
      {run.result && <div className="panel workflow-results">
        <div className="workflow-run-head"><strong>{jobs.length} matching job{jobs.length === 1 ? "" : "s"}</strong>
          <button className="button button-secondary" disabled={!jobs.length || addedRunId.current === run.id} onClick={addJobs}><Plus size={14} /> Add to Job matches</button></div>
        <p className="workflow-note">{run.result.checkedSources} pages read{run.result.filteredLowMatch ? `, ${run.result.filteredLowMatch} weak resume match${run.result.filteredLowMatch === 1 ? "" : "es"} filtered out` : ""}.{reasons.size ? ` Skipped: ${[...reasons].map(([key, count]) => `${count} ${reasonLabels[key] ?? key}`).join(", ")}.` : ""}</p>
        {jobs.map((job) => <div className="workflow-job" key={job.id}><div><strong>{job.role}</strong><span>{job.company} · {job.location}</span></div><span className="activity-badge">{job.score}% match</span>{job.url && <a href={job.url} target="_blank" rel="noreferrer" aria-label={`Open ${job.role} listing`}><ArrowUpRight size={15} /></a>}</div>)}
      </div>}
      <div className="panel workflow-trace"><div className="section-kicker">TRACE LOG</div>
        <div className="workflow-trace-list">{(run.trace ?? []).map((entry, index) => <div key={index}><time>{new Date(entry.time).toLocaleTimeString()}</time><b>{entry.stage}</b><span>{entry.message}</span></div>)}</div></div>
    </> : <div className="panel history-empty"><Sparkles size={18} /><span>Run the agent to watch it plan, search, verify and filter jobs against your resume.</span></div>}
  </section>;
}

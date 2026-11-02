import { ArrowRight, BrainCircuit, LoaderCircle, Play, Sparkles, Square, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { apiRequest } from "../../lib/api";
import { Job } from "../../types";
import { AIRun, JobAnalysis } from "./types";
import { useAIStatus, useRun } from "./useRun";

const MAX_JOBS = 100;

const verdictLabel: Record<JobAnalysis["verdict"], string> = {
  strong: "Strong fit",
  good: "Good fit",
  stretch: "Stretch",
  weak: "Weak fit",
};

export function AIAnalysis({ jobs, runId, onRunStarted, onOpenWorkflow, onOpenMatches }: {
  jobs: Job[];
  runId: string | null;
  onRunStarted: (id: string) => void;
  onOpenWorkflow: () => void;
  onOpenMatches: () => void;
}) {
  const status = useAIStatus();
  const { run, error: pollError } = useRun(runId);
  const [starting, setStarting] = useState(false);
  const [message, setMessage] = useState("");
  const running = run?.status === "running";
  const analysable = jobs.slice(0, MAX_JOBS);

  const start = async () => {
    setStarting(true);
    setMessage("");
    try {
      const started = await apiRequest<AIRun>("/api/ai/analyses", {
        method: "POST",
        body: JSON.stringify({
          jobs: analysable.map((job) => ({ id: job.id, company: job.company, role: job.role, location: job.location, tags: job.tags })),
        }),
      });
      onRunStarted(started.id);
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not start the analysis.");
    } finally {
      setStarting(false);
    }
  };

  const cancel = async () => {
    if (!run) return;
    try {
      await apiRequest(`/api/ai/analyses/${encodeURIComponent(run.id)}/cancel`, { method: "POST" });
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Could not cancel the analysis.");
    }
  };

  const doneSteps = run?.steps.filter((step) => step.status === "done").length ?? 0;

  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> AGENTIC AI</div><h1>AI analysis</h1><p className="page-subtitle">An AI pipeline reads your profile and résumé, then ranks your job matches by meaning and skill gaps.</p></div></div>

    <div className={`panel ai-engine ${status?.available ? "online" : "offline"}`}>
      <BrainCircuit size={20} />
      <div>
        {!status && <strong>Checking the AI service…</strong>}
        {status?.available && <><strong>AI service online · {status.engine}</strong><span>{status.deepLearning ? "Deep-learning embeddings are active." : "Using the lightweight engine. Set CORTEX_EMBEDDER=sentence-transformers to enable deep-learning embeddings."}</span></>}
        {status && !status.available && <><strong>AI service offline</strong><span>{status.message ?? "Start it with npm run dev:ai."}</span></>}
      </div>
    </div>

    <div className="panel ai-run-bar">
      <div><strong>{analysable.length} job{analysable.length === 1 ? "" : "s"} to analyse</strong><span>{jobs.length > MAX_JOBS ? `The first ${MAX_JOBS} of ${jobs.length} matches are analysed.` : "Uses your saved profile and résumé; they stay on this machine."}</span></div>
      {running
        ? <button className="button button-secondary" onClick={() => void cancel()}><Square size={14} /> Stop</button>
        : <button className="button button-primary" disabled={starting || !status?.available || analysable.length === 0} onClick={() => void start()}>{starting ? <LoaderCircle className="spin" size={15} /> : <Play size={15} />} Run analysis</button>}
    </div>
    {jobs.length === 0 && <div className="workflow-message">No job matches yet. <button className="text-button" onClick={onOpenMatches}>Find jobs first</button></div>}
    {(message || pollError) && <div className="workflow-error" role="alert">{message || pollError}</div>}

    {run && <div className="panel ai-progress">
      <div className="workflow-run-head"><strong>{running ? `Running · step ${Math.min(doneSteps + 1, run.steps.length)} of ${run.steps.length}` : `Analysis ${run.status}`}</strong><span className={`workflow-status ${run.status}`}>{run.status}</span></div>
      <div className="ai-progress-track"><span style={{ width: `${(doneSteps / run.steps.length) * 100}%` }} /></div>
      {run.error && <div className="workflow-error">{run.error}</div>}
      <button className="text-button" onClick={onOpenWorkflow}>See the processing workflow <ArrowRight size={13} /></button>
    </div>}

    {run?.result && <>
      <div className="panel workflow-review">
        <div className="section-kicker"><Sparkles size={13} /> FINDINGS</div>
        <p>{run.result.summary}</p>
        {run.result.learningPriorities.length > 0 && <div className="ai-chips"><span className="ai-chips-label">Learn next</span>{run.result.learningPriorities.map((item) => <span key={item.skill} className="ai-chip missing">{item.skill} · {item.jobs}</span>)}</div>}
      </div>
      <div className="panel workflow-results">
        <div className="workflow-run-head"><strong>Ranked opportunities</strong><span className="ai-note">{run.result.candidateSkills.length} of your skills recognised</span></div>
        {run.result.jobs.map((job) => <div className="ai-job" key={job.id}>
          <div className="ai-job-rank">{job.rank}</div>
          <div className="ai-job-main">
            <strong>{job.role}</strong><span>{job.company}</span>
            <div className="ai-chips">
              {job.skills.matched.map((skill) => <span key={skill} className="ai-chip matched">{skill}</span>)}
              {job.skills.missing.map((skill) => <span key={skill} className="ai-chip missing">{skill}</span>)}
            </div>
          </div>
          <div className="ai-job-score">
            <span className={`ai-verdict ${job.verdict}`}>{verdictLabel[job.verdict]}</span>
            <b>{job.fitScore}%</b>
            <small>meaning {job.semanticScore}%{job.skillCoverage !== null ? ` · skills ${job.skillCoverage}%` : ""}</small>
          </div>
        </div>)}
        <p className="ai-note"><TriangleAlert size={12} /> Scores are model estimates to help you prioritise. Read each posting before applying.</p>
      </div>
    </>}

    {!run && status?.available && <div className="panel history-empty"><BrainCircuit size={18} /><span>Run the analysis to rank your matches and see which skills to learn next.</span></div>}
  </>;
}

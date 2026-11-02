import { ExternalLink, LoaderCircle, LogIn, Plus, Search, ShieldAlert, Sparkles, Zap } from "lucide-react";
import { useEffect, useState } from "react";
import { apiRequest } from "../../lib/api";
import { Job } from "../../types";
import { PortalResult, PortalSession } from "./types";
import { useAIStatus } from "./useRun";

const EMPLOYERS = [
  { name: "Amazon", url: "https://www.amazon.jobs/en/search" },
  { name: "Oracle", url: "https://careers.oracle.com/jobs" },
  { name: "NVIDIA", url: "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite" },
  { name: "Salesforce", url: "https://salesforce.wd12.myworkdayjobs.com/External_Career_Site" },
  { name: "Adobe", url: "https://adobe.wd5.myworkdayjobs.com/external_experienced" },
  { name: "Intel", url: "https://intel.wd1.myworkdayjobs.com/External" },
];

type Props = {
  onFindEngineering: () => Promise<void>;
  discoveryRunning: boolean;
  sourceCount: number;
  onAddJob: (job: Job) => void;
  onOpenCopilot: () => void;
  onNotify: (message: string) => void;
};

export function PortalSearch({ onFindEngineering, discoveryRunning, sourceCount, onAddJob, onOpenCopilot, onNotify }: Props) {
  const status = useAIStatus();
  const [address, setAddress] = useState("");
  const [keywords, setKeywords] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<PortalResult | null>(null);
  const [session, setSession] = useState<PortalSession | null>(null);
  const [applying, setApplying] = useState("");
  const [applyNote, setApplyNote] = useState("");

  const refreshSession = () => apiRequest<PortalSession>("/api/ai/portals/session").then(setSession).catch(() => undefined);
  useEffect(() => {
    void refreshSession();
  }, []);

  const search = async (event?: React.FormEvent, override?: string) => {
    event?.preventDefault();
    setBusy(true);
    setError("");
    try {
      setResult(await apiRequest<PortalResult>("/api/ai/portals/search", { method: "POST", body: JSON.stringify({ url: (override ?? address).trim(), keywords: keywords || "software engineer" }) }));
    } catch (searchError) {
      setResult(null);
      setError(searchError instanceof Error ? searchError.message : "The search failed.");
    } finally {
      setBusy(false);
    }
  };

  const pick = (url: string) => {
    setAddress(url);
    void search(undefined, url);
  };

  const applyNow = async (job: PortalResult["jobs"][number]) => {
    setApplying(job.url);
    setApplyNote("");
    setError("");
    try {
      const outcome = await apiRequest<{ status: string; filled: string[]; message?: string }>("/api/ai/apply", { method: "POST", body: JSON.stringify({ url: job.url }) });
      setApplyNote(outcome.status === "failed" ? outcome.message ?? "Could not open the form." : `Form opened in a browser window${outcome.filled.length ? `; pre-filled ${outcome.filled.join(", ")}` : ""}. Review it, attach your résumé and press Submit yourself.`);
    } catch (applyError) {
      setError(applyError instanceof Error ? applyError.message : "Could not open the form.");
    } finally {
      setApplying("");
    }
  };

  const signIn = async () => {
    setError("");
    try {
      const next = await apiRequest<PortalSession>("/api/ai/portals/session", { method: "POST", body: JSON.stringify({ url: address.trim() }) });
      setSession(next);
      onNotify(next.message || "Sign-in window opened.");
    } catch (signInError) {
      setError(signInError instanceof Error ? signInError.message : "Could not open the sign-in window.");
    }
  };

  const track = (job: PortalResult["jobs"][number]) => {
    const company = result?.host.replace(/^(www|careers|jobs)\./, "").split(".")[0] ?? "Company";
    const name = company.charAt(0).toUpperCase() + company.slice(1);
    onAddJob({
      id: `portal-${Date.now()}-${Math.round(Math.random() * 1e4)}`, company: name, role: job.title, location: "", workMode: "Not specified", salary: "Not listed",
      posted: "Found on career portal", source: result?.host ?? "Career portal", url: job.url, score: job.score, tags: job.skills, status: "Review",
      logo: name.charAt(0), logoTone: "blue",
    });
  };

  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> JOB SEARCH</div><h1>Company portals</h1><p className="page-subtitle">Paste a company’s careers page. The agent lists the openings and ranks them against your profile and résumé.</p></div></div>

    <div className="panel portal-auto">
      <div><strong>Find software-engineering jobs across top companies</strong><span>Connects the public career boards of 25 engineering employers (Stripe, Airbnb, Databricks, Cloudflare and more), then scans them and ranks the results in Job matches. {sourceCount} board{sourceCount === 1 ? "" : "s"} connected now.</span></div>
      <button className="button button-primary" disabled={discoveryRunning} onClick={() => void onFindEngineering()}>{discoveryRunning ? <LoaderCircle className="spin" size={15} /> : <Zap size={15} />} Find engineering jobs</button>
    </div>
    <div className="panel portal-employers">
      <strong>Big employers</strong>
      <div className="ai-chips">{EMPLOYERS.map((employer) => <button key={employer.name} type="button" className="ai-chip employer-chip" disabled={busy || !status?.available} onClick={() => pick(employer.url)}>{employer.name}</button>)}</div>
      <span className="ai-note">Google and Meta do not allow automated reading of their career sites (robots.txt), so open them yourself: <a href="https://www.google.com/about/careers/applications/jobs/results/?q=software%20engineer" target="_blank" rel="noreferrer">Google</a>, <a href="https://www.metacareers.com/jobs" target="_blank" rel="noreferrer">Meta</a>.</span>
    </div>
    <form className="panel portal-form" onSubmit={(event) => void search(event)}>
      <label className="form-field"><span>Careers page address</span><input required type="url" value={address} onChange={(event) => setAddress(event.target.value)} placeholder="https://careers.example.com/jobs" /></label>
      <label className="form-field"><span>Keywords <small>Optional</small></span><input value={keywords} onChange={(event) => setKeywords(event.target.value)} placeholder="e.g. backend, python" /></label>
      <button className="button button-primary" disabled={busy || !status?.available}>{busy ? <LoaderCircle className="spin" size={15} /> : <Search size={15} />} Find jobs</button>
    </form>
    {status && !status.available && <div className="workflow-error" role="alert">The AI service is offline. Start it with npm run dev:ai.</div>}
    {error && <div className="workflow-error" role="alert">{error}</div>}

    {result?.status === "auth_required" && <div className="panel portal-auth">
      <ShieldAlert size={22} />
      <div>
        <strong>{result.host} needs you to sign in (HTTP {result.httpStatus})</strong>
        <span>Sign in with Google in the window that opens. You type your credentials there, never here; CareerPilot only keeps the browser session on this computer. Close the window when done, then search again.</span>
        <div className="ai-actions">
          <button className="button button-primary" onClick={() => void signIn()}><LogIn size={15} /> Sign in with Google</button>
          <button className="button button-secondary" onClick={() => void search()} disabled={busy}>I’ve signed in — search again</button>
        </div>
        {session?.windowOpen && <span className="ai-note">A sign-in window is open.</span>}
      </div>
    </div>}

    {result?.status === "ok" && <div className="panel ai-panel-pad">
      <div className="workflow-run-head"><strong>{result.jobs.length} opening{result.jobs.length === 1 ? "" : "s"} on {result.host}</strong><span className="ai-note">Ranked by fit to your profile</span></div>
      {result.message && <p className="ai-note">{result.message}</p>}
      {applyNote && <p className="ai-note" role="status">{applyNote}</p>}
      {result.jobs.map((job) => <div className="ai-job" key={job.url + job.title}>
        <div className="ai-job-main">
          <strong><a href={job.url} target="_blank" rel="noreferrer">{job.title}</a></strong>
          {job.snippet && <span className="ai-note">{job.snippet}</span>}
          <div className="ai-chips">{job.skills.map((skill) => <span key={skill} className="ai-chip">{skill}</span>)}</div>
        </div>
        <div className="ai-job-score"><strong>{job.score}%</strong>
          <div className="ai-actions">
            <button className="button button-secondary" onClick={() => { track(job); }}><Plus size={14} /> Add</button>
            <button className="button button-primary" disabled={Boolean(applying)} onClick={() => void applyNow(job)}>{applying === job.url ? <LoaderCircle className="spin" size={14} /> : <ExternalLink size={14} />} Apply now</button>
          </div>
        </div>
      </div>)}
      {result.jobs.length > 0 && <p className="ai-note"><Sparkles size={12} /> Added jobs appear in Job matches; open them in the Application copilot to draft an application.{" "}<button className="text-button" onClick={onOpenCopilot}>Open copilot</button></p>}
    </div>}
  </>;
}

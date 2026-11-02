import { useState } from "react";
import {
  ArrowUpRight,
  Bookmark,
  BriefcaseBusiness,
  Building2,
  Check,
  CheckCircle2,
  CircleHelp,
  MapPin,
  X,
} from "lucide-react";
import { CompanyLogo } from "../CompanyLogo";
import { apiRequest } from "../../lib/api";
import { Job, JobStatus } from "../../types";

export function JobDetailsModal({ job, onClose, onUpdateStatus }: { job: Job; onClose: () => void; onUpdateStatus: (id: string, status: JobStatus) => void }) {
  const [preparation, setPreparation] = useState<{
    readyToPrepare: boolean;
    submissionSupported: boolean;
    requiresUserApproval: boolean;
    checks: { name: string; ready: boolean; detail: string }[];
  } | null>(null);
  const [preparing, setPreparing] = useState(false);
  const [preparationError, setPreparationError] = useState("");
  const canSave = job.status === "New" || job.status === "Review";
  const checkReadiness = async () => {
    setPreparing(true);
    setPreparationError("");
    try {
      const result = await apiRequest<typeof preparation>("/api/applications/prepare", {
        method: "POST",
        body: JSON.stringify({ job }),
      });
      setPreparation(result);
    } catch (error) {
      setPreparationError(error instanceof Error ? error.message : "Could not check application readiness.");
    } finally {
      setPreparing(false);
    }
  };
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><section className="modal-card" aria-labelledby="job-details-title"><div className="modal-heading"><div className="details-company"><CompanyLogo job={job} /><div><div className="section-kicker">{job.source.toUpperCase()} · {job.posted.toUpperCase()}</div><h2 id="job-details-title">{job.company}</h2></div></div><button type="button" className="icon-button" aria-label="Close dialog" onClick={onClose}><X size={18} /></button></div><h3 className="details-role">{job.role}</h3><div className="details-meta"><span><MapPin size={14} />{job.location}</span><span><Building2 size={14} />{job.workMode}</span><span><BriefcaseBusiness size={14} />{job.salary}</span></div><div className="details-match"><div className="match-score"><span>{job.score || "—"}{job.score ? "%" : ""}</span><small>profile match</small></div><p>{job.score ? `Strong overlap with your experience in ${job.tags.slice(0, 2).join(" and ")}.` : "This opportunity was added manually. Review the role details and update your fit assessment."}</p></div><div className="details-skills"><strong>Skills mentioned</strong><div className="skill-tags">{job.tags.map((tag) => <span key={tag}>{tag}</span>)}</div></div>{job.url && <a className="details-source-link" href={job.url} target="_blank" rel="noreferrer">Open original listing <ArrowUpRight size={14} /></a>}<div className="application-prep"><div><strong>Application preparation</strong><span>Check your saved profile and parsed resume before applying.</span></div><button type="button" className="button button-secondary" onClick={() => void checkReadiness()} disabled={preparing}>{preparing ? "Checking…" : "Check readiness"}</button></div>{preparationError && <div className="source-form-error"><CircleHelp size={14} />{preparationError}</div>}{preparation && <div className="preparation-checks">{preparation.checks.map((check) => <div key={check.name} className={check.ready ? "preparation-ready" : "preparation-blocked"}><span>{check.ready ? <CheckCircle2 size={14} /> : <CircleHelp size={14} />}</span><div><strong>{check.name}</strong><small>{check.ready ? "Ready" : check.detail}</small></div></div>)}<p>Automated submission is not enabled. You will need to review and submit the application yourself.</p></div>}<div className="details-disclaimer"><CheckCircle2 size={15} /> Nothing is submitted automatically. You decide what happens next.</div><div className="modal-actions"><button type="button" className="button button-secondary" onClick={onClose}>Close</button>{canSave && <button type="button" className="button button-primary" onClick={() => { onUpdateStatus(job.id, "Saved"); onClose(); }}><Bookmark size={15} /> Save for later</button>}</div></section></div>;
}

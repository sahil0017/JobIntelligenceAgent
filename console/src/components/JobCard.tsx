import {
  ArrowUpRight,
  Bookmark,
  Building2,
  Clock3,
  MapPin,
} from "lucide-react";
import { CompanyLogo } from "./CompanyLogo";
import { Job, JobStatus } from "../types";

export function JobCard({ job, onUpdateStatus, onNotify, onOpenDetails, compact = false }: { job: Job; onUpdateStatus: (id: string, status: JobStatus) => void; onNotify: (message: string) => void; onOpenDetails?: (job: Job) => void; compact?: boolean }) {
  const isSaved = ["Saved", "Review"].includes(job.status);
  const canSave = ["New", "Saved", "Review"].includes(job.status);
  const save = () => canSave && onUpdateStatus(job.id, isSaved && job.status === "Saved" ? "New" : "Saved");
  return <article className={`job-card ${compact ? "compact" : ""}`}>
    <div className="job-card-top"><CompanyLogo job={job} /><button className={`bookmark-button ${isSaved ? "bookmarked" : ""}`} aria-label={isSaved ? "Remove saved job" : canSave ? "Save job" : "Job is already in your application tracker"} disabled={!canSave} onClick={save}><Bookmark size={17} fill={isSaved ? "currentColor" : "none"} /></button></div>
    <div className="job-title-row"><div><h3>{job.role}</h3><div className="company-name">{job.company}</div></div><div className="match-score"><span>{job.score}%</span><small>match</small></div></div>
    <div className="job-meta"><span><MapPin size={13} />{job.location}</span><span><Building2 size={13} />{job.workMode}</span><span><Clock3 size={13} />{job.posted}</span></div>
    <div className="skill-tags">{job.tags.slice(0, 3).map((tag) => <span key={tag}>{tag}</span>)}{job.tags.length > 3 && <span>+{job.tags.length - 3}</span>}</div>
    <div className="job-card-bottom"><span className="salary-label">{job.salary}</span><button className="card-action" onClick={() => job.status === "Review" ? onUpdateStatus(job.id, "Saved") : onOpenDetails ? onOpenDetails(job) : onNotify(`Opening ${job.company} job details.`)}>{job.status === "Review" ? "Review job" : "View details"}<ArrowUpRight size={14} /></button></div>
  </article>;
}

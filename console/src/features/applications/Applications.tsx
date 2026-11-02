import {
  ArrowDownRight,
  CalendarDays,
  CheckCircle2,
  ChevronDown,
  Clock3,
  Filter,
  MoreHorizontal,
  X,
} from "lucide-react";
import { CompanyLogo } from "../../components/CompanyLogo";
import { Job, JobStatus, TrackedJob } from "../../types";

export const pipelineStages: JobStatus[] = ["Review", "Saved", "Applied", "Interview", "Offer"];

export function Applications({ jobs, trackedJobs, onUpdateStatus, onKeepClosedJob, onConfirmDeleteClosedJob }: {
  jobs: Job[];
  trackedJobs: TrackedJob[];
  onUpdateStatus: (id: string, status: JobStatus) => void;
  onKeepClosedJob: (tracked: TrackedJob) => void;
  onConfirmDeleteClosedJob: (tracked: TrackedJob) => void;
}) {
  const tracked = jobs.filter((job) => ["Review", "Saved", "Applied", "Interview", "Offer", "Rejected"].includes(job.status));
  const pendingClosures = trackedJobs.filter((trackedJob) => trackedJob.closurePending);
  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> YOUR PROGRESS</div><h1>Application tracker</h1><p className="page-subtitle">One clear view of every opportunity you’re moving forward.</p></div><button className="button button-secondary" onClick={() => window.print()}><ArrowDownRight size={16} /> Export tracker</button></div>
    {pendingClosures.length > 0 && <section className="closure-review" aria-label="Closed job review">
      <div className="closure-review-heading"><div><div className="section-icon amber"><Clock3 size={17} /></div><div><h2>Review closed listings</h2><p>These applied jobs were absent from three successful career-board checks. They remain in your tracker until you choose.</p></div></div><span>{pendingClosures.length} to review</span></div>
      <div className="closure-review-list">{pendingClosures.map((trackedJob) => <div className="closure-review-item" key={trackedJob.job.id}>
        <CompanyLogo job={trackedJob.job} small />
        <div className="closure-review-copy"><strong>{trackedJob.job.role}</strong><span>{trackedJob.job.company} · {trackedJob.lastCheckedAt ? `Last checked ${new Date(trackedJob.lastCheckedAt).toLocaleString()}` : "Closed listing"}</span><small>{trackedJob.notificationSent ? "Gmail notice sent" : trackedJob.notificationAttempted ? "Gmail send outcome uncertain; check Sent" : "Gmail notice not sent"}</small></div>
        <button className="button button-secondary" onClick={() => onKeepClosedJob(trackedJob)}>Keep</button>
        <button className="button button-danger" onClick={() => onConfirmDeleteClosedJob(trackedJob)}>Review deletion</button>
      </div>)}</div>
    </section>}
    <div className="tracker-summary"><div><strong>{tracked.length}</strong><span>Total tracked</span></div><div><strong>{jobs.filter((job) => job.status === "Interview").length}</strong><span>In interview</span></div><div><strong>{jobs.filter((job) => job.status === "Offer").length}</strong><span>Offers</span></div><div className="tracker-response"><div className="response-ring"><span>—</span></div><div><strong>Response time</strong><span>Unavailable until response dates are tracked</span></div></div></div>
    <div className="pipeline-toolbar"><div><h2>Application pipeline</h2><span>Updated just now</span></div><button className="filter-select-wrap"><Filter size={15} /> Filter <ChevronDown size={13} /></button></div>
    <div className="pipeline-board">{pipelineStages.map((stage) => {
      const stageJobs = tracked.filter((job) => job.status === stage);
      return <div className="pipeline-column" key={stage}><div className="pipeline-column-heading"><span className={`stage-dot stage-${stage.toLowerCase().replace(" ", "-")}`} /><strong>{stage === "Review" ? "To review" : stage}</strong><span className="column-count">{stageJobs.length}</span></div><div className="pipeline-cards">{stageJobs.length ? stageJobs.map((job) => <div className="pipeline-card" key={job.id}><div className="pipeline-card-header"><CompanyLogo job={job} small /><button className="more-button" aria-label={`More options for ${job.company}`}><MoreHorizontal size={17} /></button></div><strong>{job.role}</strong><span className="pipeline-company">{job.company} <i>·</i> {job.location.split(",")[0]}</span><div className="pipeline-card-bottom"><span><CalendarDays size={13} /> {job.posted}</span><span className="pipeline-score">{job.score}% fit</span></div><select aria-label={`Update ${job.company} application status`} value={job.status} onChange={(event) => onUpdateStatus(job.id, event.target.value as JobStatus)}>{["Review", "Saved", "Applied", "Interview", "Offer", "Rejected"].map((item) => <option key={item}>{item}</option>)}</select></div>) : <div className="column-empty">No applications here yet</div>}</div></div>;
    })}</div>
    <div className="tracker-footnote"><CheckCircle2 size={15} /> Applied jobs are saved in your local database. Other discovered listings stay in memory until you apply.</div>
  </>;
}

export function ConfirmClosedJobModal({ tracked, onClose, onConfirm }: { tracked: TrackedJob; onClose: () => void; onConfirm: () => void }) {
  return <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <section className="modal-card" role="dialog" aria-modal="true" aria-labelledby="closed-job-title">
      <div className="modal-heading"><div><div className="section-kicker">APPLICATION TRACKER</div><h2 id="closed-job-title">Remove this closed job?</h2></div><button className="icon-button" aria-label="Close dialog" onClick={onClose}><X size={17} /></button></div>
      <p className="modal-description"><strong>{tracked.job.role}</strong> at {tracked.job.company} was missing from three successful career-board checks. Confirming removes it and its monitoring history from your local database. This does not affect an external application.</p>
      <div className="modal-actions"><button className="button button-secondary" onClick={onClose}>Cancel</button><button className="button button-danger" onClick={onConfirm}>Confirm removal</button></div>
    </section>
  </div>;
}

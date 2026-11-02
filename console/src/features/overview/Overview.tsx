import {
  ArrowRight,
  Bookmark,
  BriefcaseBusiness,
  Check,
  CheckCircle2,
  CircleHelp,
  Clock3,
  Globe2,
  LoaderCircle,
  MoreHorizontal,
  Sparkles,
  Target,
  Zap,
} from "lucide-react";
import { CompanyLogo } from "../../components/CompanyLogo";
import { JobCard } from "../../components/JobCard";
import { StatCard } from "../../components/StatCard";
import { Applications } from "../applications/Applications";
import { Job, JobStatus, Page } from "../../types";

export type OverviewProps = {
  jobs: Job[];
  pendingCount: number;
  activeCount: number;
  savedCount: number;
  matchCount: number;
  agentRunning: boolean;
  discoveryRunning: boolean;
  sourceCount: number;
  sourceError: string;
  onNavigate: (page: Page) => void;
  onUpdateStatus: (id: string, status: JobStatus) => void;
  onRunDiscovery: () => void;
  onAddJob: () => void;
  onNotify: (message: string) => void;
  onOpenDetails: (job: Job) => void;
  onManageSources: () => void;
};

export function Overview({ jobs, pendingCount, activeCount, savedCount, matchCount, agentRunning, discoveryRunning, sourceCount, sourceError, onNavigate, onUpdateStatus, onRunDiscovery, onAddJob, onNotify, onOpenDetails, onManageSources }: OverviewProps) {
  const recommended = [...jobs].filter((job) => job.status !== "Rejected").sort((a, b) => b.score - a.score).slice(0, 3);
  const pending = jobs.filter((job) => job.status === "Review").slice(0, 2);
  const dateLabel = new Date().toLocaleDateString("en", { weekday: "long", month: "long", day: "numeric", year: "numeric" }).toUpperCase();
  const pipelineCounts = [
    { label: "Discovered", count: jobs.filter((job) => job.status === "New").length, color: "blue" },
    { label: "Saved for review", count: jobs.filter((job) => ["Saved", "Review"].includes(job.status)).length, color: "mint" },
    { label: "Applications", count: jobs.filter((job) => ["Applied", "Interview", "Offer"].includes(job.status)).length, color: "purple" },
  ];
  const maxPipelineCount = Math.max(1, ...pipelineCounts.map(({ count }) => count));
  return (
    <>
      <div className="welcome-row">
        <div><div className="eyebrow"><span className="eyebrow-line" /> {dateLabel}</div><h1>Good morning, Sahil <span className="wave">✳</span></h1><p className="page-subtitle">Here’s your career search at a glance. Your next move is closer than you think.</p></div>
        <button className="button button-primary" onClick={onRunDiscovery} disabled={!agentRunning || discoveryRunning}>{discoveryRunning ? <LoaderCircle className="spin" size={16} /> : <Sparkles size={16} />}{discoveryRunning ? "Scanning..." : "Run discovery"}</button>
      </div>
      <div className="stats-grid">
        <StatCard icon={<Target size={17} />} label="Strong matches" value={String(matchCount).padStart(2, "0")} note={<span className="neutral-note">From live career sources</span>} tone="blue" />
        <StatCard icon={<BriefcaseBusiness size={17} />} label="Active applications" value={String(activeCount).padStart(2, "0")} note={<span className="neutral-note">Tracked by you</span>} tone="green" />
        <StatCard icon={<Clock3 size={17} />} label="Needs your review" value={String(pendingCount).padStart(2, "0")} note={<span className="neutral-note">Ready when you are</span>} tone="amber" />
        <StatCard icon={<Bookmark size={17} />} label="Saved for later" value={String(savedCount).padStart(2, "0")} note={<span className="neutral-note">Across your sources</span>} tone="purple" />
      </div>

      <section className="agent-banner">
        <div className="banner-spark"><Sparkles size={21} /></div>
        <div className="banner-copy"><strong>{sourceCount ? `${sourceCount} live career source${sourceCount === 1 ? "" : "s"} connected` : "Connect a company career page to get started"}</strong><span>{jobs.length ? `${jobs.length} live listing${jobs.length === 1 ? "" : "s"} in your workspace. Discovery runs only when you ask.` : "Add a Greenhouse or Lever board, then fetch its current public openings."}</span></div>
        <button className="button button-light" onClick={() => sourceCount ? onNavigate("Job matches") : onManageSources()}>{sourceCount ? "Explore matches" : "Connect a source"}<ArrowRight size={15} /></button>
        <div className="banner-decoration"><Sparkles size={78} /></div>
      </section>
      {sourceError && <div className="source-error"><CircleHelp size={15} /><span>{sourceError}</span><button onClick={onManageSources}>Manage sources</button></div>}

      <div className="section-heading-row">
        <div><div className="section-kicker">CURATED FOR YOU</div><h2>Top job matches <span className="heading-count">{String(recommended.length).padStart(2, "0")}</span></h2></div>
        <button className="text-button" onClick={() => onNavigate("Job matches")}>View all matches <ArrowRight size={15} /></button>
      </div>
      {recommended.length ? <div className="job-card-grid">
        {recommended.map((job) => <JobCard key={job.id} job={job} onUpdateStatus={onUpdateStatus} onNotify={onNotify} onOpenDetails={onOpenDetails} compact />)}
      </div> : <div className="empty-overview"><div className="empty-state-icon"><Globe2 size={20} /></div><div><strong>No live job listings yet</strong><span>Connect a career page and run discovery to bring its current openings here.</span></div><button className="button button-secondary" onClick={onManageSources}>Manage sources</button></div>}

      <div className="lower-grid">
        <section className="panel review-panel">
          <div className="panel-heading"><div><div className="section-kicker">YOUR APPROVAL</div><h2>Ready for your review <span className="heading-count">{String(pendingCount).padStart(2, "0")}</span></h2></div><button className="more-button" aria-label="More review options" onClick={() => onNotify("Your approval queue is up to date.")}><MoreHorizontal size={19} /></button></div>
          {pending.length > 0 ? <div className="review-list">{pending.map((job) => <div className="review-item" key={job.id}>
            <CompanyLogo job={job} small />
            <div className="review-item-copy"><strong>{job.role}</strong><span>{job.company} <i>·</i> {job.location}</span></div>
            <button className="review-approve" onClick={() => onUpdateStatus(job.id, "Saved")}><Check size={14} /> Review</button>
          </div>)}</div> : <div className="empty-inline"><CheckCircle2 size={18} /> Nothing waiting for your review.</div>}
          <button className="panel-footer-link" onClick={() => onNavigate("Job matches")}>Open approval queue <ArrowRight size={14} /></button>
        </section>
        <section className="panel activity-panel">
          <div className="panel-heading"><div><div className="section-kicker">LIVE SNAPSHOT</div><h2>Your job pipeline</h2></div></div>
          <div className="pipeline-snapshot">{pipelineCounts.map((item) => <div className="snapshot-row" key={item.label}><span>{item.label}</span><div className="snapshot-track"><i className={`snapshot-bar ${item.color}`} style={{ width: `${(item.count / maxPipelineCount) * 100}%` }} /></div><strong>{item.count}</strong></div>)}</div>
          <div className="chart-legend"><span><i className="legend-dot legend-blue" /> Updated when you discover jobs</span><span className="chart-insight"><Globe2 size={13} /> {sourceCount} connected source{sourceCount === 1 ? "" : "s"}</span></div>
        </section>
      </div>

      <div className="bottom-note"><div className="note-icon"><Zap size={15} /></div><span><strong>Your profile guides matching.</strong> Scores use keyword overlap with your listed skills and target role; they are not an AI judgment.</span><button onClick={() => onNavigate("My profile")}>Update profile <ArrowRight size={13} /></button></div>
    </>
  );
}

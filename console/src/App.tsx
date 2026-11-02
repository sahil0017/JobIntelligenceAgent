import { useEffect, useMemo, useRef, useState } from "react";
import {
  Bell,
  BriefcaseBusiness,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleHelp,
  Compass,
  FileText,
  LayoutDashboard,
  MapPin,
  Pause,
  Play,
  Settings2,
  Workflow,
  Zap,
  X,
  BrainCircuit,
  Sparkles,
} from "lucide-react";
import { AIAnalysis } from "./features/ai/AIAnalysis";
import { ApplicationCopilot } from "./features/ai/ApplicationCopilot";
import { ProcessingWorkflow } from "./features/ai/ProcessingWorkflow";
import { AddJobModal } from "./components/modals/AddJobModal";
import { JobDetailsModal } from "./components/modals/JobDetailsModal";
import { SourcesModal } from "./components/modals/SourcesModal";
import { AgentActivity } from "./features/agent/AgentActivity";
import { AgentJob } from "./features/agent/AgentWorkflow";
import { Applications, ConfirmClosedJobModal } from "./features/applications/Applications";
import { AreaJobSearch } from "./features/jobs/AreaJobSearch";
import { JobMatches } from "./features/jobs/JobMatches";
import { HubTabs, hubFor } from "./components/HubTabs";
import { AgentConfig } from "./features/ai/AgentConfig";
import { PortalSearch } from "./features/ai/PortalSearch";
import { AIAdmin } from "./features/admin/AIAdmin";
import { Overview } from "./features/overview/Overview";
import { Profile } from "./features/profile/Profile";
import { Settings } from "./features/settings/Settings";
import { apiRequest, readAPIResponse } from "./lib/api";
import { defaultProfile, initialJobs } from "./lib/defaults";
import { useStoredState } from "./lib/useStoredState";
import { CareerSource, DiscoveryEvent, Job, JobStatus, Page, TrackedJob, UserProfile } from "./types";

const navItems: { label: string; target: Page; icon: typeof LayoutDashboard; hub?: string }[] = [
  { label: "Overview", target: "Overview", icon: LayoutDashboard },
  { label: "Jobs", target: "Job matches", icon: Compass, hub: "jobs" },
  { label: "Applications", target: "Applications", icon: BriefcaseBusiness },
  { label: "Agents", target: "Application copilot", icon: Sparkles, hub: "agents" },
  { label: "Admin", target: "My profile", icon: Settings2, hub: "admin" },
];

function App() {
  const [page, setPage] = useState<Page>("Overview");
  const [aiRunId, setAiRunId] = useState<string | null>(null);
  const [jobs, setJobs] = useState<Job[]>(initialJobs);
  const [trackedJobs, setTrackedJobs] = useState<TrackedJob[]>([]);
  const [pendingDeleteJob, setPendingDeleteJob] = useState<TrackedJob | null>(null);
  const [agentRunning, setAgentRunning] = useStoredState("careerpilot.agentRunning", true);
  const [profileSaved, setProfileSaved] = useState(false);
  const [search, setSearch] = useState("");
  const [jobFilter, setJobFilter] = useState("All matches");
  const [jobAreaFilter, setJobAreaFilter] = useState("");
  const [jobAreaState, setJobAreaState] = useState("");
  const [jobAreaCheckedSources, setJobAreaCheckedSources] = useState(0);
  const [includeRemoteListings, setIncludeRemoteListings] = useState(true);
  const [jobRemoteRegion, setJobRemoteRegion] = useState("");
  const [showAddJob, setShowAddJob] = useState(false);
  const [discoveryRunning, setDiscoveryRunning] = useState(false);
  const [toast, setToast] = useState("");
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [sources, setSources] = useState<CareerSource[]>([]);
  const [sourceError, setSourceError] = useState("");
  const [showSources, setShowSources] = useState(false);
  const [discoveryHistory, setDiscoveryHistory] = useStoredState<DiscoveryEvent[]>("careerpilot.discoveryHistory", []);
  const [profile, setProfile] = useStoredState<UserProfile>("careerpilot.profile", defaultProfile);
  const [profileLoaded, setProfileLoaded] = useState(false);
  const [resumeName, setResumeName] = useState("");

  const filteredJobs = useMemo(() => jobs.filter((job) => {
    const matchesText = `${job.company} ${job.role} ${job.location} ${job.tags.join(" ")}`.toLowerCase().includes(search.toLowerCase());
    const matchesFilter = jobFilter === "All matches" || (jobFilter === "90%+ match" ? job.score >= 90 : job.status === jobFilter);
    const lowerLocation = job.location.toLowerCase();
    const matchesArea = !jobAreaFilter || lowerLocation.includes(jobAreaFilter.toLowerCase()) ||
      (includeRemoteListings && jobRemoteRegion.length > 0 &&
        (lowerLocation.includes("remote") || lowerLocation.includes("distributed")) &&
        lowerLocation.includes(jobRemoteRegion.toLowerCase()));
    return matchesText && matchesFilter && matchesArea;
  }), [jobs, search, jobFilter, jobAreaFilter, includeRemoteListings, jobRemoteRegion]);

  const notify = (message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(""), 3000);
  };

  const refreshTrackedJobs = async () => {
    const result = await apiRequest<{ trackedJobs: TrackedJob[] }>("/api/applied-jobs");
    setTrackedJobs(result.trackedJobs);
    setJobs((current) => {
      const trackedById = new Map(result.trackedJobs.map(({ job }) => [job.id, job]));
      const refreshed = result.trackedJobs.map(({ job }) => job);
      return [...refreshed, ...current.filter((job) => !trackedById.has(job.id))];
    });
  };

  useEffect(() => {
    let active = true;
    const refresh = async () => {
      try {
        const result = await apiRequest<{ trackedJobs: TrackedJob[] }>("/api/applied-jobs");
        if (!active) return;
        setTrackedJobs(result.trackedJobs);
        setJobs((current) => {
          const trackedById = new Map(result.trackedJobs.map(({ job }) => [job.id, job]));
          return [...result.trackedJobs.map(({ job }) => job), ...current.filter((job) => !trackedById.has(job.id))];
        });
      } catch (error) {
        if (active) notify(error instanceof Error ? `Could not load applied jobs: ${error.message}` : "Could not load applied jobs.");
      }
    };
    void refresh();
    const interval = window.setInterval(() => { void refresh(); }, 60_000);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    apiRequest<{ sources: CareerSource[] }>("/api/sources")
      .then(({ sources: configuredSources }) => setSources(configuredSources))
      .catch((error: unknown) => setSourceError(error instanceof Error ? error.message : "Could not load career sources."));
  }, []);

  useEffect(() => {
    Promise.all([
      apiRequest<UserProfile>("/api/profile"),
      apiRequest<{ filename: string }>("/api/resume"),
    ]).then(async ([savedProfile, resume]) => {
      if (savedProfile.name || savedProfile.email || savedProfile.skills) {
        setProfile(savedProfile);
      } else {
        await apiRequest<UserProfile>("/api/profile", {
          method: "PUT",
          body: JSON.stringify(profile),
        });
      }
      setResumeName(resume.filename ?? "");
      setProfileLoaded(true);
    }).catch((error: unknown) => {
      setSourceError(error instanceof Error ? error.message : "Could not load your saved profile.");
      setProfileLoaded(true);
    });
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const connected = params.get("connected");
    if (connected === "gmail" || connected === "linkedin") {
      notify(`${connected === "gmail" ? "Gmail" : "LinkedIn"} connected.`);
      setPage("Settings");
      window.history.replaceState({}, "", window.location.pathname);
    }
  }, []);

  const updateStatus = async (id: string, status: JobStatus) => {
    const job = jobs.find((item) => item.id === id);
    if (!job) return;
    const updatedJob = { ...job, status };
    const alreadyTracked = trackedJobs.some((tracked) => tracked.job.id === id);
    try {
      if (alreadyTracked) {
        await apiRequest(`/api/applied-jobs/${encodeURIComponent(id)}`, {
          method: "PUT",
          body: JSON.stringify({ job: updatedJob }),
        });
      } else if (status === "Applied") {
        await apiRequest("/api/applied-jobs", {
          method: "POST",
          body: JSON.stringify({ job: updatedJob }),
        });
      }
      setJobs((current) => current.map((item) => item.id === id ? updatedJob : item));
      notify(`${job.company} moved to ${status.toLowerCase()}.`);
      if (alreadyTracked || status === "Applied") {
        try {
          await refreshTrackedJobs();
        } catch (error) {
          notify(error instanceof Error ? `Status saved, but applied jobs could not refresh: ${error.message}` : "Status saved, but applied jobs could not refresh.");
        }
      }
    } catch (error) {
      notify(error instanceof Error ? `Could not update application status: ${error.message}` : "Could not update application status.");
    }
  };

  const keepClosedJob = async (tracked: TrackedJob) => {
    try {
      await apiRequest(`/api/applied-jobs/${encodeURIComponent(tracked.job.id)}/keep`, { method: "POST" });
      await refreshTrackedJobs();
      notify(`${tracked.job.company} will stay in your application tracker.`);
    } catch (error) {
      notify(error instanceof Error ? `Could not keep this job: ${error.message}` : "Could not keep this job.");
    }
  };

  const confirmDeleteClosedJob = async () => {
    if (!pendingDeleteJob) return;
    const { job } = pendingDeleteJob;
    try {
      await apiRequest(`/api/applied-jobs/${encodeURIComponent(job.id)}`, { method: "DELETE" });
      setPendingDeleteJob(null);
      setJobs((current) => current.filter((item) => item.id !== job.id));
      await refreshTrackedJobs();
      notify(`${job.company} was removed from your application tracker.`);
    } catch (error) {
      notify(error instanceof Error ? `Could not remove this job: ${error.message}` : "Could not remove this job.");
    }
  };

  const addAgentJobs = (found: AgentJob[]) => {
    setJobs((current) => {
      const existingIds = new Set(current.map((job) => job.id));
      const fresh = found.filter((job) => !existingIds.has(job.id)).map((job) => ({ ...job, status: "New" as JobStatus }));
      return [...fresh, ...current];
    });
  };

  const runDiscovery = async (area = "", includeRemote = true, remoteRegion = "", selectedState = "") => {
    if (!agentRunning && !area) {
      notify("Resume your agent before starting discovery.");
      return;
    }
    if (!area && sources.length === 0) {
      notify("Add a career source before running discovery.");
      setShowSources(true);
      return;
    }
    setDiscoveryRunning(true);
    setSourceError("");
    try {
      const query = new URLSearchParams({
        skills: profile.skills,
        title: profile.title,
        location: area || profile.location,
      });
      if (area) {
        query.set("area", area);
        query.set("includeRemote", String(includeRemote));
        query.set("remoteRegion", remoteRegion);
      }
      const result = area
        ? await apiRequest<{ jobs: Job[]; sourceErrors: { company?: string; url?: string; error: string; category?: string }[]; checkedSources: number; filteredLowMatch?: number }>("/api/area-jobs", {
          method: "POST",
          body: JSON.stringify({ country: remoteRegion, state: selectedState, area, includeRemote }),
        })
        : await apiRequest<{ jobs: Job[]; sourceErrors: { company?: string; url?: string; error: string; category?: string }[]; checkedSources: number; filteredLowMatch?: number }>(`/api/jobs?${query}`);
      if (area) {
        setJobAreaFilter(area);
        setJobAreaState(selectedState);
        setJobAreaCheckedSources(result.checkedSources);
        setIncludeRemoteListings(includeRemote);
        setJobRemoteRegion(remoteRegion);
        setSearch("");
        setJobFilter("All matches");
      }
      setJobs((current) => {
        const existingById = new Map(current.map((job) => [job.id, job]));
        const refreshed = result.jobs.map((job) => ({ ...job, status: existingById.get(job.id)?.status ?? "New" as JobStatus }));
        const discoveredIds = new Set(refreshed.map((job) => job.id));
        if (area) {
          const trackedOutsideArea = trackedJobs
            .map(({ job }) => job)
            .filter((job) => !discoveredIds.has(job.id));
          return [...refreshed, ...trackedOutsideArea];
        }
        return [...refreshed, ...current.filter((job) => !discoveredIds.has(job.id))];
      });
      const reasonLabels: Record<string, string> = {
        auth_required: "need a login (skipped)", closed: "closed or removed", robots: "blocked by robots.txt",
        redirect: "redirected elsewhere", no_data: "had no structured postings", error: "could not be read",
      };
      const reasonCounts = new Map<string, number>();
      result.sourceErrors.forEach(({ category }) => reasonCounts.set(category ?? "error", (reasonCounts.get(category ?? "error") ?? 0) + 1));
      const reasonSummary = [...reasonCounts].map(([category, count]) => `${count} ${reasonLabels[category] ?? category}`).join(", ");
      const lowMatchNote = result.filteredLowMatch ? `${result.filteredLowMatch} weak resume match${result.filteredLowMatch === 1 ? "" : "es"} filtered out. ` : "";
      setSourceError(area
        ? `${lowMatchNote}${reasonSummary ? `Skipped pages: ${reasonSummary}. ` : ""}${result.sourceErrors.map(({ company, url, error }) => `${company ?? url ?? "Source"}: ${error}`).join(" · ")}`.trim()
        : result.sourceErrors.map(({ company, url, error }) => `${company ?? url ?? "Source"}: ${error}`).join(" · "));
      setDiscoveryHistory((current) => [{
        id: `discovery-${Date.now()}`,
        time: new Date().toLocaleString(),
        title: area ? `Searched ${result.checkedSources} public career page${result.checkedSources === 1 ? "" : "s"}` : `Checked ${result.checkedSources} career source${result.checkedSources === 1 ? "" : "s"}`,
        detail: `${result.jobs.length} live listing${result.jobs.length === 1 ? "" : "s"} found and matched against your profile.${result.sourceErrors.length ? ` ${result.sourceErrors.length} page${result.sourceErrors.length === 1 ? "" : "s"} were skipped or had no verified postings.` : ""}`,
        badge: result.sourceErrors.length ? `${result.sourceErrors.length} source errors` : `${result.jobs.length} jobs`,
      }, ...current].slice(0, 30));
      notify(result.sourceErrors.length
        ? `Found ${result.jobs.length} live ${area ? `openings in ${area}` : "jobs"}; ${result.sourceErrors.length} page(s) were skipped or had no verified listings.`
        : area
          ? `Found ${result.jobs.length} current openings in ${area}; checked ${result.checkedSources} public career page(s).`
          : `Discovery complete — found ${result.jobs.length} live jobs.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Job discovery failed.";
      setSourceError(message);
      notify(`Discovery failed: ${message}`);
    } finally {
      setDiscoveryRunning(false);
    }
  };

  const saveProfile = async () => {
    try {
      await apiRequest<UserProfile>("/api/profile", {
        method: "PUT",
        body: JSON.stringify(profile),
      });
      setProfileSaved(true);
      notify("Your career profile is saved to the local database.");
      window.setTimeout(() => setProfileSaved(false), 2200);
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not save your profile.");
    }
  };

  const uploadResume = async (file: File) => {
    const form = new FormData();
    form.append("resume", file);
    try {
      const response = await fetch("/api/resume", { method: "POST", body: form });
      const payload = await readAPIResponse<{ filename?: string }>(response, "/api/resume");
      setResumeName(payload.filename ?? file.name);
      notify(`${file.name} uploaded and parsed. Encrypted resume text was saved locally.`);
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not parse your resume.");
    }
  };

  const removeResume = async () => {
    try {
      await apiRequest<{ deleted: boolean }>("/api/resume", { method: "DELETE" });
      setResumeName("");
      notify("Resume removed from the local database.");
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not remove your resume.");
    }
  };

  const addJob = (job: Job) => {
    setJobs((current) => [job, ...current]);
    setShowAddJob(false);
    notify("Job added to your matches.");
  };

  const runDiscoveryRef = useRef<() => Promise<void>>(async () => undefined);
  runDiscoveryRef.current = () => runDiscovery();
  const autoScanned = useRef(false);
  useEffect(() => {
    if (autoScanned.current || !profileLoaded || sources.length === 0 || !agentRunning) return;
    autoScanned.current = true;
    void runDiscovery();
    const timer = window.setInterval(() => { void runDiscoveryRef.current(); }, 30 * 60_000);
    return () => window.clearInterval(timer);
  }, [profileLoaded, sources.length, agentRunning]);

  const connectEngineeringBoards = async () => {
    try {
      const result = await apiRequest<{ added: string[]; skipped: string[]; unavailable: string[]; total: number }>("/api/sources-catalog", { method: "POST" });
      const refreshed = await apiRequest<{ sources: CareerSource[] }>("/api/sources");
      setSources(refreshed.sources);
      notify(`${result.added.length} new engineering career boards connected (${result.skipped.length} already connected). Scanning for jobs…`);
      await runDiscovery();
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not connect the engineering career boards.");
    }
  };

  const addSource = async (input: Omit<CareerSource, "id">) => {
    const result = await apiRequest<{ source: CareerSource }>("/api/sources", {
      method: "POST",
      body: JSON.stringify(input),
    });
    setSources((current) => [...current, result.source]);
    notify(`${result.source.company} career page connected.`);
  };

  const removeSource = async (source: CareerSource) => {
    const result = await apiRequest<{ sources: CareerSource[] }>(`/api/sources/${encodeURIComponent(source.id)}`, { method: "DELETE" });
    setSources(result.sources);
    notify(`${source.company} career page disconnected.`);
  };

  const pendingCount = jobs.filter((job) => job.status === "Review").length;
  const activeCount = jobs.filter((job) => ["Applied", "Interview", "Offer"].includes(job.status)).length;
  const savedCount = jobs.filter((job) => job.status === "Saved").length;
  const matchCount = jobs.filter((job) => job.score >= 80 && !["Rejected"].includes(job.status)).length;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark"><Compass size={19} strokeWidth={2.5} /></div>
          <span>career<span className="brand-light">pilot</span><sup>AI</sup></span>
        </div>
        <button className="profile-switcher" onClick={() => setPage("My profile")}>
          <div className="avatar">SK</div>
          <div className="profile-switcher-copy"><strong>{profile.name}</strong><span>Personal workspace</span></div>
          <ChevronDown size={15} />
        </button>
        <nav className="nav-list" aria-label="Main navigation">
          {navItems.map(({ label, target, icon: Icon, hub }) => {
            const active = hub ? hubFor(page)?.id === hub : page === target;
            return <button key={label} className={`nav-item ${active ? "active" : ""}`} onClick={() => setPage(target)}>
              <Icon size={18} /><span>{label}</span>
              {label === "Jobs" && <span className="nav-count">{matchCount}</span>}
              {label === "Applications" && <span className="nav-count">{activeCount}</span>}
            </button>;
          })}
        </nav>
        <div className="sidebar-bottom">
          <div className="agent-mini">
            <div className="agent-mini-heading"><span className={`agent-status-dot ${agentRunning ? "" : "paused"}`} />Agent {agentRunning ? "is ready" : "is paused"}</div>
            <p>{agentRunning ? "Ready to scan connected boards." : "Your agent is taking a break."}</p>
            <button onClick={() => setAgentRunning(!agentRunning)}>{agentRunning ? <><Pause size={13} /> Pause agent</> : <><Play size={13} /> Resume agent</>}</button>
          </div>
          <div className="sidebar-footer"><CircleHelp size={15} /> Help center <span>·</span> v0.1.0</div>
        </div>
      </aside>

      <main className="main-area">
        <header className="topbar">
          <div className="breadcrumb"><span>{hubFor(page)?.label ?? "Workspace"}</span><ChevronRight size={14} /><strong>{page}</strong></div>
          <div className="topbar-actions">
            <div className="agent-live-label"><span className={`agent-status-dot ${agentRunning ? "" : "paused"}`} />{agentRunning ? "Agent ready" : "Agent paused"}</div>
            <button className="icon-button notification-button" aria-label="Notifications" onClick={() => notify("You’re all caught up.")}><Bell size={18} /><i /></button>
            <button className="top-avatar" onClick={() => setPage("My profile")}>SK</button>
          </div>
        </header>

        <div className="page-content">
          <HubTabs page={page} onNavigate={setPage} />
          {hubFor(page)?.id === "agents" && <AgentConfig hasProfile={Boolean(profile.skills.trim() || profile.title.trim())} resumeName={resumeName} onNavigate={setPage} />}
          {page === "Company portals" && <PortalSearch onFindEngineering={connectEngineeringBoards} discoveryRunning={discoveryRunning} sourceCount={sources.length} onAddJob={addJob} onOpenCopilot={() => setPage("Application copilot")} onNotify={notify} />}
          {page === "AI & sign-ins" && <AIAdmin onNotify={notify} />}
          {page === "Overview" && <Overview
            jobs={jobs}
            pendingCount={pendingCount}
            activeCount={activeCount}
            savedCount={savedCount}
            matchCount={matchCount}
            agentRunning={agentRunning}
            discoveryRunning={discoveryRunning}
            sourceCount={sources.length}
            onNavigate={setPage}
            onUpdateStatus={updateStatus}
            onRunDiscovery={() => { void runDiscovery(); }}
            onAddJob={() => setShowAddJob(true)}
            onNotify={notify}
            onOpenDetails={setSelectedJob}
            sourceError={sourceError}
            onManageSources={() => setShowSources(true)}
          />}
          {page === "Job matches" && <JobMatches
            jobs={filteredJobs}
            search={search}
            setSearch={setSearch}
            jobFilter={jobFilter}
            setJobFilter={setJobFilter}
            onUpdateStatus={updateStatus}
            onRunDiscovery={() => jobAreaFilter ? runDiscovery(jobAreaFilter, includeRemoteListings, jobRemoteRegion, jobAreaState) : runDiscovery()}
            discoveryRunning={discoveryRunning}
            onAddJob={() => setShowAddJob(true)}
            onOpenDetails={setSelectedJob}
            onNotify={notify}
            sourceError={sourceError}
            sources={sources}
            agentRunning={agentRunning}
            activeArea={jobAreaFilter}
            includeRemoteListings={includeRemoteListings}
            remoteRegion={jobRemoteRegion}
            checkedAreaSources={jobAreaCheckedSources}
            onClearArea={() => setJobAreaFilter("")}
            onManageSources={() => setShowSources(true)}
            onOpenAreaSearch={() => setPage("Area job search")}
          />}
          {page === "Area job search" && <AreaJobSearch
            onOpenSettings={() => setPage("Settings")}
            onFindJobs={(country, state, area, includeRemote) => { void runDiscovery(area, includeRemote, country, state); }}
            onOpenJobs={() => setPage("Job matches")}
            resumeName={resumeName}
            discoveryRunning={discoveryRunning}
          />}
          {page === "Applications" && <Applications jobs={jobs} trackedJobs={trackedJobs} onUpdateStatus={updateStatus} onKeepClosedJob={keepClosedJob} onConfirmDeleteClosedJob={setPendingDeleteJob} />}
          {page === "My profile" && <Profile
            profile={profile}
            setProfile={setProfile}
            profileSaved={profileSaved}
            profileLoaded={profileLoaded}
            resumeName={resumeName}
            onUploadResume={uploadResume}
            onRemoveResume={removeResume}
            onSave={saveProfile}
          />}
          {page === "AI analysis" && <AIAnalysis jobs={jobs} runId={aiRunId} onRunStarted={setAiRunId} onOpenWorkflow={() => setPage("Processing workflow")} onOpenMatches={() => setPage("Job matches")} />}
          {page === "Application copilot" && <ApplicationCopilot jobs={jobs} runId={aiRunId} onRunStarted={setAiRunId} onOpenWorkflow={() => setPage("Processing workflow")} />}
          {page === "Processing workflow" && <ProcessingWorkflow selectedRunId={aiRunId} onSelectRun={setAiRunId} onOpenAnalysis={() => setPage("AI analysis")} />}
          {page === "Agent activity" && <AgentActivity agentRunning={agentRunning} setAgentRunning={setAgentRunning} onRunDiscovery={() => { void runDiscovery(); }} discoveryRunning={discoveryRunning} history={discoveryHistory} sources={sources} defaultCountry={jobRemoteRegion || "India"} defaultState={jobAreaState} defaultArea={jobAreaFilter || profile.location.split(",")[0].trim()} onAddJobs={addAgentJobs} />}
          {page === "Settings" && <Settings onNotify={notify} profile={profile} />}
        </div>
      </main>
      {toast && <div className="toast"><CheckCircle2 size={17} />{toast}<button aria-label="Dismiss notification" onClick={() => setToast("")}><X size={15} /></button></div>}
      {showAddJob && <AddJobModal onClose={() => setShowAddJob(false)} onAdd={addJob} />}
      {selectedJob && <JobDetailsModal job={selectedJob} onClose={() => setSelectedJob(null)} onUpdateStatus={updateStatus} />}
      {showSources && <SourcesModal sources={sources} onClose={() => setShowSources(false)} onAdd={addSource} onRemove={removeSource} />}
      {pendingDeleteJob && <ConfirmClosedJobModal tracked={pendingDeleteJob} onClose={() => setPendingDeleteJob(null)} onConfirm={confirmDeleteClosedJob} />}
    </div>
  );
}

export default App;

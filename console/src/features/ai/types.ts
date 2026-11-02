export type AIStatus = {
  available: boolean;
  message?: string;
  engine?: string;
  deepLearning?: boolean;
  llm?: string;
  llmRemote?: boolean;
  scraper?: string;
  memory?: string;
  tracing?: boolean;
  keyExpiresInDays?: number | null;
};

export type StepStatus = "pending" | "running" | "done" | "skipped" | "failed";
export type RunState = "running" | "awaiting_approval" | "completed" | "failed" | "cancelled";
export type RunKind = "analysis" | "application";

export type PipelineStep = { id: string; title: string; description: string; kind: string; agent?: string; after?: string[] };
export type RunStep = { id: string; title: string; description: string; status: StepStatus; detail: string; durationMs: number; kind?: string; agent?: string; after?: string[] };
export type TraceEntry = { time: string; stage: string; message: string };

export type JobAnalysis = {
  id: string;
  company: string;
  role: string;
  rank: number;
  fitScore: number;
  semanticScore: number;
  skillCoverage: number | null;
  skills: { matched: string[]; missing: string[]; coverage: number };
  verdict: "strong" | "good" | "stretch" | "weak";
};

export type AnalysisResult = {
  engine: string;
  jobs: JobAnalysis[];
  candidateSkills: string[];
  learningPriorities: { skill: string; jobs: number }[];
  summary: string;
};

export type Draft = {
  status: "awaiting_review" | "approved" | "rejected" | "not_recommended";
  company: string;
  role: string;
  fitScore: number;
  verdict: string;
  skills: { matched: string[]; missing: string[]; coverage: number } | null;
  tailoredBullets: string[];
  coverNote: string;
  outreachMessage: string;
  warnings: string[];
  sources: string[];
  pageTitle: string;
  pageMethod: string;
  writer: string;
  research: string;
  strategy: string;
  critique: string;
};

export type AIRun = {
  id: string;
  kind?: RunKind;
  title?: string;
  draft?: Draft | null;
  status: RunState;
  startedAt: string;
  finishedAt: string | null;
  jobCount: number;
  steps: RunStep[];
  trace: TraceEntry[];
  result: AnalysisResult | null;
  error: string;
};

export type PortalJob = { title: string; url: string; snippet: string; score: number; skills: string[] };
export type PortalResult = { status: "ok" | "auth_required"; host: string; httpStatus: number; message: string; jobs: PortalJob[] };
export type PortalSession = { signedIn: boolean; windowOpen: boolean; message: string };

import { Applications } from "./features/applications/Applications";
import { Overview } from "./features/overview/Overview";
import { Settings } from "./features/settings/Settings";

export type JobStatus = "New" | "Review" | "Saved" | "Applied" | "Interview" | "Offer" | "Rejected";

export type Job = {
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
  status: JobStatus;
  logo: string;
  logoTone: string;
  sourceProvider?: string;
  sourceBoardId?: string;
};

export type TrackedJob = {
  job: Job;
  sourceProvider: string;
  sourceBoardId: string;
  appliedAt: string;
  lastCheckedAt?: string;
  missingSuccessfulScans: number;
  closurePending: boolean;
  notificationAttempted: boolean;
  notificationSent: boolean;
};

export type CareerSource = {
  id: string;
  provider: "greenhouse" | "lever";
  company: string;
  boardId: string;
};

export type DiscoveryEvent = {
  id: string;
  time: string;
  title: string;
  detail: string;
  badge: string;
};

export type Page = "Overview" | "Job matches" | "Area job search" | "Applications" | "My profile" | "Agent activity" | "AI analysis" | "Application copilot" | "Processing workflow" | "Settings" | "Company portals" | "AI & sign-ins";

export type UserProfile = {
  name: string;
  title: string;
  email: string;
  location: string;
  experience: string;
  skills: string;
  roles: string;
};

export type PortalConfig = {
  id: string;
  name: string;
  provider: string;
  url: string;
  notes: string;
  status: string;
};

export type IntegrationSettings = {
  configured: boolean;
  connected: boolean;
  clientId: string;
  redirectUrl: string;
  secretConfigured: boolean;
  scopes: string[];
  profile?: { name?: string; email?: string };
};

export type SettingsPayload = {
  integrations: { gmail: IntegrationSettings; linkedin: IntegrationSettings };
  portals: PortalConfig[];
  webSearch: { configured: boolean };
};

export type GmailMessage = {
  id: string;
  from: string;
  subject: string;
  date: string;
  snippet: string;
  body: string;
};

export type GmailThread = {
  id: string;
  snippet: string;
  messages: GmailMessage[];
};

export type OutreachDraft = {
  recipientType: "Hiring manager" | "HR";
  to: string;
  subject: string;
  body: string;
  sent: boolean;
};

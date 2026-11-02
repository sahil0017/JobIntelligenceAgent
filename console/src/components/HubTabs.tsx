import { Page } from "../types";

export type Hub = { id: string; label: string; tabs: { page: Page; label: string }[] };

export const hubs: Hub[] = [
  { id: "jobs", label: "Jobs", tabs: [{ page: "Job matches", label: "Matches" }, { page: "Area job search", label: "Area search" }, { page: "Company portals", label: "Company portals" }] },
  { id: "agents", label: "Agents", tabs: [{ page: "Application copilot", label: "Application copilot" }, { page: "AI analysis", label: "Job analysis" }, { page: "Processing workflow", label: "Workflow" }, { page: "Agent activity", label: "Job scout" }] },
  { id: "admin", label: "Admin", tabs: [{ page: "My profile", label: "Profile & résumé" }, { page: "Settings", label: "Sources & integrations" }, { page: "AI & sign-ins", label: "AI & sign-ins" }] },
];

export function hubFor(page: Page): Hub | undefined {
  return hubs.find((hub) => hub.tabs.some((tab) => tab.page === page));
}

export function HubTabs({ page, onNavigate }: { page: Page; onNavigate: (page: Page) => void }) {
  const hub = hubFor(page);
  if (!hub) return null;
  return <div className="hub-tabs" role="tablist" aria-label={hub.label}>
    {hub.tabs.map((tab) => <button key={tab.page} role="tab" aria-selected={page === tab.page} className={`hub-tab ${page === tab.page ? "active" : ""}`} onClick={() => onNavigate(tab.page)}>{tab.label}</button>)}
  </div>;
}

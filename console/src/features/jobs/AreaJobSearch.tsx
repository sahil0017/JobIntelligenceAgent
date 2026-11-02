import { useEffect, useState } from "react";
import {
  CheckCircle2,
  CircleHelp,
  Compass,
  LoaderCircle,
  Search,
  ShieldCheck,
} from "lucide-react";
import { Settings } from "../settings/Settings";
import { apiRequest } from "../../lib/api";

export function AreaJobSearch({ onOpenSettings, onFindJobs, onOpenJobs, resumeName, discoveryRunning }: {
  onOpenSettings: () => void;
  onFindJobs: (country: string, state: string, area: string, includeRemote: boolean) => void;
  onOpenJobs: () => void;
  resumeName: string;
  discoveryRunning: boolean;
}) {
  const [country, setCountry] = useState("India");
  const [state, setState] = useState("Karnataka");
  const [area, setArea] = useState("Bengaluru");
  const [includeRemote, setIncludeRemote] = useState(true);
  const [searchStatus, setSearchStatus] = useState<"checking" | "ready" | "setup" | "error">("checking");
  const [statusError, setStatusError] = useState("");

  const checkSearchStatus = async () => {
    setSearchStatus("checking");
    setStatusError("");
    try {
      const { configured } = await apiRequest<{ configured: boolean }>("/api/settings/web-search");
      setSearchStatus(configured ? "ready" : "setup");
    } catch (loadError) {
      setSearchStatus("error");
      setStatusError(loadError instanceof Error ? loadError.message : "Could not check web-search settings.");
    }
  };

  useEffect(() => {
    void checkSearchStatus();
  }, []);

  const searchJobs = (event: React.FormEvent) => {
    event.preventDefault();
    if (searchStatus !== "ready" || !area.trim() || !country.trim() || discoveryRunning) return;
    onFindJobs(country.trim(), state.trim(), area.trim(), includeRemote);
    onOpenJobs();
  };

  return <section className="panel area-search-panel">
    <div className="welcome-row page-title-row">
      <div><div className="eyebrow"><span className="eyebrow-line" /> LOCATION-BASED JOB DISCOVERY</div><h1>Find jobs in your area</h1><p className="page-subtitle">Search public employer career pages and match open roles against your profile and resume.</p></div>
    </div>
    <form onSubmit={(event) => searchJobs(event)}>
      <div className="company-search-heading"><div className="section-icon mint"><Compass size={17} /></div><div><h2>Your search area</h2><p>We look for software roles at companies of different sizes, including MNCs and startups.</p></div></div>
      <div className="company-search-fields area-location-fields">
        <label className="form-field"><span>Country</span><input required maxLength={80} value={country} onChange={(event) => setCountry(event.target.value)} placeholder="Country" /></label>
        <label className="form-field"><span>State / province <small>(optional)</small></span><input maxLength={80} value={state} onChange={(event) => setState(event.target.value)} placeholder="State or province" /></label>
        <label className="form-field"><span>City / area</span><input required maxLength={100} value={area} onChange={(event) => setArea(event.target.value)} placeholder="City or neighborhood" /></label>
      </div>
      <label className="area-remote-toggle area-search-remote"><input type="checkbox" checked={includeRemote} onChange={(event) => setIncludeRemote(event.target.checked)} /> Include remote roles open to {country || "your country"}</label>
      <div className="area-search-explainer"><ShieldCheck size={16} /><p>We search the public web, then keep listings with job-posting data from an employer's own site or a recognized hiring platform. Pages that disallow automated access are skipped. Your resume stays on this device; only your target role, skills, and location are sent to Brave Search.</p></div>
      <div className="company-search-actions">
        <div className={`company-search-status ${searchStatus === "setup" || searchStatus === "error" ? "needs-setup" : ""}`} role="status">
          {searchStatus === "checking" && <><LoaderCircle className="spin" size={15} /> Checking job-search setup…</>}
          {searchStatus === "ready" && <><CheckCircle2 size={15} /> Ready to search. {resumeName ? `Matching with ${resumeName}.` : "Add a resume in My profile for stronger matching."}</>}
          {searchStatus === "setup" && <><CircleHelp size={15} /> Add a Brave Search API key once in Settings to enable public web search.</>}
          {searchStatus === "error" && <><CircleHelp size={15} /> Could not check search setup: {statusError}</>}
        </div>
        {searchStatus === "setup" && <button type="button" className="button button-secondary" onClick={onOpenSettings}>Configure web search</button>}
        {searchStatus === "error" && <button type="button" className="button button-secondary" onClick={() => void checkSearchStatus()}>Retry</button>}
        {searchStatus === "ready" && <button className="button button-primary" disabled={discoveryRunning || !area.trim() || !country.trim()}>{discoveryRunning ? <><LoaderCircle className="spin" size={15} /> Searching public career pages…</> : <><Search size={15} /> Find matching jobs</>}</button>}
      </div>
    </form>
  </section>;
}

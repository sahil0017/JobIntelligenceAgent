import { useEffect, useState } from "react";
import {
  Check,
  CheckCircle2,
  CircleHelp,
  Globe2,
  Linkedin,
  LoaderCircle,
  Mail,
  Search,
  ShieldCheck,
  X,
} from "lucide-react";
import { Profile } from "../profile/Profile";
import { apiRequest } from "../../lib/api";
import { GmailMessage, GmailThread, IntegrationSettings, Job, OutreachDraft, PortalConfig, SettingsPayload, UserProfile } from "../../types";

export function Settings({ onNotify, profile }: { onNotify: (message: string) => void; profile: UserProfile }) {
  const [gmail, setGmail] = useState({ clientId: "", clientSecret: "", redirectUrl: "http://localhost:8787/api/gmail/oauth/callback" });
  const [linkedin, setLinkedin] = useState({ clientId: "", clientSecret: "", redirectUrl: "http://localhost:8787/api/linkedin/oauth/callback" });
  const [gmailStatus, setGmailStatus] = useState<IntegrationSettings | null>(null);
  const [linkedinStatus, setLinkedinStatus] = useState<IntegrationSettings | null>(null);
  const [webSearchApiKey, setWebSearchApiKey] = useState("");
  const [webSearchConfigured, setWebSearchConfigured] = useState(false);
  const [portals, setPortals] = useState<PortalConfig[]>([]);
  const [portalName, setPortalName] = useState("");
  const [portalProvider, setPortalProvider] = useState("naukri");
  const [portalUrl, setPortalUrl] = useState("");
  const [portalNotes, setPortalNotes] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [gmailQuery, setGmailQuery] = useState("newer_than:90d (job OR hiring OR recruiter OR career OR application)");
  const [gmailThreads, setGmailThreads] = useState<GmailThread[]>([]);
  const [gmailSearchBusy, setGmailSearchBusy] = useState(false);
  const [selectedLead, setSelectedLead] = useState<GmailMessage | null>(null);
  const [jobTitle, setJobTitle] = useState("");
  const [jobCompany, setJobCompany] = useState("");
  const [jobURL, setJobURL] = useState("");
  const [managerEmail, setManagerEmail] = useState("");
  const [hrEmail, setHREmail] = useState("");
  const [outreachDrafts, setOutreachDrafts] = useState<OutreachDraft[]>([]);
  const [sendConfirmation, setSendConfirmation] = useState<OutreachDraft | null>(null);

  const refreshSettings = async () => {
    const result = await apiRequest<SettingsPayload>("/api/settings");
    setGmail((current) => ({ ...current, clientId: result.integrations.gmail.clientId, redirectUrl: result.integrations.gmail.redirectUrl || current.redirectUrl }));
    setLinkedin((current) => ({ ...current, clientId: result.integrations.linkedin.clientId, redirectUrl: result.integrations.linkedin.redirectUrl || current.redirectUrl }));
    setGmailStatus(result.integrations.gmail);
    setLinkedinStatus(result.integrations.linkedin);
    setPortals(result.portals);
    setWebSearchConfigured(result.webSearch.configured);
  };

  useEffect(() => {
    refreshSettings().catch((loadError: unknown) => setError(loadError instanceof Error ? loadError.message : "Could not load settings."))
      .finally(() => setLoading(false));
  }, []);

  const saveIntegration = async (provider: "gmail" | "linkedin") => {
    setBusy(`save-${provider}`);
    setError("");
    try {
      const values = provider === "gmail" ? gmail : linkedin;
      await apiRequest(`/api/settings/integrations/${provider}`, {
        method: "PUT",
        body: JSON.stringify(values),
      });
      if (provider === "gmail") setGmail((current) => ({ ...current, clientSecret: "" }));
      else setLinkedin((current) => ({ ...current, clientSecret: "" }));
      await refreshSettings();
      onNotify(`${provider === "gmail" ? "Gmail" : "LinkedIn"} OAuth settings saved securely.`);
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Could not save integration settings.");
    } finally {
      setBusy("");
    }
  };

  const saveWebSearchKey = async () => {
    setBusy("save-web-search");
    setError("");
    try {
      await apiRequest("/api/settings/web-search", {
        method: "PUT",
        body: JSON.stringify({ apiKey: webSearchApiKey }),
      });
      setWebSearchApiKey("");
      await refreshSettings();
      onNotify("Brave Search API key saved encrypted in the local database.");
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Could not save the Brave Search API key.");
    } finally {
      setBusy("");
    }
  };

  const removeWebSearchKey = async () => {
    setBusy("remove-web-search");
    setError("");
    try {
      await apiRequest("/api/settings/web-search", { method: "DELETE" });
      await refreshSettings();
      onNotify("Brave Search API key removed.");
    } catch (removeError) {
      setError(removeError instanceof Error ? removeError.message : "Could not remove the Brave Search API key.");
    } finally {
      setBusy("");
    }
  };

  const connectIntegration = async (provider: "gmail" | "linkedin") => {
    setBusy(`connect-${provider}`);
    setError("");
    try {
      const result = await apiRequest<{ authorizationUrl: string }>(`/api/${provider}/oauth/start`);
      window.location.assign(result.authorizationUrl);
    } catch (connectError) {
      setError(connectError instanceof Error ? connectError.message : "Could not start OAuth.");
      setBusy("");
    }
  };

  const disconnectIntegration = async (provider: "gmail" | "linkedin") => {
    setBusy(`disconnect-${provider}`);
    setError("");
    try {
      await apiRequest(provider === "gmail" ? "/api/gmail/connection" : "/api/linkedin/connection", { method: "DELETE" });
      await refreshSettings();
      onNotify(`${provider === "gmail" ? "Gmail" : "LinkedIn"} disconnected.`);
    } catch (disconnectError) {
      setError(disconnectError instanceof Error ? disconnectError.message : "Could not disconnect integration.");
    } finally {
      setBusy("");
    }
  };

  const addPortal = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy("portal");
    setError("");
    try {
      const result = await apiRequest<{ portal: PortalConfig }>("/api/portals", {
        method: "POST",
        body: JSON.stringify({ name: portalName, provider: portalProvider, url: portalUrl, notes: portalNotes }),
      });
      setPortals((current) => [...current, result.portal]);
      setPortalName("");
      setPortalUrl("");
      setPortalNotes("");
      onNotify(`${result.portal.name} portal settings saved.`);
    } catch (portalError) {
      setError(portalError instanceof Error ? portalError.message : "Could not save portal settings.");
    } finally {
      setBusy("");
    }
  };

  const removePortal = async (portal: PortalConfig) => {
    try {
      const result = await apiRequest<{ portals: PortalConfig[] }>(`/api/portals/${encodeURIComponent(portal.id)}`, { method: "DELETE" });
      setPortals(result.portals);
      onNotify(`${portal.name} configuration removed.`);
    } catch (removeError) {
      setError(removeError instanceof Error ? removeError.message : "Could not remove portal.");
    }
  };

  const searchGmail = async () => {
    setGmailSearchBusy(true);
    setError("");
    try {
      const result = await apiRequest<{ threads: GmailThread[] }>(
        `/api/gmail/threads?q=${encodeURIComponent(gmailQuery)}&limit=10`,
      );
      setGmailThreads(result.threads);
    } catch (searchError) {
      setError(searchError instanceof Error ? searchError.message : "Could not search Gmail.");
    } finally {
      setGmailSearchBusy(false);
    }
  };

  const selectLead = (message: GmailMessage) => {
    setSelectedLead(message);
    setJobTitle(message.subject);
    setJobCompany("");
    setJobURL("");
    setOutreachDrafts([]);
    setError("");
  };

  const createOutreachDrafts = () => {
    const company = jobCompany.trim();
    const role = jobTitle.trim();
    const recipients: OutreachDraft[] = [];
    const roleAndCompany = [role || "the role", company].filter(Boolean).join(" at ");
    const profileDetails = [
      profile.title ? `I currently work as a ${profile.title}` : "",
      profile.experience ? `with ${profile.experience} of experience` : "",
      profile.skills ? `and have experience with ${profile.skills}` : "",
    ].filter(Boolean).join(", ");
    const intro = profileDetails ? `${profileDetails}.` : "I would be glad to share more about my background.";
    const listing = jobURL.trim() ? `\n\nJob listing: ${jobURL.trim()}` : "";
    const signature = [profile.name, profile.email].filter(Boolean).join("\n");
    const subject = `Interest in ${roleAndCompany}`;

    if (managerEmail.trim()) {
      recipients.push({
        recipientType: "Hiring manager",
        to: managerEmail.trim(),
        subject,
        body: `Hello Hiring Manager,\n\nI came across the ${roleAndCompany} opportunity and would like to express my interest. ${intro} I would welcome the chance to discuss how I could contribute to the team.${listing}\n\nBest,\n${signature}`,
        sent: false,
      });
    }
    if (hrEmail.trim()) {
      recipients.push({
        recipientType: "HR",
        to: hrEmail.trim(),
        subject,
        body: `Hello,\n\nI am reaching out to express my interest in ${roleAndCompany}. ${intro} Could you please share the appropriate next steps or connect me with the hiring team?${listing}\n\nThank you,\n${signature}`,
        sent: false,
      });
    }
    if (!recipients.length) {
      setError("Enter an email address for the hiring manager, HR, or both.");
      return;
    }
    setOutreachDrafts(recipients);
    setError("");
  };

  const updateOutreachDraft = (recipientType: OutreachDraft["recipientType"], field: "to" | "subject" | "body", value: string) => {
    setOutreachDrafts((current) => current.map((draft) =>
      draft.recipientType === recipientType ? { ...draft, [field]: value } : draft,
    ));
  };

  const sendOutreachDraft = async () => {
    if (!sendConfirmation) return;
    setBusy("send-email");
    setError("");
    try {
      await apiRequest<{ messageId: string; status: string }>("/api/gmail/send", {
        method: "POST",
        body: JSON.stringify({
          to: sendConfirmation.to,
          subject: sendConfirmation.subject,
          body: sendConfirmation.body,
          confirm: true,
        }),
      });
      setOutreachDrafts((current) => current.map((draft) =>
        draft.recipientType === sendConfirmation.recipientType ? { ...draft, sent: true } : draft,
      ));
      onNotify(`Email sent to ${sendConfirmation.to}.`);
      setSendConfirmation(null);
    } catch (sendError) {
      setError(sendError instanceof Error
        ? `${sendError.message} If the result is uncertain, check Gmail Sent before retrying.`
        : "Could not send the email. Check Gmail Sent before retrying.");
    } finally {
      setBusy("");
    }
  };

  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> YOUR CONNECTIONS</div><h1>Settings</h1><p className="page-subtitle">Configure account connections and portals used to prepare applications.</p></div><span className="settings-storage-badge"><CheckCircle2 size={14} /> Saved to local SQLite</span></div>
    {error && <div className="source-error"><CircleHelp size={15} /><span>{error}</span></div>}
    {loading ? <div className="panel settings-loading"><LoaderCircle className="spin" size={18} /> Loading saved settings…</div> : <div className="settings-grid">
      <section className="panel settings-card">
        <div className="settings-card-heading"><div className="settings-brand-icon gmail"><Mail size={18} /></div><div><h2>Gmail</h2><p>Search recruiter messages and send reviewed emails through the official Gmail API.</p></div><span className={`integration-state ${gmailStatus?.connected ? "connected" : ""}`}>{gmailStatus?.connected ? "Connected" : gmailStatus?.configured ? "Configured" : "Not configured"}</span></div>
        <div className="settings-form">
          <label className="form-field"><span>Google OAuth client ID</span><input value={gmail.clientId} onChange={(event) => setGmail((current) => ({ ...current, clientId: event.target.value }))} autoComplete="off" /></label>
          <label className="form-field"><span>Google OAuth client secret</span><input type="password" value={gmail.clientSecret} placeholder={gmailStatus?.secretConfigured ? "Saved encrypted; leave blank to keep it" : ""} onChange={(event) => setGmail((current) => ({ ...current, clientSecret: event.target.value }))} autoComplete="new-password" /></label>
          <label className="form-field"><span>Authorized redirect URI</span><input value={gmail.redirectUrl} onChange={(event) => setGmail((current) => ({ ...current, redirectUrl: event.target.value }))} /></label>
        </div>
        <div className="settings-scope"><ShieldCheck size={14} /> Gmail requests read-only access plus permission to send messages. Search job-related mail and review every draft before confirming an individual send. Reauthorize Gmail to grant the new send permission.</div>
        <div className="settings-actions"><button className="button button-secondary" onClick={() => void saveIntegration("gmail")} disabled={busy !== ""}>{busy === "save-gmail" ? "Saving…" : "Save configuration"}</button><button className="button button-primary" onClick={() => void connectIntegration("gmail")} disabled={!gmailStatus?.configured || busy !== ""}>{busy === "connect-gmail" ? "Connecting…" : gmailStatus?.connected ? "Reauthorize Gmail" : "Connect Gmail"}</button>{gmailStatus?.connected && <button className="text-button" onClick={() => void disconnectIntegration("gmail")} disabled={busy !== ""}>Disconnect</button>}</div>
        {gmailStatus?.connected && <div className="gmail-leads">
          <div className="gmail-leads-heading"><div><h3>Job leads from Gmail</h3><p>Search emails for job listings and recruiter messages.</p></div></div>
          <div className="gmail-search-row"><input aria-label="Gmail search query" value={gmailQuery} onChange={(event) => setGmailQuery(event.target.value)} /><button className="button button-secondary" onClick={() => void searchGmail()} disabled={gmailSearchBusy || !gmailQuery.trim()}>{gmailSearchBusy ? "Searching…" : "Search Gmail"}</button></div>
          {gmailThreads.length > 0 && <div className="gmail-lead-list">{gmailThreads.flatMap((thread) => thread.messages).map((message) => <article className="gmail-lead-item" key={message.id}>
            <div><strong>{message.subject || "(No subject)"}</strong><span>{message.from}{message.date ? ` · ${message.date}` : ""}</span><p>{message.body || message.snippet}</p></div>
            <button className="button button-small" onClick={() => selectLead(message)}>Use as job lead</button>
          </article>)}</div>}
          {selectedLead && <div className="gmail-draft-form">
            <div className="source-form-title"><strong>Prepare outreach drafts</strong><span>Details are editable. Enter the recipients; CareerPilot does not guess contact addresses.</span></div>
            <div className="form-grid">
              <label className="form-field"><span>Job title</span><input value={jobTitle} onChange={(event) => setJobTitle(event.target.value)} /></label>
              <label className="form-field"><span>Company</span><input value={jobCompany} onChange={(event) => setJobCompany(event.target.value)} /></label>
              <label className="form-field form-full"><span>Job listing URL (optional)</span><input type="url" value={jobURL} onChange={(event) => setJobURL(event.target.value)} placeholder="https://…" /></label>
              <label className="form-field"><span>Hiring manager email (optional)</span><input type="email" value={managerEmail} onChange={(event) => setManagerEmail(event.target.value)} placeholder="manager@company.com" /></label>
              <label className="form-field"><span>HR email (optional)</span><input type="email" value={hrEmail} onChange={(event) => setHREmail(event.target.value)} placeholder="hr@company.com" /></label>
            </div>
            <button className="button button-primary" onClick={createOutreachDrafts}>Create editable drafts</button>
            {outreachDrafts.map((draft) => <div className="gmail-outreach-draft" key={draft.recipientType}>
              <div className="gmail-draft-title"><strong>{draft.recipientType} draft</strong>{draft.sent && <span>Sent</span>}</div>
              <label className="form-field"><span>To</span><input type="email" value={draft.to} onChange={(event) => updateOutreachDraft(draft.recipientType, "to", event.target.value)} disabled={draft.sent} /></label>
              <label className="form-field"><span>Subject</span><input value={draft.subject} onChange={(event) => updateOutreachDraft(draft.recipientType, "subject", event.target.value)} disabled={draft.sent} /></label>
              <label className="form-field"><span>Message</span><textarea rows={7} value={draft.body} onChange={(event) => updateOutreachDraft(draft.recipientType, "body", event.target.value)} disabled={draft.sent} /></label>
              {!draft.sent &&               <button className="button button-secondary" onClick={() => { setError(""); setSendConfirmation(draft); }}>Review &amp; send…</button>}
            </div>)}
          </div>}
        </div>}
      </section>

      <section className="panel settings-card">
        <div className="settings-card-heading"><div className="settings-brand-icon linkedin"><Linkedin size={18} /></div><div><h2>LinkedIn</h2><p>Connect your professional identity using LinkedIn OIDC.</p></div><span className={`integration-state ${linkedinStatus?.connected ? "connected" : ""}`}>{linkedinStatus?.connected ? "Connected" : linkedinStatus?.configured ? "Configured" : "Not configured"}</span></div>
        <div className="settings-form">
          <label className="form-field"><span>LinkedIn client ID</span><input value={linkedin.clientId} onChange={(event) => setLinkedin((current) => ({ ...current, clientId: event.target.value }))} autoComplete="off" /></label>
          <label className="form-field"><span>LinkedIn client secret</span><input type="password" value={linkedin.clientSecret} placeholder={linkedinStatus?.secretConfigured ? "Saved encrypted; leave blank to keep it" : ""} onChange={(event) => setLinkedin((current) => ({ ...current, clientSecret: event.target.value }))} autoComplete="new-password" /></label>
          <label className="form-field"><span>Authorized redirect URI</span><input value={linkedin.redirectUrl} onChange={(event) => setLinkedin((current) => ({ ...current, redirectUrl: event.target.value }))} /></label>
        </div>
        <div className="settings-scope"><ShieldCheck size={14} /> Requested scopes: openid, profile, email. No LinkedIn job-search API access is assumed.</div>
        {linkedinStatus?.profile?.name && <div className="linked-profile"><CheckCircle2 size={15} /> Linked to {linkedinStatus.profile.name}{linkedinStatus.profile.email ? ` · ${linkedinStatus.profile.email}` : ""}</div>}
        <div className="settings-actions"><button className="button button-secondary" onClick={() => void saveIntegration("linkedin")} disabled={busy !== ""}>{busy === "save-linkedin" ? "Saving…" : "Save configuration"}</button><button className="button button-primary" onClick={() => void connectIntegration("linkedin")} disabled={!linkedinStatus?.configured || busy !== ""}>{busy === "connect-linkedin" ? "Connecting…" : "Connect LinkedIn"}</button>{linkedinStatus?.connected && <button className="text-button" onClick={() => void disconnectIntegration("linkedin")} disabled={busy !== ""}>Disconnect</button>}</div>
      </section>

      <section className="panel settings-card portal-settings-card">
        <div className="settings-card-heading"><div className="settings-brand-icon portal"><Globe2 size={18} /></div><div><h2>Other job portals</h2><p>Save portal details for your application workflow.</p></div></div>
        <div className="portal-integration-note"><CircleHelp size={15} /> Greenhouse and Lever boards are connected under Job matches → Sources. Naukri and custom portal records are saved but require an approved adapter before the agent can search or apply.</div>
        <form className="portal-settings-form" onSubmit={(event) => void addPortal(event)}>
          <label className="form-field"><span>Portal</span><select value={portalProvider} onChange={(event) => setPortalProvider(event.target.value)}><option value="naukri">Naukri</option><option value="other">Other portal</option></select></label>
          <label className="form-field"><span>Display name</span><input required maxLength={100} value={portalName} onChange={(event) => setPortalName(event.target.value)} placeholder={portalProvider === "naukri" ? "Naukri" : "Portal name"} /></label>
          <label className="form-field form-full"><span>Official portal URL</span><input required type="url" pattern="https://.*" value={portalUrl} onChange={(event) => setPortalUrl(event.target.value)} placeholder="https://www.example.com/" /></label>
          <label className="form-field form-full"><span>Notes for your workflow</span><textarea rows={2} maxLength={1000} value={portalNotes} onChange={(event) => setPortalNotes(event.target.value)} placeholder="Optional notes; do not enter passwords or access tokens." /></label>
          <button className="button button-primary" disabled={busy !== ""}>{busy === "portal" ? "Saving…" : "Save portal"}</button>
        </form>
        {portals.length > 0 && <div className="portal-config-list">{portals.map((portal) => <div className="portal-config-item" key={portal.id}><div><strong>{portal.name}</strong><span>{portal.url}</span><small>{portal.status === "adapter_unavailable" ? "Configured · adapter needed" : portal.status}</small></div><button className="text-button" onClick={() => void removePortal(portal)}>Remove</button></div>)}</div>}
      </section>

      <section className="panel settings-card web-search-settings-card">
        <div className="settings-card-heading"><div className="settings-brand-icon portal"><Globe2 size={18} /></div><div><h2>Public job search</h2><p>Brave Search finds public employer career pages; job listings are verified before matching.</p></div><span className={`integration-state ${webSearchConfigured ? "connected" : ""}`}>{webSearchConfigured ? "Ready" : "Setup needed"}</span></div>
        <label className="form-field"><span>Brave Search API key</span><input type="password" value={webSearchApiKey} onChange={(event) => setWebSearchApiKey(event.target.value)} placeholder={webSearchConfigured ? "Saved encrypted; enter a new key to replace" : "Paste your API key"} autoComplete="new-password" /></label>
        <div className="settings-scope"><ShieldCheck size={14} /> Saved encrypted on this device and never returned to the browser. CareerPilot sends job title, skills, and location to Brave Search, then fetches only public HTTPS pages that allow crawling. Your resume stays local.</div>
        <div className="settings-actions"><button className="button button-secondary" onClick={() => void saveWebSearchKey()} disabled={busy !== "" || !webSearchApiKey.trim()}>{busy === "save-web-search" ? "Saving…" : "Save API key"}</button>{webSearchConfigured && <button className="text-button" onClick={() => void removeWebSearchKey()} disabled={busy !== ""}>{busy === "remove-web-search" ? "Removing…" : "Remove key"}</button>}<a className="text-button" href="https://brave.com/search/api/" target="_blank" rel="noreferrer">Get a Brave Search API key</a></div>
      </section>

      <section className="settings-safety-card">
        <ShieldCheck size={17} /><div><strong>Application safety is on</strong><p>Profile, portal settings, integration keys, OAuth client secrets, and parsed resume text are stored in the local database. Secrets and resume text are encrypted. The agent can prepare applications, but automated submission is not enabled; every future submission must require your confirmation.</p></div>
      </section>
    </div>}
    {sendConfirmation && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && busy !== "send-email") setSendConfirmation(null); }}>
      <section className="modal-card gmail-confirm-modal" role="dialog" aria-modal="true" aria-labelledby="gmail-confirm-title">
        <div className="modal-heading"><div><div className="section-kicker">FINAL REVIEW</div><h2 id="gmail-confirm-title">Send this email?</h2></div><button className="icon-button" aria-label="Cancel send" onClick={() => setSendConfirmation(null)} disabled={busy === "send-email"}><X size={16} /></button></div>
        <p className="modal-description">This will send one email from your connected Gmail account. Verify the recipient and full message; sending cannot be undone.</p>
        {error && <div className="source-error gmail-send-error"><CircleHelp size={15} /><span>{error}</span></div>}
        <div className="gmail-confirm-content"><strong>To</strong><span>{sendConfirmation.to}</span><strong>Subject</strong><span>{sendConfirmation.subject}</span><strong>Message</strong><pre>{sendConfirmation.body}</pre></div>
        <div className="modal-actions"><button className="button button-secondary" onClick={() => setSendConfirmation(null)} disabled={busy === "send-email"}>Cancel</button><button className="button button-primary" onClick={() => void sendOutreachDraft()} disabled={busy === "send-email"}>{busy === "send-email" ? "Sending…" : "Confirm & send"}</button></div>
      </section>
    </div>}
  </>;
}

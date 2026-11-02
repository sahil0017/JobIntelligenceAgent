import { BrainCircuit, LogOut, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { apiRequest } from "../../lib/api";
import { PortalSession } from "../ai/types";
import { useAIStatus } from "../ai/useRun";

const row = (label: string, value: string, ok?: boolean) => <div className="admin-row" key={label}><span>{label}</span><strong className={ok === false ? "admin-warn" : ""}>{value}</strong></div>;

export function AIAdmin({ onNotify }: { onNotify: (message: string) => void }) {
  const status = useAIStatus();
  const [session, setSession] = useState<PortalSession | null>(null);
  useEffect(() => {
    apiRequest<PortalSession>("/api/ai/portals/session").then(setSession).catch(() => undefined);
  }, []);

  const [signInUrl, setSignInUrl] = useState("https://accounts.google.com/");
  const signIn = async () => {
    try {
      const next = await apiRequest<PortalSession>("/api/ai/portals/session", { method: "POST", body: JSON.stringify({ url: signInUrl }) });
      setSession(next);
      onNotify(next.message || "Sign-in window opened.");
    } catch (error) {
      onNotify(error instanceof Error ? error.message : "Could not open the sign-in window.");
    }
  };

  const signOut = async () => {
    try {
      const next = await apiRequest<PortalSession>("/api/ai/portals/session", { method: "DELETE" });
      setSession(next);
      onNotify(next.message || "Signed out.");
    } catch (error) {
      onNotify(error instanceof Error ? error.message : "Could not sign out.");
    }
  };

  const days = status?.keyExpiresInDays;
  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> ADMIN</div><h1>AI &amp; sign-ins</h1><p className="page-subtitle">How the agents are configured on this machine. Change these in cortex/.env, then restart with npm run dev:ai.</p></div></div>
    <div className="panel ai-panel-pad">
      <div className="workflow-run-head"><strong><BrainCircuit size={15} /> AI service</strong></div>
      {!status && <p className="ai-note">Checking…</p>}
      {status && !status.available && <p className="workflow-error">{status.message ?? "The AI service is offline. Start it with npm run dev:ai."}</p>}
      {status?.available && <>
        {row("Embedding engine", `${status.engine}${status.deepLearning ? " (deep learning)" : " (lightweight)"}`)}
        {row("Language model", `${status.llm}${status.llmRemote ? " (hosted)" : " (local)"}`, status.llm !== "none")}
        {row("Page reader", status.scraper ?? "unknown")}
        {row("Tracing", status.tracing ? "Phoenix on" : "Off")}
        {days != null && row("Hosted-model key", days < 0 ? `Expired ${-days} days ago` : `Expires in ${days} days`, days > 14)}
      </>}
    </div>
    <div className="panel ai-panel-pad">
      <div className="workflow-run-head"><strong><ShieldCheck size={15} /> Career-portal sign-in</strong></div>
      <p className="ai-note">When a company portal answers 401/403, you sign in with Google in a browser window. Only the browser session is stored, locally; your Google password is never seen by CareerPilot.</p>
      {row("Saved browser session", session?.signedIn ? "Present" : "None yet. It is created the first time you sign in")}
      <label className="form-field"><span>Sign in ahead of time (address of the portal or Google)</span><input type="url" value={signInUrl} onChange={(event) => setSignInUrl(event.target.value)} /></label>
      <div className="ai-actions"><button className="button button-primary" onClick={() => void signIn()}>Open sign-in window</button><button className="button button-secondary" disabled={!session?.signedIn} onClick={() => void signOut()}><LogOut size={14} /> Sign out and delete session</button></div>
    </div>
  </>;
}

import {
  CheckCircle2,
  Clock3,
  Compass,
  LoaderCircle,
  Mail,
  Pause,
  Play,
  Sparkles,
} from "lucide-react";
import { ActivityRow } from "../../components/ActivityRow";
import { AgentJob, AgentWorkflow } from "./AgentWorkflow";
import { CareerSource, DiscoveryEvent } from "../../types";

export function AgentActivity({ agentRunning, setAgentRunning, onRunDiscovery, discoveryRunning, history, sources, defaultCountry, defaultState, defaultArea, onAddJobs }: { agentRunning: boolean; setAgentRunning: (value: boolean) => void; onRunDiscovery: () => void; discoveryRunning: boolean; history: DiscoveryEvent[]; sources: CareerSource[]; defaultCountry: string; defaultState: string; defaultArea: string; onAddJobs: (jobs: AgentJob[]) => void }) {
  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> YOUR JOB DISCOVERY</div><h1>Agent activity</h1><p className="page-subtitle">Discovery runs only when you start it. No background searches happen.</p></div><button className={`button ${agentRunning ? "button-secondary" : "button-primary"}`} onClick={() => setAgentRunning(!agentRunning)}>{agentRunning ? <><Pause size={15} /> Pause agent</> : <><Play size={15} /> Resume agent</>}</button></div>
    <div className={`agent-control-card ${agentRunning ? "" : "is-paused"}`}><div className="agent-orb"><Sparkles size={24} /></div><div className="agent-control-copy"><div className="agent-state-label"><span className={`agent-status-dot ${agentRunning ? "" : "paused"}`} />{agentRunning ? "READY WHEN YOU ARE" : "DISCOVERY PAUSED"}</div><h2>{agentRunning ? "Your career sources are ready." : "Discovery is paused."}</h2><p>{sources.length ? `A manual scan checks ${sources.length} public career board${sources.length === 1 ? "" : "s"} and ranks current openings against your profile.` : "Connect a public Greenhouse or Lever board before starting a scan."}</p><div className="agent-task-chips"><span><CheckCircle2 size={13} /> Public career feeds</span><span><CheckCircle2 size={13} /> Local keyword matching</span><span><Mail size={13} /> Gmail job leads</span></div></div><button className="button button-light" disabled={!agentRunning || discoveryRunning || sources.length === 0} onClick={onRunDiscovery}>{discoveryRunning ? <LoaderCircle className="spin" size={15} /> : <Sparkles size={15} />}{discoveryRunning ? "Scanning..." : "Run now"}</button></div>
    <AgentWorkflow defaultCountry={defaultCountry} defaultState={defaultState} defaultArea={defaultArea} onAddJobs={onAddJobs} />
    <div className="section-heading-row activity-heading"><div><div className="section-kicker">SAVED LOCALLY</div><h2>Discovery history</h2></div><span className="history-count">{history.length} run{history.length === 1 ? "" : "s"}</span></div>
    <div className="panel activity-log">{history.length ? history.map((event) => <ActivityRow key={event.id} icon={<Compass size={16} />} tone="blue" time={event.time} title={event.title} detail={event.detail} badge={event.badge} />) : <div className="history-empty"><Clock3 size={18} /><span>No discovery runs recorded yet. Start a scan to see results here.</span></div>}</div>
    <div className="safety-note"><div><CheckCircle2 size={18} /></div><p><strong>You’re always in control.</strong><br />This integration only reads the public job listings on the boards you configure. It does not submit applications or send messages.</p></div>
  </>;
}

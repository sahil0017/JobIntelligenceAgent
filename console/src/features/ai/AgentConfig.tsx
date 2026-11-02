import { Check, Settings2, TriangleAlert } from "lucide-react";
import { Page } from "../../types";
import { useAIStatus } from "./useRun";

type Props = { hasProfile: boolean; resumeName: string; onNavigate: (page: Page) => void };

export function AgentConfig({ hasProfile, resumeName, onNavigate }: Props) {
  const status = useAIStatus();
  const items = [
    { label: "Profile", ok: hasProfile, note: hasProfile ? "ready" : "add skills", page: "My profile" as Page },
    { label: "Résumé", ok: Boolean(resumeName), note: resumeName || "not uploaded", page: "My profile" as Page },
    { label: "AI service", ok: Boolean(status?.available), note: status ? (status.available ? status.engine ?? "online" : "offline") : "checking…", page: "AI & sign-ins" as Page },
    { label: "Language model", ok: Boolean(status?.available && status.llm && status.llm !== "none"), note: status?.llm ?? "—", page: "AI & sign-ins" as Page },
  ];
  return <div className="agent-config" aria-label="Agent configuration">
    {items.map((item) => <button key={item.label} className={`agent-config-item ${item.ok ? "ok" : "warn"}`} onClick={() => onNavigate(item.page)} title="Change in Admin">
      {item.ok ? <Check size={13} /> : <TriangleAlert size={13} />}<span>{item.label}</span><em>{item.note}</em>
    </button>)}
    <button className="text-button agent-config-admin" onClick={() => onNavigate("My profile")}><Settings2 size={13} /> Configure in Admin</button>
  </div>;
}

import { useState } from "react";
import {
  Building2,
  CheckCircle2,
  CircleHelp,
  LoaderCircle,
  Plus,
  X,
} from "lucide-react";
import { CareerSource, Job } from "../../types";

export function SourcesModal({ sources, onClose, onAdd, onRemove }: {
  sources: CareerSource[];
  onClose: () => void;
  onAdd: (source: Omit<CareerSource, "id">) => Promise<void>;
  onRemove: (source: CareerSource) => Promise<void>;
}) {
  const [provider, setProvider] = useState<CareerSource["provider"]>("greenhouse");
  const [company, setCompany] = useState("");
  const [boardId, setBoardId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await onAdd({ provider, company, boardId });
      setCompany("");
      setBoardId("");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Could not add that source.");
    } finally {
      setBusy(false);
    }
  };

  const remove = async (source: CareerSource) => {
    setBusy(true);
    setError("");
    try {
      await onRemove(source);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Could not remove that source.");
    } finally {
      setBusy(false);
    }
  };

  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><section className="modal-card sources-modal" aria-labelledby="sources-title">
    <div className="modal-heading"><div><div className="section-kicker">PUBLIC CAREER BOARDS</div><h2 id="sources-title">Career sources</h2></div><button type="button" className="icon-button" aria-label="Close dialog" onClick={onClose}><X size={18} /></button></div>
    <p className="modal-description">Connect official Greenhouse or Lever boards. CareerPilot reads their public listings only; it does not sign in or bypass access controls.</p>
    <div className="source-list">{sources.map((source) => <div className="source-list-item" key={source.id}><div className="source-list-icon"><Building2 size={16} /></div><div className="source-list-copy"><strong>{source.company}</strong><span>{source.provider === "greenhouse" ? "Greenhouse" : "Lever"} · board ID: {source.boardId}</span></div><button className="source-remove" disabled={busy} onClick={() => remove(source)}>Remove</button></div>)}{sources.length === 0 && <div className="source-list-empty">No career boards connected yet.</div>}</div>
    <form className="source-form" onSubmit={submit}>
      <div className="source-form-title"><strong>Add a career board</strong><span>Find the board ID in the company’s official job listings URL.</span></div>
      <div className="form-grid">
        <label className="form-field"><span>Provider</span><select value={provider} onChange={(event) => setProvider(event.target.value as CareerSource["provider"])}><option value="greenhouse">Greenhouse</option><option value="lever">Lever</option></select></label>
        <label className="form-field"><span>Company name</span><input required maxLength={100} value={company} onChange={(event) => setCompany(event.target.value)} placeholder="e.g. Example Inc." /></label>
        <label className="form-field form-full"><span>Board ID</span><input required pattern="[A-Za-z0-9._-]+" maxLength={100} value={boardId} onChange={(event) => setBoardId(event.target.value)} placeholder={provider === "greenhouse" ? "e.g. stripe" : "e.g. example-company"} /><small className="source-help">{provider === "greenhouse" ? "For boards.greenhouse.io/stripe, enter stripe." : "For jobs.lever.co/example-company, enter example-company."}</small></label>
      </div>
      {error && <div className="source-form-error"><CircleHelp size={14} />{error}</div>}
      <button className="button button-primary source-add-button" disabled={busy || !company.trim() || !boardId.trim()}>{busy ? <LoaderCircle className="spin" size={15} /> : <Plus size={15} />}{busy ? "Saving..." : "Connect source"}</button>
    </form>
    <div className="source-footnote"><CheckCircle2 size={14} /> Sources and job data are stored on this device. Job listings refresh only when you run discovery.</div>
  </section></div>;
}

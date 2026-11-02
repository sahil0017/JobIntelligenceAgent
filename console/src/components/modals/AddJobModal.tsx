import { useState } from "react";
import {
  Plus,
  X,
} from "lucide-react";
import { Job } from "../../types";

export function AddJobModal({ onClose, onAdd }: { onClose: () => void; onAdd: (job: Job) => void }) {
  const [role, setRole] = useState("");
  const [company, setCompany] = useState("");
  const [url, setUrl] = useState("");
  const [location, setLocation] = useState("Bengaluru, India");
  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (!role.trim() || !company.trim()) return;
    onAdd({
      id: `job-${Date.now()}`, company: company.trim(), role: role.trim(), location, workMode: "Not specified", salary: "Not listed", posted: "Added just now", source: url.trim() ? "Added by you" : "Manual entry", ...(url.trim() ? { url: url.trim() } : {}),
      score: 0, tags: ["Needs review"], status: "Review", logo: company.trim().charAt(0).toUpperCase(), logoTone: "blue",
    });
  };
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><form className="modal-card" onSubmit={submit}><div className="modal-heading"><div><div className="section-kicker">SAVE AN OPPORTUNITY</div><h2>Add a job</h2></div><button type="button" className="icon-button" aria-label="Close dialog" onClick={onClose}><X size={18} /></button></div><p className="modal-description">Bring a role into your workspace and keep everything in one place.</p><label className="form-field"><span>Job title <b>*</b></span><input autoFocus required value={role} onChange={(event) => setRole(event.target.value)} placeholder="e.g. Backend Engineer" /></label><label className="form-field"><span>Company <b>*</b></span><input required value={company} onChange={(event) => setCompany(event.target.value)} placeholder="e.g. Acme Inc." /></label><label className="form-field"><span>Location</span><input value={location} onChange={(event) => setLocation(event.target.value)} placeholder="City, country or remote" /></label><label className="form-field"><span>Job posting URL <small>Optional</small></span><input type="url" value={url} onChange={(event) => setUrl(event.target.value)} placeholder="https://" /></label><div className="modal-actions"><button type="button" className="button button-secondary" onClick={onClose}>Cancel</button><button type="submit" className="button button-primary"><Plus size={15} /> Add to matches</button></div></form></div>;
}

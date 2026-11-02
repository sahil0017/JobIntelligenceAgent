import {
  ArrowRight,
  Check,
  CheckCircle2,
  Clock3,
  FileText,
  Target,
  Upload,
} from "lucide-react";
import { UserProfile } from "../../types";

export function Profile({ profile, setProfile, profileSaved, profileLoaded, resumeName, onUploadResume, onRemoveResume, onSave }: {
  profile: UserProfile;
  setProfile: (value: (current: typeof profile) => typeof profile) => void;
  profileSaved: boolean;
  profileLoaded: boolean;
  resumeName: string;
  onUploadResume: (file: File) => void;
  onRemoveResume: () => void;
  onSave: () => void;
}) {
  const profileFields = [profile.title, profile.location, profile.skills, profile.roles];
  const profileCompletion = Math.round(profileFields.filter((field) => field.trim().length > 0).length / profileFields.length * 100);
  return <>
    <div className="welcome-row page-title-row"><div><div className="eyebrow"><span className="eyebrow-line" /> YOUR FOUNDATION</div><h1>Career profile</h1><p className="page-subtitle">A little context helps your agent find a much better fit.</p></div><button className="button button-primary" onClick={onSave} disabled={!profileLoaded}>{profileSaved ? <Check size={16} /> : <CheckCircle2 size={16} />}{profileSaved ? "Saved" : profileLoaded ? "Save changes" : "Loading…"}</button></div>
    <div className="profile-layout"><section className="panel profile-form"><div className="profile-section-title"><div className="section-icon"><FileText size={17} /></div><div><h2>About you</h2><p>The basics recruiters and your agent need to know.</p></div></div>
      <div className="form-grid"><label className="form-field"><span>Full name</span><input value={profile.name} onChange={(event) => setProfile((current) => ({ ...current, name: event.target.value }))} /></label><label className="form-field"><span>Current role</span><input value={profile.title} onChange={(event) => setProfile((current) => ({ ...current, title: event.target.value }))} /></label><label className="form-field"><span>Email address</span><input type="email" value={profile.email} onChange={(event) => setProfile((current) => ({ ...current, email: event.target.value }))} /></label><label className="form-field"><span>Preferred location</span><input value={profile.location} onChange={(event) => setProfile((current) => ({ ...current, location: event.target.value }))} /></label><label className="form-field"><span>Years of experience</span><input value={profile.experience} onChange={(event) => setProfile((current) => ({ ...current, experience: event.target.value }))} /></label><label className="form-field"><span>Roles you’re interested in</span><input value={profile.roles} onChange={(event) => setProfile((current) => ({ ...current, roles: event.target.value }))} /></label><label className="form-field form-full"><span>Your skills <small>Separate with commas</small></span><textarea rows={3} value={profile.skills} onChange={(event) => setProfile((current) => ({ ...current, skills: event.target.value }))} /></label></div>
      <div className="profile-divider" /><div className="profile-section-title resume-title"><div className="section-icon mint"><Upload size={17} /></div><div><h2>Your resume</h2><p>Text-based PDF and DOCX resumes are parsed and encrypted in your local database.</p></div></div><label className="resume-upload"><input type="file" accept=".pdf,.docx" onChange={(event) => { const file = event.target.files?.[0]; if (file) onUploadResume(file); event.currentTarget.value = ""; }} /><div className="upload-icon"><Upload size={19} /></div><div><strong>{resumeName || "Upload your resume"}</strong><span>{resumeName ? "Parsed and saved locally" : "PDF or DOCX, up to 10 MB"}</span></div><span className="button button-small">{resumeName ? "Replace" : "Choose file"}</span>{resumeName && <button type="button" className="resume-remove" onClick={(event) => { event.preventDefault(); onRemoveResume(); }}>Remove</button>}</label>
    </section>
      <aside className="profile-side"><div className="panel completeness-card"><div className="section-kicker">PROFILE DETAILS</div><div className="completeness-main"><div className="completion-ring"><span>{profileCompletion}<small>%</small></span></div><div><strong>{profileCompletion === 100 ? "Ready to match" : "Add your preferences"}</strong><p>Scores use your current role, target roles, location, and skills.</p></div></div><div className="completion-items">{[["Current role", profile.title], ["Preferred location", profile.location], ["Skills", profile.skills], ["Target roles", profile.roles]].map(([label, value]) => <div className={value.trim() ? "" : "incomplete-item"} key={label}>{value.trim() ? <CheckCircle2 size={15} /> : <Clock3 size={15} />} {label}<span>{value.trim() ? "Done" : "Add"}</span></div>)}</div></div>
      <div className="privacy-card"><div className="privacy-icon"><CheckCircle2 size={16} /></div><strong>Your data stays yours.</strong><p>Your profile is stored locally on this device. We never apply to a job or send a message without your say-so.</p><button>Learn about privacy <ArrowRight size={13} /></button></div></aside>
    </div>
  </>;
}

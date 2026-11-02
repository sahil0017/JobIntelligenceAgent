export function StatCard({ icon, label, value, note, tone }: { icon: React.ReactNode; label: string; value: string; note: React.ReactNode; tone: string }) {
  return <div className="stat-card"><div className={`stat-icon ${tone}`}>{icon}</div><div className="stat-label">{label}</div><div className="stat-value">{value}</div><div className="stat-note">{note}</div><div className={`stat-accent ${tone}`} /></div>;
}

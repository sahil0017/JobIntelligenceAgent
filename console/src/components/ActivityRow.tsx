export function ActivityRow({ icon, tone, time, title, detail, badge }: { icon: React.ReactNode; tone: string; time: string; title: string; detail: string; badge: string }) {
  return <div className="activity-row"><div className={`activity-row-icon ${tone}`}>{icon}</div><div className="activity-row-copy"><strong>{title}</strong><span>{detail}</span></div><span className="activity-badge">{badge}</span><time>{time}</time></div>;
}

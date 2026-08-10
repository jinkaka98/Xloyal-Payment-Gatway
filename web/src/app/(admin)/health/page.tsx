import { Activity, CheckCircle2, RefreshCw, Server } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/format";

export default async function HealthPage() {
  const providers = await api.getProviderHealth();
  const operational = providers.filter((provider) => provider.status === "operational").length;
  return <div className="page"><PageHeader eyebrow="Infrastructure" title="System health" description="Provider connectivity and core service checks." actions={<button className="button"><RefreshCw size={17} />Run checks</button>} />
    <div className="health-banner"><CheckCircle2 size={24} /><div><strong>Core payment services are operational</strong><p>{operational} of {providers.length} checks passing. One provider reports elevated latency.</p></div><span>Last updated just now</span></div>
    <section className="health-grid">{providers.map((provider) => <article className="health-item" key={provider.id}><div className="health-item-top"><div className="health-icon">{provider.id === "database" || provider.id === "worker" ? <Server size={20} /> : <Activity size={20} />}</div><StatusBadge status={provider.status} /></div><h2>{provider.name}</h2><p>{provider.message}</p><dl><div><dt>Latency</dt><dd>{provider.latencyMs} ms</dd></div><div><dt>30-day uptime</dt><dd>{provider.uptime}%</dd></div><div><dt>Last check</dt><dd>{formatRelativeTime(provider.lastCheckedAt)}</dd></div></dl></article>)}</section>
  </div>;
}

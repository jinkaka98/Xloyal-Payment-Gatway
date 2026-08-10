import { Download } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";

export default async function AuditLogsPage() {
  const events = await api.getAuditEvents();
  return <div className="page"><PageHeader eyebrow="Security and compliance" title="Audit logs" description="Immutable record of administrative and system activity." actions={<button className="button"><Download size={17} />Export log</button>} />
    <section className="section-block"><div className="section-heading"><div><h2>Recent events</h2><p>Newest events appear first.</p></div></div><div className="table-scroll"><table><thead><tr><th>Timestamp</th><th>Actor</th><th>Action</th><th>Resource</th><th>Tenant</th><th>Source</th></tr></thead><tbody>{events.map((event) => <tr key={event.id}><td>{formatDate(event.createdAt)}</td><td><strong>{event.actor}</strong></td><td><code className="event-code">{event.action}</code></td><td>{event.resourceType}<span className="cell-subtitle">{event.resourceId}</span></td><td>{event.tenantName}</td><td>{event.ipAddress}</td></tr>)}</tbody></table></div></section>
  </div>;
}

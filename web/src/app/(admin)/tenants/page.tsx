import { Plus } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatCurrency, formatDate } from "@/lib/format";

export default async function TenantsPage() {
  const items = await api.getTenants();
  return <div className="page"><PageHeader eyebrow="Configuration" title="Tenants" description="Organizations authorized to create and manage QRIS invoices." actions={<button className="button button-primary"><Plus size={17} />Add tenant</button>} />
    <section className="section-block"><div className="section-heading"><div><h2>Tenant directory</h2><p>{items.length} tenant records</p></div></div><div className="table-scroll"><table><thead><tr><th>Tenant</th><th>Status</th><th>Invoices</th><th>Processed volume</th><th>Created</th></tr></thead><tbody>{items.map((tenant) => <tr key={tenant.id}><td><strong>{tenant.name}</strong><span className="cell-subtitle">{tenant.slug} · {tenant.id}</span></td><td><StatusBadge status={tenant.active ? "active" : "inactive"} /></td><td>{tenant.invoiceCount.toLocaleString("id-ID")}</td><td className="amount-cell">{formatCurrency(tenant.volume)}</td><td>{formatDate(tenant.createdAt, false)}</td></tr>)}</tbody></table></div></section>
  </div>;
}

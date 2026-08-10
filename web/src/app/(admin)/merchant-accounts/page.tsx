import { Plus, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/format";

export default async function MerchantAccountsPage() {
  const items = await api.getMerchantAccounts();
  return <div className="page"><PageHeader eyebrow="Provider setup" title="Merchant accounts" description="Provider credentials and connectivity assigned to each tenant." actions={<button className="button button-primary"><Plus size={17} />Connect account</button>} />
    <section className="section-block"><div className="section-heading"><div><h2>Connected accounts</h2><p>Credential values stay encrypted and are never displayed.</p></div><button className="button"><RefreshCw size={16} />Check all</button></div><div className="table-scroll"><table><thead><tr><th>Account</th><th>Tenant</th><th>Provider</th><th>Connectivity</th><th>Success rate</th><th>Enabled</th></tr></thead><tbody>{items.map((account) => <tr key={account.id}><td><strong>{account.name}</strong><span className="cell-subtitle">{account.id}</span></td><td>{account.tenantName}</td><td>{account.provider}</td><td><StatusBadge status={account.providerStatus} /><span className="cell-subtitle">Checked {formatRelativeTime(account.lastCheckedAt)}</span></td><td className="amount-cell">{account.successRate}%</td><td><StatusBadge status={account.active ? "active" : "inactive"} /></td></tr>)}</tbody></table></div></section>
  </div>;
}

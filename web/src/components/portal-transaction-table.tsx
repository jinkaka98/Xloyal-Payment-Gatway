import { formatCurrency, formatDate } from "@/lib/format";
import type { PortalTransaction } from "@/lib/types";

export function PortalTransactionTable({ items }: { items: PortalTransaction[] }) {
  return <div className="table-scroll"><table><thead><tr><th>Reference</th><th>Merchant ID</th><th>Tenant</th><th>Paid at</th><th className="align-right">Amount</th><th>Source</th><th>Match</th></tr></thead><tbody>{items.map((item) => <tr key={item.id}><td><code>{item.reference}</code></td><td>{item.merchant_id}</td><td>{item.tenant_id || "Unassigned"}</td><td>{formatDate(item.paid_at)}</td><td className="align-right amount-cell">{formatCurrency(item.amount)}</td><td>{item.source}</td><td>{item.match_confidence}</td></tr>)}</tbody></table></div>;
}

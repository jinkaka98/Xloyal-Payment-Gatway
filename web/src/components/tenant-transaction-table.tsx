"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { Search } from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import { StatusBadge } from "@/components/status-badge";
import { formatCurrency, formatDate } from "@/lib/format";
import type { Invoice, InvoiceStatus, Tenant } from "@/lib/types";

export function TenantTransactionTable({ invoices, tenants, tenantNames }: { invoices: Invoice[]; tenants: Tenant[]; tenantNames: Record<string, string> }) {
  const [tenantID, setTenantID] = useState("all");
  const [status, setStatus] = useState<"all" | InvoiceStatus>("all");
  const [query, setQuery] = useState("");
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return invoices.filter((invoice) => (tenantID === "all" || invoice.tenantId === tenantID) && (status === "all" || invoice.status === status) && (!needle || `${invoice.id} ${invoice.idempotencyKey} ${invoice.description} ${invoice.tenantId}`.toLowerCase().includes(needle)));
  }, [invoices, query, status, tenantID]);

  return <section className="section-block">
    <div className="section-heading"><div><span className="section-kicker">Tenant request ledger</span><h2>Request transaksi per Tenant ID</h2><p>{visible.length} dari {invoices.length} request invoice.</p></div></div>
    <div className="tenant-transaction-toolbar">
      <label className="search-field"><Search size={17} /><span className="sr-only">Cari transaksi</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Cari invoice atau order ID" /></label>
      <label><span>Tenant</span><select value={tenantID} onChange={(event) => setTenantID(event.target.value)}><option value="all">Semua Tenant ID</option>{tenants.map((tenant) => <option key={tenant.id} value={tenant.id}>{tenant.name} ({tenant.id})</option>)}</select></label>
      <label><span>Status</span><select value={status} onChange={(event) => setStatus(event.target.value as "all" | InvoiceStatus)}><option value="all">Semua status</option><option value="creating">Creating</option><option value="pending">Pending</option><option value="paid">Paid</option><option value="expired">Expired</option><option value="failed">Failed</option></select></label>
    </div>
    {visible.length === 0 ? <EmptyState title="Belum ada request transaksi" description="Request akan muncul setelah tenant membuat invoice melalui API." /> : <div className="table-scroll"><table><thead><tr><th>Request invoice</th><th>Tenant ID</th><th>Waktu request</th><th className="align-right">Nominal</th><th>Status</th><th>Cross-check saldo</th><th>Provider reference</th></tr></thead><tbody>
      {visible.map((invoice) => <tr key={invoice.id}><td><Link className="table-link" href={`/invoices/${invoice.id}`}>{invoice.id}</Link><span className="cell-subtitle">Order: {invoice.idempotencyKey || "Tidak tersedia"}</span></td><td><strong>{tenantNames[invoice.tenantId] || invoice.tenantId}</strong><span className="cell-subtitle"><code>{invoice.tenantId}</code></span></td><td>{formatDate(invoice.createdAt)}<span className="cell-subtitle">Tenant API</span></td><td className="align-right amount-cell">{formatCurrency(invoice.amount)}</td><td><StatusBadge status={invoice.status} /></td><td><strong>{invoice.checkCount > 0 ? `${invoice.checkCount} kali check` : "Belum dicek"}</strong><span className="cell-subtitle">{invoice.lastCheckedAt ? formatDate(invoice.lastCheckedAt) : invoice.status === "paid" ? "Cocok dari browser" : "Menunggu request check"}</span></td><td>{invoice.providerReference ? <code>{invoice.providerReference}</code> : "Belum tersedia"}</td></tr>)}
    </tbody></table></div>}
  </section>;
}

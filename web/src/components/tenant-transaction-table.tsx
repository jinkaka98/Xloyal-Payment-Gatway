"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { Search } from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import { StatusBadge } from "@/components/status-badge";
import { formatCurrency, formatDate } from "@/lib/format";
import type { InvoiceStatus, Tenant, TenantTransaction } from "@/lib/types";

type TransactionMode = "all" | TenantTransaction["mode"];

export function TenantTransactionTable({ transactions, tenants, tenantNames }: { transactions: TenantTransaction[]; tenants: Tenant[]; tenantNames: Record<string, string> }) {
  const [tenantID, setTenantID] = useState("all");
  const [status, setStatus] = useState<"all" | InvoiceStatus>("all");
  const [mode, setMode] = useState<TransactionMode>("all");
  const [query, setQuery] = useState("");
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return transactions.filter((transaction) =>
      (tenantID === "all" || transaction.tenantId === tenantID) &&
      (status === "all" || transaction.status === status) &&
      (mode === "all" || transaction.mode === mode) &&
      (!needle || `${transaction.id} ${transaction.idempotencyKey} ${transaction.tenantId} ${transaction.providerReference} ${transaction.matchedTransactionId}`.toLowerCase().includes(needle))
    );
  }, [mode, query, status, tenantID, transactions]);

  return <section className="section-block">
    <div className="section-heading"><div><span className="section-kicker">Tenant transaction ledger</span><h2>Transaksi per Tenant ID</h2><p>{visible.length} dari {transactions.length} transaksi production dan Sandbox.</p></div></div>
    <div className="tenant-transaction-toolbar">
      <label className="search-field"><Search size={17} /><span className="sr-only">Cari transaksi</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Cari transaksi atau order ID" /></label>
      <label><span>Tenant</span><select value={tenantID} onChange={(event) => setTenantID(event.target.value)}><option value="all">Semua Tenant ID</option>{tenants.map((tenant) => <option key={tenant.id} value={tenant.id}>{tenant.name} ({tenant.id})</option>)}</select></label>
      <label><span>Mode transaksi</span><select value={mode} onChange={(event) => setMode(event.target.value as TransactionMode)}><option value="all">Semua mode</option><option value="production">Production</option><option value="sandbox">Sandbox</option></select></label>
      <label><span>Status</span><select value={status} onChange={(event) => setStatus(event.target.value as "all" | InvoiceStatus)}><option value="all">Semua status</option><option value="creating">Creating</option><option value="pending">Pending</option><option value="paid">Paid</option><option value="expired">Expired</option><option value="failed">Failed</option></select></label>
    </div>
    {visible.length === 0 ? <EmptyState title="Belum ada transaksi tenant" description="Transaksi akan muncul setelah tenant membuat invoice atau QRIS dinamis melalui API." /> : <div className="table-scroll"><table><thead><tr><th>Request transaksi</th><th>Tenant ID</th><th>Mode</th><th>Waktu request</th><th className="align-right">Nominal</th><th>Status</th><th>Cross-check</th><th>Referensi</th></tr></thead><tbody>
      {visible.map((transaction) => <tr key={`${transaction.kind}-${transaction.id}`}><td>{transaction.kind === "invoice" ? <Link className="table-link" href={`/invoices/${transaction.id}`}>{transaction.id}</Link> : <code>{transaction.id}</code>}<span className="cell-subtitle">{transaction.kind === "qris" ? "QRIS dinamis" : "Invoice provider"} / Order: {transaction.idempotencyKey || "Tidak tersedia"}</span></td><td><strong>{tenantNames[transaction.tenantId] || transaction.tenantId}</strong><span className="cell-subtitle"><code>{transaction.tenantId}</code></span></td><td><span className={`qris-mode${transaction.mode === "sandbox" ? " enabled" : ""}`}>{transaction.mode === "sandbox" ? "Sandbox" : "Production"}</span></td><td>{formatDate(transaction.createdAt)}<span className="cell-subtitle">{transaction.requestSource === "tenant_api" ? "Tenant QRIS API" : "Tenant Invoice API"}</span></td><td className="align-right amount-cell">{formatCurrency(transaction.payableAmount)}{transaction.uniqueAmountCode > 0 && <span className="cell-subtitle">Tagihan {formatCurrency(transaction.amount)} + kode {transaction.uniqueAmountCode}</span>}</td><td><StatusBadge status={transaction.status} /></td><td><strong>{transaction.checkCount > 0 ? `${transaction.checkCount} kali check` : "Belum dicek"}</strong><span className="cell-subtitle">{transaction.lastCheckedAt ? formatDate(transaction.lastCheckedAt) : transaction.validation || "Menunggu validasi"}</span></td><td>{transaction.matchedTransactionId ? <code>{transaction.matchedTransactionId}</code> : transaction.providerReference ? <code>{transaction.providerReference}</code> : "Belum tersedia"}</td></tr>)}
    </tbody></table></div>}
  </section>;
}

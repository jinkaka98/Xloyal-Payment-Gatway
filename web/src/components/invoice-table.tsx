"use client";

import { useDeferredValue, useState } from "react";
import Link from "next/link";
import { Search } from "lucide-react";
import { filterInvoices, type InvoiceFilter } from "@/lib/invoices";
import { formatCurrency, formatDate } from "@/lib/format";
import type { Invoice } from "@/lib/types";
import { EmptyState } from "./empty-state";
import { StatusBadge } from "./status-badge";

const filters: { value: InvoiceFilter; label: string }[] = [
  { value: "all", label: "All" }, { value: "creating", label: "Creating" }, { value: "pending", label: "Pending" }, { value: "paid", label: "Paid" }, { value: "failed", label: "Failed" }, { value: "expired", label: "Expired" },
];

export function InvoiceTable({ invoices, compact = false }: { invoices: Invoice[]; compact?: boolean }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<InvoiceFilter>("all");
  const deferredQuery = useDeferredValue(query);
  const visible = compact ? invoices.slice(0, 5) : filterInvoices(invoices, deferredQuery, status);
  return <div className="table-module">
    {!compact && <div className="table-toolbar">
      <label className="search-field"><Search size={17} aria-hidden="true" /><span className="sr-only">Search invoices</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search invoice, customer, or tenant" /></label>
      <div className="segmented-control" aria-label="Filter invoice status">{filters.map((item) => <button key={item.value} className={status === item.value ? "selected" : ""} onClick={() => setStatus(item.value)}>{item.label}</button>)}</div>
    </div>}
    {visible.length === 0 ? <EmptyState title="No invoices recorded" description="Invoices will appear after a tenant creates a QRIS payment request." /> : <div className="table-scroll"><table>
      <thead><tr><th>Invoice</th><th>Customer</th><th>Tenant</th><th>Created</th><th className="align-right">Amount</th><th>Status</th></tr></thead>
      <tbody>{visible.map((invoice) => <tr key={invoice.id}>
        <td><Link className="table-link" href={`/invoices/${invoice.id}`}>{invoice.id}</Link><span className="cell-subtitle">{invoice.provider}</span></td>
        <td>{invoice.customerName}<span className="cell-subtitle">{invoice.description}</span></td>
        <td>{invoice.tenantName}</td><td>{formatDate(invoice.createdAt)}</td>
        <td className="align-right amount-cell">{formatCurrency(invoice.amount)}</td><td><StatusBadge status={invoice.status} /></td>
      </tr>)}</tbody>
    </table></div>}
  </div>;
}

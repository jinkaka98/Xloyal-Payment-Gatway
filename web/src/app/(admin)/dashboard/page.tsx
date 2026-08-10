import Link from "next/link";
import { ArrowRight, CheckCircle2, CircleDollarSign, Clock3, Gauge } from "lucide-react";
import { InvoiceTable } from "@/components/invoice-table";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatCurrency, formatRelativeTime } from "@/lib/format";

export default async function DashboardPage() {
  const summary = await api.getDashboard();
  const metrics = [
    { label: "Processed volume", value: formatCurrency(summary.totalVolume), detail: "Today", icon: CircleDollarSign },
    { label: "Paid invoices", value: String(summary.paidInvoices), detail: "Today", icon: CheckCircle2 },
    { label: "Pending", value: String(summary.pendingInvoices), detail: "Requires monitoring", icon: Clock3 },
    { label: "Success rate", value: `${summary.successRate}%`, detail: "Last 24 hours", icon: Gauge },
  ];
  return <div className="page">
    <PageHeader eyebrow="Operations overview" title="Dashboard" description="Live payment activity across all tenants and QRIS providers." actions={<Link href="/invoices" className="button button-primary">View invoices <ArrowRight size={17} /></Link>} />
    <section className="metric-grid" aria-label="Payment summary">{metrics.map(({ label, value, detail, icon: Icon }) => <article className="metric" key={label}><div className="metric-heading"><span>{label}</span><Icon size={19} /></div><strong>{value}</strong><small>{detail}</small></article>)}</section>
    <div className="dashboard-grid">
      <section className="section-block dashboard-main"><div className="section-heading"><div><h2>Recent invoices</h2><p>Latest activity across connected merchants.</p></div><Link href="/invoices" className="text-link">View all <ArrowRight size={15} /></Link></div><InvoiceTable invoices={summary.recentInvoices} compact /></section>
      <section className="section-block provider-panel"><div className="section-heading"><div><h2>Provider status</h2><p>Current connectivity checks.</p></div></div><div className="provider-list">{summary.providerHealth.slice(0,3).map((provider) => <div className="provider-row" key={provider.id}><div><strong>{provider.name}</strong><span>{provider.latencyMs} ms · {formatRelativeTime(provider.lastCheckedAt)}</span></div><StatusBadge status={provider.status} /></div>)}</div><Link href="/health" className="button button-wide">Open system health <ArrowRight size={16} /></Link></section>
    </div>
  </div>;
}

import Link from "next/link";
import { ArrowRight, CheckCircle2, CircleDollarSign, Clock3, Gauge, Radio } from "lucide-react";
import { InvoiceTable } from "@/components/invoice-table";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatCurrency, formatRelativeTime } from "@/lib/format";

export default async function DashboardPage() {
  const summary = await api.getDashboard();
  const connected = summary.providerHealth.length > 0;
  const metrics = [
    { label: "Volume processed", value: formatCurrency(summary.totalVolume), detail: "Settled today", icon: CircleDollarSign },
    { label: "Paid invoices", value: String(summary.paidInvoices), detail: "Confirmed payments", icon: CheckCircle2 },
    { label: "Pending checks", value: String(summary.pendingInvoices), detail: "Watching next cycle", icon: Clock3 },
    { label: "Success rate", value: `${summary.successRate}%`, detail: "Last 24 hours", icon: Gauge },
  ];

  return <div className="page control-page">
    <PageHeader eyebrow="InterActive QRIS / live operations" title="Dashboard QRIS" description="Monitor payment flow, merchant connectivity, and exception handling from one control surface." actions={<Link href="/global-transactions" className="button button-primary">Global transaction log <ArrowRight size={17} /></Link>} />
    <section className="control-brief" aria-label="QRIS control status">
      <div className="control-brief-mark"><Radio size={20} /></div>
      <div><span>Machine checker</span><strong>{connected ? "Watching connected merchant sessions" : "No merchant session is connected"}</strong><p>{connected ? "Pending invoices are held for the next provider or browser reconciliation cycle." : "Connect a Merchant ID and import a browser session before reconciliation can begin."}</p></div>
      <div className="control-brief-state"><span>SYNC WINDOW</span><strong>05:00</strong><small>Every five minutes</small></div>
    </section>
    <section className="metric-grid" aria-label="Payment summary">{metrics.map(({ label, value, detail, icon: Icon }, index) => <article className={`metric metric-${index + 1}`} key={label}><div className="metric-heading"><span>{label}</span><Icon size={19} /></div><strong>{value}</strong><small>{detail}</small></article>)}</section>
    <div className="dashboard-grid">
      <section className="section-block dashboard-main"><div className="section-heading"><div><span className="section-kicker">Payment rail</span><h2>Recent invoices</h2><p>Latest activity across connected merchants.</p></div><Link href="/invoices" className="text-link">View all <ArrowRight size={15} /></Link></div><InvoiceTable invoices={summary.recentInvoices} compact /></section>
      <section className="section-block provider-panel"><div className="section-heading"><div><span className="section-kicker">Connection rail</span><h2>Merchant signal</h2><p>Latest provider and checker response.</p></div></div>{connected ? <div className="provider-list">{summary.providerHealth.slice(0, 3).map((provider) => <div className="provider-row" key={provider.id}><div><strong>{provider.name}</strong><span>{provider.latencyMs} ms - {formatRelativeTime(provider.lastCheckedAt)}</span></div><StatusBadge status={provider.status} /></div>)}</div> : <p className="empty-copy">No health signal yet. This panel only displays live API and checker results.</p>}<Link href="/merchant-connecting" className="button button-wide">Manage connections <ArrowRight size={16} /></Link></section>
    </div>
  </div>;
}

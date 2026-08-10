import Link from "next/link";
import { ArrowLeft, Copy, ExternalLink, RefreshCw } from "lucide-react";
import { notFound } from "next/navigation";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { api } from "@/lib/api";
import { formatCurrency, formatDate } from "@/lib/format";

export default async function InvoiceDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const invoice = await api.getInvoice(id);
  if (!invoice) notFound();
  return <div className="page detail-page">
    <Link href="/invoices" className="back-link"><ArrowLeft size={16} />Back to invoices</Link>
    <PageHeader eyebrow="Invoice detail" title={invoice.id} description={`${invoice.tenantName} · ${invoice.description}`} actions={<><button className="button"><RefreshCw size={16} />Check status</button><button className="button button-primary"><ExternalLink size={16} />Open provider</button></>} />
    <div className="detail-grid">
      <section className="section-block detail-summary"><div className="invoice-total"><div><span>Amount due</span><strong>{formatCurrency(invoice.amount)}</strong></div><StatusBadge status={invoice.status} /></div><dl className="detail-list"><div><dt>Customer</dt><dd>{invoice.customerName}</dd></div><div><dt>Merchant account</dt><dd>{invoice.merchantName}</dd></div><div><dt>Provider</dt><dd>{invoice.provider}</dd></div><div><dt>Provider reference</dt><dd><code>{invoice.providerReference}</code></dd></div><div><dt>Created</dt><dd>{formatDate(invoice.createdAt)}</dd></div><div><dt>Expires</dt><dd>{formatDate(invoice.expiresAt)}</dd></div></dl></section>
      <section className="section-block qr-section"><div className="section-heading"><div><h2>QRIS payment code</h2><p>Scan from a compatible banking or wallet app.</p></div></div><div className="qr-placeholder" role="img" aria-label="QRIS code placeholder"><div className="qr-center">QRIS</div></div><button className="button button-wide"><Copy size={16} />Copy QR payload</button><code className="qr-payload">{invoice.qrPayload}</code></section>
      <section className="section-block timeline-section"><div className="section-heading"><div><h2>Payment timeline</h2><p>Provider and system events for this invoice.</p></div></div><ol className="timeline">{invoice.timeline.map((event) => <li key={event.id} className={`timeline-${event.state}`}><span className="timeline-dot" aria-hidden="true" /><div><strong>{event.label}</strong><p>{event.description}</p><time dateTime={event.timestamp}>{formatDate(event.timestamp)}</time></div></li>)}</ol></section>
    </div>
  </div>;
}

import { Download, Plus } from "lucide-react";
import { InvoiceTable } from "@/components/invoice-table";
import { PageHeader } from "@/components/page-header";
import { api } from "@/lib/api";

export default async function InvoicesPage() {
  const items = await api.getInvoices();
  return <div className="page"><PageHeader eyebrow="Payment activity" title="Invoices" description="Search, inspect, and reconcile QRIS payment requests." actions={<><button className="button"><Download size={17} />Export</button><button className="button button-primary"><Plus size={17} />Create invoice</button></>} /><InvoiceTable invoices={items} /></div>;
}

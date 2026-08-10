import type { Invoice, InvoiceStatus } from "./types";

export type InvoiceFilter = InvoiceStatus | "all";

export function filterInvoices(invoices: Invoice[], query: string, status: InvoiceFilter) {
  const normalized = query.trim().toLowerCase();
  return invoices.filter((invoice) => {
    const statusMatches = status === "all" || invoice.status === status;
    const queryMatches = !normalized || [invoice.id, invoice.customerName, invoice.tenantName ?? ""]
      .some((value) => value.toLowerCase().includes(normalized));
    return statusMatches && queryMatches;
  });
}

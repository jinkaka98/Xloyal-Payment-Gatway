import type { InvoiceStatus, ProviderStatus } from "@/lib/types";

type Status = InvoiceStatus | ProviderStatus | "active" | "inactive";
const labels: Record<Status, string> = { creating: "Creating", paid: "Paid", pending: "Pending", failed: "Failed", expired: "Expired", operational: "Operational", degraded: "Degraded", offline: "Offline", active: "Active", inactive: "Inactive" };

export function StatusBadge({ status }: { status: Status }) {
  return <span className={`status-badge status-${status}`}><span aria-hidden="true" />{labels[status]}</span>;
}

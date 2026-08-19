import type { InvoiceStatus, ProviderStatus, QRISTransactionStatus } from "@/lib/types";

type Status = InvoiceStatus | QRISTransactionStatus | ProviderStatus | "active" | "inactive" | "connected" | "disconnected" | "expired" | "reconnect_required";
const labels: Record<Status, string> = { creating: "Creating", paid: "Paid", pending: "Pending", failed: "Failed", cancelled: "Cancelled", expired: "Expired", operational: "Operational", degraded: "Degraded", offline: "Offline", active: "Active", inactive: "Inactive", connected: "Connected", disconnected: "Disconnected", reconnect_required: "Reconnect required" };

export function StatusBadge({ status }: { status: Status }) {
  return <span className={`status-badge status-${status}`}><span aria-hidden="true" />{labels[status]}</span>;
}

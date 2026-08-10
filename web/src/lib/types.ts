export type InvoiceStatus = "creating" | "pending" | "paid" | "expired" | "failed";
export type ProviderStatus = "operational" | "degraded" | "offline";

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  active: boolean;
  invoiceCount: number;
  volume: number;
  createdAt: string;
}

export interface MerchantAccount {
  id: string;
  tenantId: string;
  tenantName: string;
  provider: string;
  name: string;
  active: boolean;
  providerStatus: ProviderStatus;
  lastCheckedAt: string;
  successRate: number;
}

export interface InvoiceEvent {
  id: string;
  label: string;
  description: string;
  timestamp: string;
  state: "complete" | "current" | "error";
}

export interface Invoice {
  id: string;
  tenantId: string;
  tenantName: string;
  merchantAccountId: string;
  merchantName: string;
  provider: string;
  providerReference: string;
  customerName: string;
  description: string;
  amount: number;
  currency: "IDR";
  status: InvoiceStatus;
  qrPayload: string;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
  timeline: InvoiceEvent[];
}

export interface AuditEvent {
  id: string;
  actor: string;
  action: string;
  resourceType: string;
  resourceId: string;
  tenantName: string;
  ipAddress: string;
  createdAt: string;
}

export interface ProviderHealth {
  id: string;
  name: string;
  status: ProviderStatus;
  latencyMs: number;
  uptime: number;
  lastCheckedAt: string;
  message: string;
}

export interface DashboardSummary {
  totalVolume: number;
  paidInvoices: number;
  pendingInvoices: number;
  successRate: number;
  recentInvoices: Invoice[];
  providerHealth: ProviderHealth[];
}

export interface QRISTemplate {
  id: string;
  name: string;
  image_mime: string;
  merchant_name: string;
  merchant_city: string;
  created_at: string;
}

export interface TestPayment {
  id: string;
  qris_template_id: string;
  amount: number;
  dynamic_payload: string;
  status: InvoiceStatus;
  created_at: string;
  expires_at: string;
}

export interface ApiEnvelope<T> {
  data: T;
  requestId?: string;
}

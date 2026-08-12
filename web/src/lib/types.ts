export type InvoiceStatus = "creating" | "pending" | "paid" | "expired" | "failed";
export type ProviderStatus = "operational" | "degraded" | "offline";
export type HealthKind = "frontend" | "admin_proxy" | "backend_api" | "database" | "browser_session" | "provider_api";

export interface Tenant {
  id: string;
  name: string;
  merchantId: string;
  siteUrl: string;
  callbackUrl: string;
  webhookUrl: string;
  active: boolean;
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
  lastCheckedAt: string | undefined;
  checkCount: number;
  idempotencyKey: string;
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
  kind: HealthKind;
  status: ProviderStatus;
  latencyMs: number;
  endpoint?: string | undefined;
  lastCheckedAt: string;
  lastConnectedAt?: string | undefined;
  lastSyncedAt?: string | undefined;
  updatedAt?: string | undefined;
  message: string;
}

export interface SystemHealthSnapshot {
  overallStatus: ProviderStatus;
  checkedAt: string;
  checks: ProviderHealth[];
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
  tenant_id?: string;
  name: string;
  image_mime: string;
  merchant_name: string;
  merchant_city: string;
  access_scope: "all_tenants" | "selected_tenant";
  static_to_dynamic: boolean;
  max_requests_per_minute: number;
  active: boolean;
  created_at: string;
}

export interface TestPayment {
  id: string;
  qris_template_id: string;
  merchant_id?: string;
  tenant_id?: string;
  amount: number;
  dynamic_payload: string;
  status: InvoiceStatus;
  request_source: string;
  match_confidence: string;
  matched_transaction_id?: string;
  created_at: string;
  updated_at: string;
  expires_at: string;
  last_checked_at?: string;
  next_check_at?: string;
  check_count: number;
}

export interface ApiEnvelope<T> {
  data: T;
  requestId?: string;
}
export interface MerchantID { id: string; interactive_merchant_id: string; name: string; active: boolean; created_at: string; }
export interface MerchantConnection { merchant_id: string; status: "disconnected" | "connected" | "expired" | "reconnect_required"; last_synced_at?: string; last_error?: string; updated_at: string; }
export interface PortalTransaction { id: string; merchant_id: string; tenant_id?: string; reference: string; amount: number; status: string; paid_at: string; source: string; match_confidence: string; invoice_id?: string; }
export interface GlobalTransactionLog {
  id: string;
  event_type: "merchant_transaction" | "qris_test_check";
  merchant_id?: string;
  tenant_id?: string;
  reference: string;
  amount: number;
  status: InvoiceStatus;
  event_at: string;
  source: string;
  request_source: string;
  validation: string;
  expires_at?: string;
  last_checked_at?: string;
  next_check_at?: string;
  check_count: number;
  invoice_id?: string;
  test_payment_id?: string;
  matched_transaction_id?: string;
}
export interface Tariff { merchant_id: string; basis_points: number; fixed_fee: number; active: boolean; }

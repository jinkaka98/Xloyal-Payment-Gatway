import type { ApiEnvelope, AuditEvent, DashboardSummary, GlobalTransactionLog, Invoice, MerchantAccount, MerchantConnection, MerchantID, PortalTransaction, ProviderHealth, QRISTemplate, Tariff, Tenant } from "./types";

const API_URL = process.env.INTERNAL_API_URL ?? process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const USE_MOCK = process.env.NEXT_PUBLIC_USE_MOCK_API === "true";

async function request<T>(path: string, fallback: T): Promise<T> {
  if (USE_MOCK) return fallback;
  const token = process.env.ADMIN_API_TOKEN;
  if (!token) throw new Error("ADMIN_API_TOKEN is required when mock mode is disabled");
  const response = await fetch(`${API_URL}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
  });
  if (!response.ok) throw new Error(`API request failed (${response.status})`);
  const payload = await response.json() as ApiEnvelope<T> | T;
  return typeof payload === "object" && payload !== null && "data" in payload ? (payload as ApiEnvelope<T>).data : payload as T;
}

interface ApiInvoice {
  id: string;
  tenant_id: string;
  merchant_account_id: string;
  idempotency_key: string;
  amount: number;
  currency: "IDR";
  description: string;
  provider_reference: string;
  qr_payload: string;
  status: Invoice["status"];
  created_at: string;
  updated_at: string;
  expires_at: string;
  last_checked_at?: string;
  check_count: number;
}

interface ApiTenant {
  id: string;
  merchant_id?: string;
  name: string;
  site_url?: string;
  callback_url?: string;
  webhook_url?: string;
  sandbox_mode: boolean;
  active: boolean;
  api_key_recoverable?: boolean;
  created_at: string;
}

interface ApiMerchant {
  id: string;
  tenant_id: string;
  provider: string;
  name: string;
  active: boolean;
  created_at: string;
}

interface ApiAudit {
  id: string;
  tenant_id?: string;
  actor: string;
  action: string;
  resource_type: string;
  resource_id: string;
  created_at: string;
}

interface ApiHealth {
  id: string;
  name: string;
  kind: ProviderHealth["kind"];
  status: ProviderHealth["status"];
  latency_ms: number;
  endpoint?: string;
  last_checked_at: string;
  last_connected_at?: string;
  last_synced_at?: string;
  updated_at?: string;
  message: string;
}

function invoiceFromApi(item: ApiInvoice): Invoice {
  return {
    id: item.id,
    tenantId: item.tenant_id,
    tenantName: item.tenant_id,
    merchantAccountId: item.merchant_account_id,
    merchantName: item.merchant_account_id,
    provider: "interactive_qris",
    providerReference: item.provider_reference,
    customerName: item.description || "QRIS customer",
    description: item.description,
    amount: item.amount,
    currency: item.currency,
    status: item.status,
    qrPayload: item.qr_payload,
    createdAt: item.created_at,
    updatedAt: item.updated_at,
    expiresAt: item.expires_at,
    lastCheckedAt: item.last_checked_at,
    checkCount: item.check_count ?? 0,
    idempotencyKey: item.idempotency_key,
    timeline: [
      { id: `${item.id}-created`, label: "Invoice created", description: "Payment request accepted by Xloyal.", timestamp: item.created_at, state: "complete" },
      { id: `${item.id}-status`, label: item.status === "paid" ? "Payment confirmed" : `Status: ${item.status}`, description: "Latest status reported by the QRIS provider.", timestamp: item.updated_at, state: item.status === "failed" ? "error" : "current" },
    ],
  };
}

export const api = {
	async getMerchantIDs(): Promise<MerchantID[]> { return request<MerchantID[]>("/admin/merchant-ids", []); },
	async getMerchantConnection(id: string): Promise<MerchantConnection | null> { if (USE_MOCK) return null; try { return await request<MerchantConnection>(`/admin/merchant-ids/${encodeURIComponent(id)}/connection`, {} as MerchantConnection); } catch { return null; } },
	async getMerchantTransactions(merchantID = ""): Promise<PortalTransaction[]> { return request<PortalTransaction[]>(`/admin/merchant-transactions?merchant_id=${encodeURIComponent(merchantID)}`, []); },
	async getGlobalTransactions(limit = 500): Promise<GlobalTransactionLog[]> { return request<GlobalTransactionLog[]>(`/admin/global-transactions?limit=${limit}`, []); },
	async getTariff(id: string): Promise<Tariff> { return request<Tariff>(`/admin/merchant-ids/${encodeURIComponent(id)}/tariff`, { merchant_id: id, basis_points: 0, fixed_fee: 0, active: true }); },
	async getQRSTemplates(): Promise<QRISTemplate[]> { return request<QRISTemplate[]>("/admin/qris-templates", []); },
  async getDashboard(): Promise<DashboardSummary> {
    const fallback = {
      totalVolume: 0,
      paidInvoices: 0,
      pendingInvoices: 0,
      successRate: 0,
      recentInvoices: [],
      providerHealth: [],
    };
    if (USE_MOCK) return fallback;
    const raw = await request<{
      total_volume: number;
      paid_invoices: number;
      pending_invoices: number;
      success_rate: number;
      recent_invoices: ApiInvoice[];
    }>("/admin/dashboard", {
      total_volume: 0, paid_invoices: 0, pending_invoices: 0, success_rate: 0, recent_invoices: [],
    });
    return {
      totalVolume: raw.total_volume,
      paidInvoices: raw.paid_invoices,
      pendingInvoices: raw.pending_invoices,
      successRate: Number(raw.success_rate.toFixed(1)),
      recentInvoices: raw.recent_invoices.map(invoiceFromApi),
      providerHealth: await api.getProviderHealth(),
    };
  },
  async getTenants(): Promise<Tenant[]> {
    if (USE_MOCK) return [];
    const items = await request<ApiTenant[]>("/admin/tenants", []);
    return items.map((item) => ({ id: item.id, name: item.name, merchantId: item.merchant_id ?? "", siteUrl: item.site_url ?? "", callbackUrl: item.callback_url ?? "", webhookUrl: item.webhook_url ?? "", sandboxMode: item.sandbox_mode, active: item.active, apiKeyRecoverable: item.api_key_recoverable, createdAt: item.created_at }));
  },
  async getMerchantAccounts(): Promise<MerchantAccount[]> {
    if (USE_MOCK) return [];
    const [items, health] = await Promise.all([request<ApiMerchant[]>("/admin/merchant-accounts", []), api.getProviderHealth()]);
    const healthByID = new Map(health.map((item) => [item.id, item]));
    return items.map((item) => {
      const state = healthByID.get(item.id);
      return {
        id: item.id, tenantId: item.tenant_id, tenantName: item.tenant_id, provider: item.provider, name: item.name, active: item.active,
        providerStatus: state?.status ?? "offline", lastCheckedAt: state?.lastCheckedAt ?? item.created_at, successRate: 0,
      };
    });
  },
  async getInvoices(): Promise<Invoice[]> {
    if (USE_MOCK) return [];
    return (await request<ApiInvoice[]>("/admin/invoices", [])).map(invoiceFromApi);
  },
  async getInvoice(id: string): Promise<Invoice | null> {
    if (USE_MOCK) return null;
    try {
      return invoiceFromApi(await request<ApiInvoice>(`/admin/invoices/${encodeURIComponent(id)}`, {} as ApiInvoice));
    } catch {
      return null;
    }
  },
  async getAuditEvents(): Promise<AuditEvent[]> {
    if (USE_MOCK) return [];
    const items = await request<ApiAudit[]>("/admin/audit-events", []);
    return items.map((item) => ({
      id: item.id, actor: item.actor, action: item.action, resourceType: item.resource_type, resourceId: item.resource_id,
      tenantName: item.tenant_id || "system", ipAddress: "internal", createdAt: item.created_at,
    }));
  },
  async getProviderHealth(): Promise<ProviderHealth[]> {
    if (USE_MOCK) return [];
    const items = await request<ApiHealth[]>("/admin/health", []);
    return items.map((item) => ({
      id: item.id, name: item.name, kind: item.kind, status: item.status, latencyMs: item.latency_ms,
      endpoint: item.endpoint, lastCheckedAt: item.last_checked_at, lastConnectedAt: item.last_connected_at,
      lastSyncedAt: item.last_synced_at, updatedAt: item.updated_at, message: item.message,
    }));
  },
};

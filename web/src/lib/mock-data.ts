import type { AuditEvent, Invoice, MerchantAccount, ProviderHealth, Tenant } from "./types";

const timeline = (id: string, status: Invoice["status"]): Invoice["timeline"] => [
  { id: `${id}-created`, label: "Invoice created", description: "Payment request accepted", timestamp: "2026-08-10T10:42:00Z", state: "complete" },
  { id: `${id}-qr`, label: "QRIS issued", description: "QR payload received from provider", timestamp: "2026-08-10T10:42:02Z", state: "complete" },
  ...(status === "paid" ? [{ id: `${id}-paid`, label: "Payment confirmed", description: "Provider confirmed settlement", timestamp: "2026-08-10T10:44:18Z", state: "complete" as const }] : []),
  ...(status === "failed" ? [{ id: `${id}-failed`, label: "Payment failed", description: "Provider rejected the transaction", timestamp: "2026-08-10T10:44:18Z", state: "error" as const }] : []),
  ...(status === "pending" ? [{ id: `${id}-pending`, label: "Awaiting payment", description: "Next provider check scheduled", timestamp: "2026-08-10T10:47:00Z", state: "current" as const }] : []),
];

export const tenants: Tenant[] = [
  { id: "tnt_aurora", name: "Aurora Retail", slug: "aurora-retail", active: true, invoiceCount: 1248, volume: 428500000, createdAt: "2026-05-04T08:00:00Z" },
  { id: "tnt_nusantara", name: "Nusantara Foods", slug: "nusantara-foods", active: true, invoiceCount: 816, volume: 284200000, createdAt: "2026-06-12T08:00:00Z" },
  { id: "tnt_studio", name: "Studio Sembilan", slug: "studio-sembilan", active: false, invoiceCount: 94, volume: 34700000, createdAt: "2026-07-22T08:00:00Z" },
];

export const merchantAccounts: MerchantAccount[] = [
  { id: "mrc_001", tenantId: "tnt_aurora", tenantName: "Aurora Retail", provider: "Xendit", name: "Aurora Main", active: true, providerStatus: "operational", lastCheckedAt: "2026-08-10T11:58:00Z", successRate: 99.8 },
  { id: "mrc_002", tenantId: "tnt_nusantara", tenantName: "Nusantara Foods", provider: "Midtrans", name: "Nusantara Online", active: true, providerStatus: "degraded", lastCheckedAt: "2026-08-10T11:57:00Z", successRate: 97.1 },
  { id: "mrc_003", tenantId: "tnt_studio", tenantName: "Studio Sembilan", provider: "Xendit", name: "Studio Archive", active: false, providerStatus: "operational", lastCheckedAt: "2026-08-10T11:55:00Z", successRate: 98.4 },
];

export const invoices: Invoice[] = [
  { id: "INV-20260810-1042", tenantId: "tnt_aurora", tenantName: "Aurora Retail", merchantAccountId: "mrc_001", merchantName: "Aurora Main", provider: "Xendit", providerReference: "qr_2HT81K", customerName: "Rina Kartika", description: "Order AR-88124", amount: 185000, currency: "IDR", status: "paid", qrPayload: "00020101021226670016COM.XLOYAL.QRIS01189360091500012345670215INV202608101042", createdAt: "2026-08-10T10:42:00Z", updatedAt: "2026-08-10T10:44:18Z", expiresAt: "2026-08-10T11:12:00Z", timeline: timeline("1042", "paid") },
  { id: "INV-20260810-1038", tenantId: "tnt_nusantara", tenantName: "Nusantara Foods", merchantAccountId: "mrc_002", merchantName: "Nusantara Online", provider: "Midtrans", providerReference: "qr_9VX20M", customerName: "Budi Santoso", description: "Catering invoice NF-2408", amount: 625000, currency: "IDR", status: "pending", qrPayload: "00020101021226670016COM.XLOYAL.QRIS01189360091500098765470215INV202608101038", createdAt: "2026-08-10T10:38:00Z", updatedAt: "2026-08-10T10:47:00Z", expiresAt: "2026-08-10T11:08:00Z", timeline: timeline("1038", "pending") },
  { id: "INV-20260810-1015", tenantId: "tnt_aurora", tenantName: "Aurora Retail", merchantAccountId: "mrc_001", merchantName: "Aurora Main", provider: "Xendit", providerReference: "qr_6PA81D", customerName: "Siti Ananda", description: "Order AR-88111", amount: 92000, currency: "IDR", status: "failed", qrPayload: "00020101021226670016COM.XLOYAL.QRIS01189360091500045678970215INV202608101015", createdAt: "2026-08-10T10:15:00Z", updatedAt: "2026-08-10T10:17:18Z", expiresAt: "2026-08-10T10:45:00Z", timeline: timeline("1015", "failed") },
  { id: "INV-20260810-0954", tenantId: "tnt_aurora", tenantName: "Aurora Retail", merchantAccountId: "mrc_001", merchantName: "Aurora Main", provider: "Xendit", providerReference: "qr_1DF30P", customerName: "Dimas Pratama", description: "Order AR-88097", amount: 345000, currency: "IDR", status: "expired", qrPayload: "00020101021226670016COM.XLOYAL.QRIS01189360091500022233370215INV202608100954", createdAt: "2026-08-10T09:54:00Z", updatedAt: "2026-08-10T10:24:00Z", expiresAt: "2026-08-10T10:24:00Z", timeline: timeline("0954", "pending") },
];

export const auditEvents: AuditEvent[] = [
  { id: "evt_0081", actor: "admin@xloyal.id", action: "invoice.status_changed", resourceType: "invoice", resourceId: "INV-20260810-1042", tenantName: "Aurora Retail", ipAddress: "103.18.32.14", createdAt: "2026-08-10T10:44:18Z" },
  { id: "evt_0080", actor: "system:poller", action: "provider.check", resourceType: "invoice", resourceId: "INV-20260810-1038", tenantName: "Nusantara Foods", ipAddress: "internal", createdAt: "2026-08-10T10:43:00Z" },
  { id: "evt_0079", actor: "ops@xloyal.id", action: "merchant.updated", resourceType: "merchant_account", resourceId: "mrc_002", tenantName: "Nusantara Foods", ipAddress: "36.85.12.21", createdAt: "2026-08-10T09:51:00Z" },
  { id: "evt_0078", actor: "admin@xloyal.id", action: "tenant.disabled", resourceType: "tenant", resourceId: "tnt_studio", tenantName: "Studio Sembilan", ipAddress: "103.18.32.14", createdAt: "2026-08-10T08:12:00Z" },
];

export const providerHealth: ProviderHealth[] = [
  { id: "xendit", name: "Xendit QRIS", status: "operational", latencyMs: 184, uptime: 99.98, lastCheckedAt: "2026-08-10T11:58:00Z", message: "All requests processing normally" },
  { id: "midtrans", name: "Midtrans QRIS", status: "degraded", latencyMs: 842, uptime: 99.42, lastCheckedAt: "2026-08-10T11:57:00Z", message: "Elevated response latency" },
  { id: "database", name: "PostgreSQL", status: "operational", latencyMs: 12, uptime: 100, lastCheckedAt: "2026-08-10T11:59:00Z", message: "Primary database healthy" },
  { id: "worker", name: "Payment worker", status: "operational", latencyMs: 31, uptime: 99.99, lastCheckedAt: "2026-08-10T11:59:00Z", message: "Queue depth within target" },
];

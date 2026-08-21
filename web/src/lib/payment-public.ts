export type PaymentStatus = "payment_pending" | "paid" | "cancelled" | "expired" | "failed" | "redirecting" | "closed";

export interface PaymentThemeConfig {
  template_key?: string;
  schema_version?: number;
  branding?: { display_name?: string; logo_url?: string; tagline?: string };
  tenant_branding?: { display_name?: string; logo_url?: string; tagline?: string; primary_color?: string; favicon_url?: string };
  colors?: Record<string, string>;
  layout?: { max_width?: number; radius?: number; density?: "comfortable" | "compact" };
  payment_visibility?: { show_qr?: boolean; show_amount?: boolean; show_description?: boolean; show_reference?: boolean };
  timer?: { enabled?: boolean; warning_seconds?: number };
  success_copy?: { title?: string; message?: string };
  redirect_delay?: number;
}

export interface PublicPaymentSession {
  session_id: string;
  invoice_id: string;
  status: PaymentStatus;
  payment_status: "creating" | "pending" | "paid" | "expired" | "failed";
  checkout_url?: string;
  amount: number;
  requested_amount?: number;
  unique_amount_code?: number;
  qris_merchant_name?: string;
  qris_merchant_city?: string;
  currency: string;
  description?: string;
  qr_payload?: string;
  expires_at: string;
  server_now: string;
  theme?: { id: string; version: number; config: PaymentThemeConfig };
  redirect?: { success_url?: string; cancel_url?: string; failed_url?: string; expired_url?: string };
}

export interface PaymentEvent {
  event_id: string;
  payment_session_id: string;
  invoice_id: string;
  status: "payment_pending" | "verifying" | "paid" | "failed" | "expired" | "cancelled" | "redirecting" | "closed";
  sequence: number;
  occurred_at: string;
}

const API_URL = (process.env.NEXT_PUBLIC_API_URL ?? "https://api.alpakyros.net").replace(/\/$/, "");

function endpoint(path: string) { return `${API_URL}${path}`; }

async function parseResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = new Error(`payment request failed: ${response.status}`) as Error & { status?: number; body?: unknown };
    error.status = response.status;
    try { error.body = await response.json(); } catch { /* no public error body */ }
    throw error;
  }
  return response.json() as Promise<T>;
}

export async function getPaymentSession(token: string): Promise<PublicPaymentSession> {
  const response = await fetch(endpoint(`/v1/payment-sessions/${encodeURIComponent(token)}`), { cache: "no-store", headers: { Accept: "application/json" } });
  return parseResponse<PublicPaymentSession>(response);
}

export async function cancelPaymentSession(token: string): Promise<PublicPaymentSession> {
  const response = await fetch(endpoint(`/v1/payment-sessions/${encodeURIComponent(token)}/cancel`), { method: "POST", headers: { Accept: "application/json" } });
  return parseResponse<PublicPaymentSession>(response);
}

export function paymentEventsURL(token: string, afterSequence: number) {
  return endpoint(`/v1/payment-sessions/${encodeURIComponent(token)}/events?after_sequence=${afterSequence}`);
}

export function publicApiURL() { return API_URL; }

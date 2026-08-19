import type { TenantQRISTransaction } from "./types";

const terminalStatuses = new Set<TenantQRISTransaction["status"]>(["paid", "expired", "failed", "cancelled"]);

interface TenantQRISControllerOptions {
  apiBase: string;
  tenantId: string;
  transaction: TenantQRISTransaction;
  apiKey: string;
  onUpdate: (transaction: TenantQRISTransaction) => void;
  onError?: (error: unknown) => void;
}

interface ErrorPayload {
  error?: string;
  transaction?: TenantQRISTransaction;
}

export class TenantQRISRequestError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly transaction?: TenantQRISTransaction,
  ) {
    super(message);
    this.name = "TenantQRISRequestError";
  }
}

export function createTenantQRISController(options: TenantQRISControllerOptions) {
  let transaction = options.transaction;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopped = true;
  let pollingGeneration = 0;
  const apiBase = options.apiBase.replace(/\/$/, "");
  const transactionURL = `${apiBase}/v1/tenants/${encodeURIComponent(options.tenantId)}/transactions/qris/${encodeURIComponent(transaction.id)}`;
  const headers = { Accept: "application/json", "X-API-Key": options.apiKey };

  function publish(next: TenantQRISTransaction) {
    transaction = next;
    options.onUpdate(next);
  }

  function stopPolling() {
    stopped = true;
    pollingGeneration += 1;
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
  }

  function schedule() {
    if (stopped || terminalStatuses.has(transaction.status)) return;
    const delaySeconds = Math.max(1, transaction.poll_after_seconds ?? 15);
    timer = setTimeout(() => void poll(), delaySeconds * 1000);
  }

  async function poll() {
    timer = undefined;
    if (stopped || terminalStatuses.has(transaction.status)) return;
    const generation = pollingGeneration;
    try {
      const response = await fetch(transactionURL, { method: "GET", headers, cache: "no-store" });
      const payload = await response.json().catch(() => ({})) as TenantQRISTransaction & ErrorPayload;
      if (stopped || generation !== pollingGeneration) return;
      if (!response.ok) throw new TenantQRISRequestError(payload.error ?? `Status QRIS gagal dibaca (${response.status}).`, response.status);
      publish(payload);
      if (terminalStatuses.has(payload.status)) stopPolling();
    } catch (error) {
      options.onError?.(error);
    }
    schedule();
  }

  async function cancel(): Promise<TenantQRISTransaction> {
    stopPolling();
    const response = await fetch(`${transactionURL}/cancel`, { method: "POST", headers });
    const payload = await response.json().catch(() => ({})) as TenantQRISTransaction & ErrorPayload;
    if (response.ok) {
      publish(payload);
      return payload;
    }
    if (payload.transaction && terminalStatuses.has(payload.transaction.status)) publish(payload.transaction);
    throw new TenantQRISRequestError(
      payload.error ?? `Transaksi QRIS gagal dibatalkan (${response.status}).`,
      response.status,
      payload.transaction,
    );
  }

  return {
    startPolling() {
      if (!terminalStatuses.has(transaction.status)) {
        stopped = false;
        schedule();
      }
    },
    stopPolling,
    cancel,
    current: () => transaction,
  };
}

import { afterEach, describe, expect, it, vi } from "vitest";
import { createTenantQRISController } from "./tenant-qris-client";
import type { TenantQRISTransaction } from "./types";

const pending: TenantQRISTransaction = {
  id: "trx_pending",
  status: "pending",
  requested_amount: 10_000,
  payable_amount: 10_037,
  unique_amount_code: 37,
  poll_after_seconds: 15,
};

describe("tenant QRIS client controller", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("polls sequentially with a timeout and stops at a terminal status", async () => {
    vi.useFakeTimers();
    const updates: TenantQRISTransaction[] = [];
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ ...pending, status: "paid" }), { status: 200 }),
    );
    const controller = createTenantQRISController({
      apiBase: "https://api.alpakyros.net",
      tenantId: "tenant_lite",
      transaction: pending,
      apiKey: "xl_live_test",
      onUpdate: (transaction) => updates.push(transaction),
    });

    controller.startPolling();
    expect(fetchMock).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(15_000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(updates.at(-1)?.status).toBe("paid");
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("stops polling before posting cancel and publishes the server response", async () => {
    vi.useFakeTimers();
    const updates: TenantQRISTransaction[] = [];
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ ...pending, status: "cancelled" }), { status: 200 }),
    );
    const controller = createTenantQRISController({
      apiBase: "https://api.alpakyros.net/",
      tenantId: "tenant_lite",
      transaction: pending,
      apiKey: "xl_live_test",
      onUpdate: (transaction) => updates.push(transaction),
    });

    controller.startPolling();
    const result = await controller.cancel();
    await vi.advanceTimersByTimeAsync(30_000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.alpakyros.net/v1/tenants/tenant_lite/transactions/qris/trx_pending/cancel",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({ "X-API-Key": "xl_live_test" }),
      }),
    );
    expect(result.status).toBe("cancelled");
    expect(updates).toEqual([expect.objectContaining({ status: "cancelled" })]);
  });

  it("does not fabricate cancelled state after a network failure", async () => {
    const updates: TenantQRISTransaction[] = [];
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("network down"));
    const controller = createTenantQRISController({
      apiBase: "https://api.alpakyros.net",
      tenantId: "tenant_lite",
      transaction: pending,
      apiKey: "xl_live_test",
      onUpdate: (transaction) => updates.push(transaction),
    });

    await expect(controller.cancel()).rejects.toThrow("network down");
    expect(updates).toEqual([]);
    expect(controller.current().status).toBe("pending");
  });

  it("uses the server transaction as source of truth when cancel conflicts", async () => {
    const updates: TenantQRISTransaction[] = [];
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({
        error: "transaction is already terminal",
        transaction: { ...pending, status: "paid" },
      }), { status: 409 }),
    );
    const controller = createTenantQRISController({
      apiBase: "https://api.alpakyros.net",
      tenantId: "tenant_lite",
      transaction: pending,
      apiKey: "xl_live_test",
      onUpdate: (transaction) => updates.push(transaction),
    });

    await expect(controller.cancel()).rejects.toMatchObject({ status: 409 });
    expect(updates).toEqual([expect.objectContaining({ status: "paid" })]);
    expect(controller.current().status).toBe("paid");
  });

  it("ignores a stale polling response after cancel succeeds", async () => {
    vi.useFakeTimers();
    let finishPoll!: (response: Response) => void;
    const pollingResponse = new Promise<Response>((resolve) => { finishPoll = resolve; });
    const updates: TenantQRISTransaction[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation((_, init) => {
      if (init?.method === "POST") {
        return Promise.resolve(new Response(JSON.stringify({ ...pending, status: "cancelled" }), { status: 200 }));
      }
      return pollingResponse;
    });
    const controller = createTenantQRISController({
      apiBase: "https://api.alpakyros.net",
      tenantId: "tenant_lite",
      transaction: pending,
      apiKey: "xl_live_test",
      onUpdate: (transaction) => updates.push(transaction),
    });

    controller.startPolling();
    await vi.advanceTimersByTimeAsync(15_000);
    await controller.cancel();
    finishPoll(new Response(JSON.stringify(pending), { status: 200 }));
    await vi.runAllTimersAsync();

    expect(controller.current().status).toBe("cancelled");
    expect(updates).toEqual([expect.objectContaining({ status: "cancelled" })]);
  });
});

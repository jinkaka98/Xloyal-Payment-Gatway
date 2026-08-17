import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TenantTransactionTable } from "./tenant-transaction-table";
import type { Tenant, TenantTransaction } from "@/lib/types";

const tenants: Tenant[] = [
  { id: "tenant-production", name: "Production tenant", merchantId: "merchant-a", siteUrl: "", callbackUrl: "", webhookUrl: "", sandboxMode: false, active: true, createdAt: "2026-08-17T00:00:00Z" },
  { id: "tenant-sandbox", name: "Sandbox tenant", merchantId: "merchant-a", siteUrl: "", callbackUrl: "", webhookUrl: "", sandboxMode: true, active: true, createdAt: "2026-08-17T00:00:00Z" },
];

const transactions: TenantTransaction[] = [
  { id: "invoice-production", tenantId: "tenant-production", merchantId: "merchant-a", kind: "invoice", mode: "production", requestSource: "tenant_api", idempotencyKey: "order-production", amount: 25000, payableAmount: 25000, uniqueAmountCode: 0, currency: "IDR", status: "paid", providerReference: "provider-1", validation: "matched", matchedTransactionId: "", createdAt: "2026-08-17T00:00:00Z", updatedAt: "2026-08-17T00:01:00Z", expiresAt: "2026-08-17T00:30:00Z", lastCheckedAt: "2026-08-17T00:01:00Z", checkCount: 1 },
  { id: "qris-sandbox", tenantId: "tenant-sandbox", merchantId: "merchant-a", kind: "qris", mode: "sandbox", requestSource: "tenant_api", idempotencyKey: "order-sandbox", amount: 31000, payableAmount: 31037, uniqueAmountCode: 37, currency: "IDR", status: "pending", providerReference: "", validation: "waiting_first_check", matchedTransactionId: "", createdAt: "2026-08-17T00:02:00Z", updatedAt: "2026-08-17T00:02:00Z", expiresAt: "2026-08-17T00:32:00Z", lastCheckedAt: undefined, checkCount: 0 },
];

describe("TenantTransactionTable", () => {
  it("shows production and sandbox transactions and filters by mode", () => {
    render(<TenantTransactionTable transactions={transactions} tenants={tenants} tenantNames={{ "tenant-production": "Production tenant", "tenant-sandbox": "Sandbox tenant" }} />);

    expect(screen.getAllByText("Production").length).toBeGreaterThan(1);
    expect(screen.getAllByText("Sandbox").length).toBeGreaterThan(1);
    fireEvent.change(screen.getByLabelText("Mode transaksi"), { target: { value: "sandbox" } });
    expect(screen.getByText("qris-sandbox")).toBeInTheDocument();
    expect(screen.getByText("Rp31.037")).toBeInTheDocument();
    expect(screen.getByText("Tagihan Rp31.000 + kode 37")).toBeInTheDocument();
    expect(screen.queryByText("invoice-production")).not.toBeInTheDocument();
  });
});

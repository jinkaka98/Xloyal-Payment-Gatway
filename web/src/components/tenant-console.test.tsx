import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TenantConsole } from "./tenant-console";
import type { QRISTemplate } from "@/lib/types";

const templates: QRISTemplate[] = [
  { id: "qris_lite_primary", tenant_id: "tenant_alpakyros_lite", name: "QRIS Alpakyros LITE", image_mime: "image/png", merchant_name: "ALPAKYROS", merchant_city: "JAKARTA", access_scope: "selected_tenant" as const, static_to_dynamic: true, max_requests_per_minute: 60, active: true, created_at: "2026-08-14T00:00:00Z" },
  { id: "qris_legacy_shared", name: "QRIS Shared Legacy", image_mime: "image/png", merchant_name: "ALPAKYROS", merchant_city: "JAKARTA", access_scope: "" as QRISTemplate["access_scope"], static_to_dynamic: true, max_requests_per_minute: 60, active: true, created_at: "2026-08-14T00:00:00Z" },
];

const tenant = {
  id: "tenant_alpakyros_lite",
  name: "Alpakyros LITE",
  merchantId: "merchant_qris",
  siteUrl: "https://lite.alpakyros.net",
  callbackUrl: "",
  webhookUrl: "",
  sandboxMode: false,
  active: true,
  createdAt: "2026-08-14T00:00:00Z",
};

describe("TenantConsole credentials and documentation", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("reveals and copies a stored API key from the edit dialog", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ api_key: "xl_live_revealed" }), { status: 200 }));
    render(<TenantConsole initialTenants={[tenant]} merchants={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    expect(screen.getAllByText("tenant_alpakyros_lite").length).toBeGreaterThan(1);
    fireEvent.click(screen.getByRole("button", { name: "Lihat API key" }));

    expect(await screen.findByText("xl_live_revealed")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants/tenant_alpakyros_lite/credentials", expect.objectContaining({ method: "GET" }));
    fireEvent.click(screen.getByRole("button", { name: "Salin API key" }));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith("xl_live_revealed"));
  });

  it("offers rotation when the legacy tenant key was never encrypted", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "api_key_rotation_required", rotation_required: true }), { status: 409 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ api_key: "xl_live_rotated" }), { status: 200 }));
    render(<TenantConsole initialTenants={[tenant]} merchants={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    fireEvent.click(screen.getByRole("button", { name: "Lihat API key" }));
    expect(await screen.findByRole("button", { name: "Rotasi API key" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Rotasi API key" }));
    expect(await screen.findByText("xl_live_rotated")).toBeInTheDocument();
  });

  it("keeps a rotated tenant recoverable after closing and reopening edit", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ api_key: "xl_live_rotated" }), { status: 200 }));
    render(<TenantConsole initialTenants={[{ ...tenant, apiKeyRecoverable: false }]} merchants={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    fireEvent.click(screen.getByRole("button", { name: "Rotasi API key" }));
    expect(await screen.findByText("xl_live_rotated")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Tutup" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    expect(screen.queryByRole("button", { name: "Rotasi API key" })).not.toBeInTheDocument();
  });

  it("documents the deployed Alpakyros QRIS routes with the selected tenant ID", () => {
    render(<TenantConsole initialTenants={[{ ...tenant, siteUrl: "https://lite.alpakyros.net/store" }]} merchants={[]} qrisTemplates={templates} />);
    fireEvent.click(screen.getByRole("button", { name: "Dokumentasi Alpakyros LITE" }));

    expect(screen.getAllByText(/https:\/\/api\.alpakyros\.net\/v1\/tenants\/tenant_alpakyros_lite\/transactions\/qris/).length).toBeGreaterThan(0);
    expect(screen.getByText(/GET \/v1\/tenants\/tenant_alpakyros_lite\/transactions\/qris\/\{transaction_id\}/)).toBeInTheDocument();
    expect(screen.getAllByText(/X-API-Key/).length).toBeGreaterThan(0);
    expect(screen.getByText("qris_lite_primary")).toBeInTheDocument();
    expect(screen.getByText("qris_legacy_shared")).toBeInTheDocument();
    expect(screen.getByText(/Shared .* ALPAKYROS/)).toBeInTheDocument();
    expect(screen.getAllByText(/https:\/\/api\.alpakyros\.net\/v1\/tenants\/tenant_alpakyros_lite\/qris\/templates/).length).toBeGreaterThan(0);
    expect(screen.getByText(/data:image\/png;base64/)).toBeInTheDocument();
    expect(screen.getByText(/Request browser hanya diterima dari origin/)).toHaveTextContent("https://lite.alpakyros.net");
    expect(screen.getByText(/Request browser hanya diterima dari origin/)).not.toHaveTextContent("/store");
  });

  it("documents unique amount fields and the payable amount example", () => {
    render(<TenantConsole initialTenants={[{ ...tenant, useUniqueAmountCode: true }]} merchants={[]} qrisTemplates={templates} />);
    fireEvent.click(screen.getByRole("button", { name: "Dokumentasi Alpakyros LITE" }));

    expect(screen.getByText(/^requested_amount:/)).toHaveTextContent("Rp10.000");
    expect(screen.getByText(/^payable_amount:/)).toHaveTextContent("Rp10.037");
    expect(screen.getByText(/^unique_amount_code:/)).toHaveTextContent("37");
    expect(screen.getByText(/Bayar tepat/)).toHaveTextContent("Rp10.037");
    expect(screen.getByText("Alur integrasi wajib")).toBeInTheDocument();
    expect(screen.getByText(/Sumber status order saat ini/)).toHaveTextContent("polling endpoint status");
    expect(screen.getAllByText(/Idempotency-Key/).length).toBeGreaterThan(0);
  });

  it("documents server-backed QRIS cancellation for tenant clients", () => {
    render(<TenantConsole initialTenants={[tenant]} merchants={[]} qrisTemplates={templates} />);
    fireEvent.click(screen.getByRole("button", { name: "Dokumentasi Alpakyros LITE" }));

    expect(screen.getAllByText(/transactions\/qris\/\{transaction_id\}\/cancel/).length).toBeGreaterThan(0);
    expect(screen.getByText(/Hentikan timer polling lokal sebelum mengirim cancel/)).toBeInTheDocument();
    expect(screen.getByText(/HTTP 409 berarti transaksi sudah terminal/)).toHaveTextContent("source of truth");
    expect(screen.getAllByText("cancelled").length).toBeGreaterThan(0);
    expect(screen.getByText(/QR transaksi cancelled/)).toHaveTextContent("410 Gone");
    expect(screen.getByText(/await qrisController.cancel/)).toBeInTheDocument();
  });

  it("deletes a tenant after explicit confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));
    render(<TenantConsole initialTenants={[tenant]} merchants={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "Hapus Alpakyros LITE" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/admin/tenants/tenant_alpakyros_lite", expect.objectContaining({ method: "DELETE" })));
    expect(screen.queryByText("Alpakyros LITE")).not.toBeInTheDocument();
  });

  it("keeps the tenant visible when deletion fails", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ error: "delete tenant failed" }), { status: 500 }));
    render(<TenantConsole initialTenants={[tenant]} merchants={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "Hapus Alpakyros LITE" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("delete tenant failed");
    expect(screen.getByText("Alpakyros LITE")).toBeInTheDocument();
  });

  it("persists sandbox mode from edit and shows the updated state", async () => {
    const merchant = { id: "merchant_qris", interactive_merchant_id: "00828", name: "QRIS", active: true, created_at: "2026-08-14T00:00:00Z" };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: tenant.id,
      name: tenant.name,
      merchant_id: tenant.merchantId,
      site_url: tenant.siteUrl,
      callback_url: "",
      webhook_url: "",
      sandbox_mode: true,
      active: true,
      created_at: tenant.createdAt,
    }), { status: 200 }));
    render(<TenantConsole initialTenants={[tenant]} merchants={[merchant]} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Sandbox mode" }));
    fireEvent.click(screen.getByRole("button", { name: "Simpan perubahan" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const request = fetchMock.mock.calls[0]?.[1];
    expect(JSON.parse(String(request?.body))).toMatchObject({ sandbox_mode: true });
    expect(await screen.findByText("Sandbox")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    const sandboxToggle = screen.getByRole("checkbox", { name: "Sandbox mode" });
    expect(sandboxToggle).toBeChecked();
    expect(sandboxToggle).toHaveAccessibleDescription("Izinkan request browser dari origin mana pun. API key tetap wajib.");
  });

  it("persists the unique amount code setting from edit", async () => {
    const merchant = { id: "merchant_qris", interactive_merchant_id: "00828", name: "QRIS", active: true, created_at: "2026-08-14T00:00:00Z" };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: tenant.id,
      name: tenant.name,
      merchant_id: tenant.merchantId,
      site_url: tenant.siteUrl,
      callback_url: "",
      webhook_url: "",
      sandbox_mode: false,
      use_unique_amount_code: true,
      active: true,
      created_at: tenant.createdAt,
    }), { status: 200 }));
    render(<TenantConsole initialTenants={[{ ...tenant, useUniqueAmountCode: false }]} merchants={[merchant]} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    const toggle = screen.getByRole("checkbox", { name: "Gunakan kode unik nominal" });
    expect(toggle).not.toBeChecked();
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole("button", { name: "Simpan perubahan" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const request = fetchMock.mock.calls[0]?.[1];
    expect(JSON.parse(String(request?.body))).toMatchObject({ use_unique_amount_code: true });
    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    expect(screen.getByRole("checkbox", { name: "Gunakan kode unik nominal" })).toBeChecked();
  });

  it("persists a bounded unique amount cooldown", async () => {
    const merchant = { id: "merchant_qris", interactive_merchant_id: "00828", name: "QRIS", active: true, created_at: "2026-08-14T00:00:00Z" };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: tenant.id, name: tenant.name, merchant_id: tenant.merchantId, site_url: tenant.siteUrl,
      callback_url: "", webhook_url: "", sandbox_mode: false, use_unique_amount_code: true,
      unique_amount_cooldown_minutes: 45, active: true, created_at: tenant.createdAt,
    }), { status: 200 }));
    render(<TenantConsole initialTenants={[{ ...tenant, useUniqueAmountCode: true, uniqueAmountCooldownMinutes: 30 }]} merchants={[merchant]} />);
    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    fireEvent.change(screen.getByLabelText(/Cooldown kode unik/), { target: { value: "45" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan perubahan" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toMatchObject({ unique_amount_cooldown_minutes: 45 });
    fireEvent.click(screen.getByRole("button", { name: "Edit Alpakyros LITE" }));
    expect(screen.getByLabelText(/Cooldown kode unik/)).toHaveValue(45);
  });
});

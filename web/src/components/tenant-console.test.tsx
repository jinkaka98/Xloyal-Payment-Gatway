import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TenantConsole } from "./tenant-console";

const tenant = {
  id: "tenant_alpakyros_lite",
  name: "Alpakyros LITE",
  merchantId: "merchant_qris",
  siteUrl: "https://lite.alpakyros.net",
  callbackUrl: "",
  webhookUrl: "",
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
    render(<TenantConsole initialTenants={[tenant]} merchants={[]} />);
    fireEvent.click(screen.getByRole("button", { name: "Dokumentasi Alpakyros LITE" }));

    expect(screen.getAllByText(/https:\/\/api\.alpakyros\.net\/v1\/tenants\/tenant_alpakyros_lite\/transactions\/qris/).length).toBeGreaterThan(0);
    expect(screen.getByText(/GET \/v1\/tenants\/tenant_alpakyros_lite\/transactions\/qris\/\{transaction_id\}/)).toBeInTheDocument();
    expect(screen.getAllByText(/X-API-Key/).length).toBeGreaterThan(0);
  });
});

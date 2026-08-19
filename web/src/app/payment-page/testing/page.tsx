"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { FolderOpen, Paintbrush, RotateCcw, Star } from "lucide-react";
import { PaymentPageRenderer } from "@/components/payment-page-renderer";
import { AdminPageHeader } from "@/components/payment-page-admin";
import type { PaymentStatus, PaymentThemeConfig, PublicPaymentSession } from "@/lib/payment-public";

type DemoState =
  | "pending"
  | "verifying"
  | "completed"
  | "failed"
  | "expired"
  | "cancelled"
  | "redirecting";

const states: Array<{
  key: DemoState;
  label: string;
  status: PaymentStatus;
  title: string;
}> = [
  {
    key: "pending",
    label: "Menunggu",
    status: "payment_pending",
    title: "Menunggu Pembayaran",
  },
  {
    key: "verifying",
    label: "Verifikasi",
    status: "payment_pending",
    title: "Memverifikasi Pembayaran",
  },
  {
    key: "completed",
    label: "Sukses",
    status: "paid",
    title: "Pembayaran Berhasil",
  },
  {
    key: "failed",
    label: "Gagal",
    status: "failed",
    title: "Pembayaran Gagal",
  },
  {
    key: "expired",
    label: "Kedaluwarsa",
    status: "expired",
    title: "Pembayaran Kedaluwarsa",
  },
  {
    key: "cancelled",
    label: "Dibatalkan",
    status: "cancelled",
    title: "Pembayaran Dibatalkan",
  },
  {
    key: "redirecting",
    label: "Redirect",
    status: "redirecting",
    title: "Pembayaran Berhasil",
  },
];

const viewports = ["mobile", "tablet", "desktop"] as const;

type Theme = {
  id: string;
  tenant_id: string;
  name: string;
  template: string;
  status: "DRAFT" | "PUBLISHED" | "ARCHIVED";
  version: number;
  is_default: boolean;
  config?: PaymentThemeConfig;
};

const LOCAL_STORAGE_KEY = "xloyal_custom_payment_themes_v2";

const initialFallbackConfig: PaymentThemeConfig = {
  schema_version: 1,
  template_key: "modern",
  colors: {
    primary: "#1A5C55",
    background: "#F4F6F2",
    surface: "#FFFFFF",
    text: "#18231F",
    muted: "#68766F",
    success: "#16805B",
    danger: "#B44444",
  },
  branding: { display_name: "Demo Merchant", tagline: "Pembayaran aman & terverifikasi" },
  layout: { max_width: 520, radius: 16, density: "comfortable" },
  payment_visibility: { show_qr: true, show_amount: true, show_description: true },
  timer: { enabled: true, warning_seconds: 120 },
  success_copy: {
    title: "Pembayaran Berhasil",
    message: "Terima kasih, pembayaran Anda telah berhasil kami terima dan diverifikasi.",
  },
  redirect_delay: 5,
};

export default function TestingModePage() {
  const [state, setState] = useState<DemoState>("pending");
  const [viewport, setViewport] = useState<(typeof viewports)[number]>("desktop");
  const [themes, setThemes] = useState<Theme[]>([]);
  const [activeTheme, setActiveTheme] = useState<Theme | null>(null);

  // Load themes from API and LocalStorage
  useEffect(() => {
    async function fetchThemes() {
      let loadedThemes: Theme[] = [];
      try {
        const response = await fetch("/api/admin/payment-themes", { cache: "no-store" });
        if (response.ok) {
          const apiThemes = (await response.json()) as Theme[];
          if (Array.isArray(apiThemes) && apiThemes.length > 0) {
            loadedThemes = apiThemes;
          }
        }
      } catch {
        // fallback
      }

      if (loadedThemes.length === 0) {
        try {
          const raw = localStorage.getItem(LOCAL_STORAGE_KEY);
          if (raw) {
            const parsed = JSON.parse(raw) as Theme[];
            if (Array.isArray(parsed) && parsed.length > 0) {
              loadedThemes = parsed;
            }
          }
        } catch {
          // ignore
        }
      }

      setThemes(loadedThemes);

      // Pick default theme
      const defaultTheme = loadedThemes.find((t) => t.is_default) ?? loadedThemes[0] ?? null;
      setActiveTheme(defaultTheme);
    }

    void fetchThemes();
  }, []);

  const selected = states.find((item) => item.key === state)!;
  const currentConfig: PaymentThemeConfig = activeTheme?.config ?? initialFallbackConfig;

  const session = useMemo<PublicPaymentSession>(
    () => ({
      session_id: "demo-session",
      invoice_id: "INV-DEMO-2026",
      status: selected.status,
      payment_status:
        selected.status === "paid"
          ? "paid"
          : selected.status === "failed"
            ? "failed"
            : selected.status === "expired"
              ? "expired"
              : "pending",
      amount: 1002,
      currency: "IDR",
      description: "Demo Transaksi Paket Layanan Digital",
      qr_payload: "000201010212demo-qris-xloyal-payload-2026",
      expires_at: new Date(Date.now() + 29 * 60_000 + 12_000).toISOString(),
      server_now: new Date().toISOString(),
      theme: {
        id: activeTheme?.id ?? "demo-theme",
        version: activeTheme?.version ?? 1,
        config: currentConfig,
      },
      redirect: {
        success_url: "#success",
        cancel_url: "#cancelled",
        failed_url: "#failed",
        expired_url: "#expired",
      },
    }),
    [selected.status, activeTheme, currentConfig],
  );

  return (
    <div className="page testing-page">
      <AdminPageHeader
        title="Testing Mode"
        description="Simulator live checkout QRIS menggunakan tema default yang Anda tetapkan pada menu Custom Web Payment."
        action={
          <Link href="/custom-web-payment" className="button" style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
            <Paintbrush size={14} /> Atur Tema & Preset
          </Link>
        }
      />

      <div className="testing-layout">
        {/* CONTROLS PANEL */}
        <section className="testing-controls-panel">
          <div style={{ marginBottom: "14px", paddingBottom: "14px", borderBottom: "1px solid var(--line)" }}>
            <p className="eyebrow" style={{ margin: "0 0 6px" }}>TEMA CHECKOUT AKTIF</p>
            {themes.length > 0 ? (
              <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
                <select
                  className="theme-select-dropdown"
                  style={{ width: "100%", maxWidth: "100%", height: "36px" }}
                  value={activeTheme?.id ?? ""}
                  onChange={(e) => {
                    const match = themes.find((t) => t.id === e.target.value);
                    if (match) setActiveTheme(match);
                  }}
                >
                  {themes.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name} [{t.status}] {t.is_default ? "⭐ (Default Utama)" : ""}
                    </option>
                  ))}
                </select>

                <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
                  {activeTheme?.is_default && (
                    <span className="studio-badge studio-badge-default">
                      <Star size={11} fill="currentColor" /> Default Checkout
                    </span>
                  )}
                  <span className="studio-badge studio-badge-template">
                    {currentConfig.colors?.background === "#0F1715" ? "Dark Mode" : "Light Mode"}
                  </span>
                </div>
              </div>
            ) : (
              <div style={{ fontSize: "12px", color: "var(--muted)" }}>
                Menggunakan tema bawaan sistem (Modern Teal Light).
              </div>
            )}
          </div>

          <p className="eyebrow">SIMULASI STATUS PEMBAYARAN</p>
          <h2 style={{ fontSize: "15px", marginBottom: "10px" }}>Pilih Status Transaksi</h2>
          
          <div className="state-selector" role="radiogroup" aria-label="Payment preview state">
            {states.map((item) => (
              <button
                type="button"
                key={item.key}
                className={`state-option ${state === item.key ? "selected" : ""}`}
                aria-pressed={state === item.key}
                onClick={() => setState(item.key)}
              >
                <span className="state-radio" aria-hidden="true" />
                {item.label}
              </button>
            ))}
          </div>

          <div className="testing-toolbar">
            <button
              type="button"
              className="button"
              onClick={() => setState("pending")}
              style={{ display: "inline-flex", alignItems: "center", gap: "4px" }}
            >
              <RotateCcw size={13} /> Reset
            </button>
            {viewports.map((item) => (
              <button
                type="button"
                className={`button ${viewport === item ? "button-primary" : ""}`}
                key={item}
                onClick={() => setViewport(item)}
              >
                {item.charAt(0).toUpperCase() + item.slice(1)}
              </button>
            ))}
          </div>
        </section>

        {/* PREVIEW FRAME */}
        <section className="testing-preview">
          <div className="theme-preview-toolbar">
            <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <strong>Live Checkout Preview</strong>
              <span style={{ fontSize: "11px", color: "var(--muted)" }}>· {activeTheme?.name ?? "Modern Teal"}</span>
            </div>
            <span style={{ fontSize: "12px", fontWeight: 700, color: "var(--accent)" }}>{selected.title}</span>
          </div>

          <div className={`theme-preview-frame theme-preview-${viewport}`}>
            <PaymentPageRenderer
              session={session}
              remainingSeconds={state === "expired" ? 0 : 872}
              statusLabel={selected.title}
              connected
              cancelling={false}
              onCancel={() => setState("cancelled")}
              onRedirect={() => setState("completed")}
            />
          </div>
        </section>
      </div>

      <div className="notice notice-info" style={{ marginTop: "18px" }}>
        <strong>LIVE PREVIEW SYNC</strong>
        <span>
          Layar ini secara otomatis menggunakan <strong>Tema Default ({activeTheme?.name ?? "Modern Teal"})</strong> yang Anda tetapkan pada menu Custom Web Payment.
        </span>
      </div>
    </div>
  );
}

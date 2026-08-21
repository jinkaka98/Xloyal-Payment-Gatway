"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CheckCircle2,
  Copy,
  FolderOpen,
  Layers,
  Layout,
  Monitor,
  Moon,
  Palette,
  Plus,
  RotateCcw,
  Save,
  SlidersHorizontal,
  Smartphone,
  Star,
  Sun,
  Tablet,
  Trash2,
  Upload,
  UploadCloud,
  X,
} from "lucide-react";
import { PaymentPageRenderer } from "@/components/payment-page-renderer";
import type {
  PaymentStatus,
  PaymentThemeConfig,
  PublicPaymentSession,
} from "@/lib/payment-public";

export type Theme = {
  id: string;
  tenant_id: string;
  name: string;
  template: string;
  status: "DRAFT" | "PUBLISHED" | "ARCHIVED";
  version: number;
  is_default: boolean;
  config: PaymentThemeConfig;
  created_at: string;
  updated_at: string;
};

const API = "/api/admin/payment-themes";

async function requireApiSuccess(response: Response, action: string): Promise<Response> {
  if (response.ok) return response;
  let detail = `${response.status} ${response.statusText}`.trim();
  try {
    const body = (await response.clone().json()) as { error?: string; detail?: string };
    detail = body.error ?? body.detail ?? detail;
  } catch {
    // Keep the HTTP status when the proxy did not return JSON.
  }
  throw new Error(`${action}: ${detail}`);
}
const colorKeys = [
  { key: "primary", label: "Brand Accent / Tombol", desc: "Tombol utama, sorotan brand & indikator aktif" },
  { key: "background", label: "Page Background", desc: "Warna latar belakang layar checkout" },
  { key: "surface", label: "Surface / Card Box", desc: "Warna latar panel kotak transaksi" },
  { key: "text", label: "Teks Utama", desc: "Judul merchant, status & nominal harga" },
  { key: "muted", label: "Teks Sekunder", desc: "Instruksi scan, slogan & label pembantu" },
  { key: "success", label: "Indikator Sukses", desc: "Centang & status pembayaran berhasil" },
  { key: "danger", label: "Indikator Alert / Gagal", desc: "Timer kritis & status pembayaran gagal" },
] as const;

type ModernModeKey = "light" | "dark";

const modernModes: Record<
  ModernModeKey,
  {
    label: string;
    tag: string;
    description: string;
    swatch: string[];
    colors: Required<NonNullable<PaymentThemeConfig["colors"]>>;
  }
> = {
  light: {
    label: "Modern Teal (Mode Terang)",
    tag: "Light Mode",
    description: "Tampilan cerah, bersih & elegan bernuansa Teal standar",
    swatch: ["#1A5C55", "#F4F6F2", "#FFFFFF"],
    colors: {
      primary: "#1A5C55",
      background: "#F4F6F2",
      surface: "#FFFFFF",
      text: "#18231F",
      muted: "#68766F",
      success: "#16805B",
      danger: "#B44444",
    },
  },
  dark: {
    label: "Modern Teal (Mode Gelap)",
    tag: "Dark Mode",
    description: "Tampilan gelap kontras tinggi dengan aksen Emerald Teal",
    swatch: ["#4ECDC4", "#0F1715", "#182421"],
    colors: {
      primary: "#4ECDC4",
      background: "#0F1715",
      surface: "#182421",
      text: "#F0FDF9",
      muted: "#8FA39D",
      success: "#34D399",
      danger: "#F87171",
    },
  },
};

const initialConfig: PaymentThemeConfig = {
  schema_version: 1,
  template_key: "modern",
  colors: modernModes.light.colors,
  branding: { display_name: "Demo Merchant", tagline: "Pembayaran aman & terverifikasi", logo_url: "" },
  layout: { max_width: 520, radius: 16, density: "comfortable" },
  payment_visibility: { show_qr: true, show_amount: true, show_description: true },
  timer: { enabled: true, warning_seconds: 120 },
  success_copy: {
    title: "Pembayaran Berhasil",
    message: "Terima kasih, pembayaran Anda telah kami terima dan diverifikasi.",
  },
  redirect_delay: 5,
};

function cfgFromTheme(theme?: Theme | null): PaymentThemeConfig {
  if (!theme?.config) return structuredClone(initialConfig);
  return {
    ...initialConfig,
    ...structuredClone(theme.config),
    colors: { ...initialConfig.colors, ...(theme.config.colors ?? {}) },
    branding: { ...initialConfig.branding, ...(theme.config.branding ?? {}) },
    layout: { ...initialConfig.layout, ...(theme.config.layout ?? {}) },
    payment_visibility: {
      ...initialConfig.payment_visibility,
      ...(theme.config.payment_visibility ?? {}),
    },
    timer: { ...initialConfig.timer, ...(theme.config.timer ?? {}) },
    success_copy: {
      ...initialConfig.success_copy,
      ...(theme.config.success_copy ?? {}),
    },
  };
}

const mockSession = (
  config: PaymentThemeConfig,
  status: PaymentStatus,
): PublicPaymentSession => ({
  session_id: "preview-session",
  invoice_id: "INV-2026-PREVIEW",
  status,
  payment_status:
    status === "paid"
      ? "paid"
      : status === "expired"
        ? "expired"
        : status === "failed"
          ? "failed"
          : "pending",
  requested_amount: 1000,
  amount: 1001,
  unique_amount_code: 1,
  qris_merchant_name: "XLOYAL MERCHANT",
  qris_merchant_city: "BANDAR LAMPUNG",
  currency: "IDR",
  description: "Pesanan #XL-88231 (1x Paket Layanan Digital)",
  qr_payload: "000201010212fixture-preview-qris-xloyal",
  expires_at: new Date(Date.now() + 29 * 60_000 + 12_000).toISOString(),
  server_now: new Date().toISOString(),
  theme: { id: "preview", version: 1, config },
  redirect: {
    success_url: "https://merchant.example/success",
    cancel_url: "https://merchant.example/cancelled",
    failed_url: "https://merchant.example/failed",
    expired_url: "https://merchant.example/expired",
  },
});

type InspectorTab = "layout" | "branding" | "colors" | "flow";

export default function CustomWebPaymentPage() {
  const [themes, setThemes] = useState<Theme[]>([]);
  const [selected, setSelected] = useState<Theme | null>(null);
  const [config, setConfig] = useState<PaymentThemeConfig>(initialConfig);
  const [name, setName] = useState("Tema Pembayaran Baru");
  const [activeTab, setActiveTab] = useState<InspectorTab>("layout");
  
  // Current mode (Light or Dark)
  const currentMode: ModernModeKey = useMemo(() => {
    return config.colors?.background === "#0F1715" ? "dark" : "light";
  }, [config.colors?.background]);

  // Modal for theme manager drawer
  const [showThemeModal, setShowThemeModal] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | "PUBLISHED" | "DRAFT" | "ARCHIVED">("ALL");

  // Simulator controls
  const [previewStatus, setPreviewStatus] = useState<PaymentStatus | "verifying">("payment_pending");
  const [viewport, setViewport] = useState<"mobile" | "tablet" | "desktop">("mobile");
  
  // Notifications & loading
  const [toast, setToast] = useState<{ message: string; type: "info" | "success" | "error" } | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  // Logo upload state & ref
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isDraggingLogo, setIsDraggingLogo] = useState(false);
  const [showUrlInput, setShowUrlInput] = useState(false);

  const showToast = (message: string, type: "info" | "success" | "error" = "info") => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 4000);
  };

  function handleLogoFile(file: File) {
    if (!file.type.startsWith("image/")) {
      showToast("File harus berupa gambar (PNG, JPG, SVG, WebP)", "error");
      return;
    }
    if (file.size > 32 * 1024) {
      showToast("Ukuran file logo maksimal 32KB agar dapat disimpan di konfigurasi tema", "error");
      return;
    }
    const reader = new FileReader();
    reader.onload = (e) => {
      const dataUrl = e.target?.result as string;
      if (dataUrl) {
        setConfig((c) => ({
          ...c,
          branding: { ...c.branding, logo_url: dataUrl },
        }));
        showToast("Logo berhasil diunggah!", "success");
      }
    };
    reader.onerror = () => {
      showToast("Gagal membaca file logo", "error");
    };
    reader.readAsDataURL(file);
  }

  const load = useCallback(async () => {
    let list: Theme[] = [];
    try {
      const response = await fetch(API, { cache: "no-store" });
      if (response.ok) {
        const apiThemes = (await response.json()) as Theme[];
        if (Array.isArray(apiThemes) && apiThemes.length > 0) {
          list = apiThemes;
        }
      }
    } catch {
      showToast("Tema tidak dapat dimuat dari server", "error");
    }

    setThemes(list);

    if (list.length > 0) {
      setSelected((prev) => {
        const next = (prev && list.find((t) => t.id === prev.id)) ?? list.find((t) => t.is_default) ?? list[0] ?? null;
        if (next) {
          setName(next.name);
          setConfig(cfgFromTheme(next));
        }
        return next;
      });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const preview = useMemo(
    () =>
      mockSession(
        { ...config, template_key: "modern" },
        previewStatus === "verifying" ? "payment_pending" : previewStatus,
      ),
    [config, previewStatus],
  );

  function selectTheme(theme: Theme) {
    setSelected(theme);
    setName(theme.name);
    setConfig(cfgFromTheme(theme));
  }

  function handleNewTheme() {
    setSelected(null);
    setName("Tema Pembayaran Baru (Draft)");
    setConfig(structuredClone(initialConfig));
    showToast("Membuat draft tema baru", "info");
    setShowThemeModal(false);
  }

  function switchModernMode(mode: ModernModeKey) {
    const selectedMode = modernModes[mode];
    setConfig((current) => ({
      ...current,
      colors: { ...current.colors, ...selectedMode.colors },
      template_key: "modern",
    }));
    showToast(`Mode ${selectedMode.tag} diterapkan`, "info");
  }

  function updateColor(key: string, value: string) {
    setConfig((current) => ({
      ...current,
      colors: { ...current.colors, [key]: value },
    }));
  }

  function resetColorsToPreset() {
    const presetCol = modernModes[currentMode].colors;
    setConfig((current) => ({
      ...current,
      colors: { ...current.colors, ...presetCol },
    }));
    showToast(`Warna direset ke ${modernModes[currentMode].label}`, "info");
  }

  async function save() {
    setIsSaving(true);
    const cleanConfig = { ...config, template_key: "modern" };

    try {
      if (!selected) {
        // Create new Theme
        const newTheme: Theme = {
          id: `theme-${Date.now()}`,
          tenant_id: "",
          name: name.trim() || "Tema Pembayaran Baru",
          template: "modern",
          status: "DRAFT",
          version: 1,
          is_default: themes.length === 0,
          config: cleanConfig,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        };

        // Try API
        try {
          const response = await fetch(API, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              tenant_id: "",
              name: newTheme.name,
              template: "modern",
              config: cleanConfig,
            }),
          });
          await requireApiSuccess(response, "Gagal membuat tema");
          const apiCreated = (await response.json()) as Theme;
          newTheme.id = apiCreated.id || newTheme.id;
        } catch {
          showToast("Tema tidak tersimpan di server", "error");
          return;
        }

        const updatedList = [newTheme, ...themes];
        setThemes(updatedList);
        setSelected(newTheme);
        showToast("Tema baru berhasil disimpan sebagai DRAFT!", "success");
      } else {
        // Update existing Theme
        const updatedTheme: Theme = {
          ...selected,
          name: name.trim() || selected.name,
          config: cleanConfig,
          version: (selected.version || 1) + 1,
          updated_at: new Date().toISOString(),
        };

        // Try API
        let savedTheme: Partial<Theme> = {};
        try {
          const response = await fetch(`${API}/${selected.id}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              name: updatedTheme.name,
              config: cleanConfig,
            }),
          });
          const savedResponse = await requireApiSuccess(response, "Gagal memperbarui tema");
          savedTheme = (await savedResponse.json()) as Partial<Theme>;
          if (selected.status === "PUBLISHED") {
            const publishResponse = await fetch(`${API}/${selected.id}/publish`, { method: "POST" });
            const publishedResponse = await requireApiSuccess(publishResponse, "Gagal menerapkan tema");
            const publishedTheme = (await publishedResponse.json()) as Partial<Theme>;
            Object.assign(savedTheme, publishedTheme);
          }
        } catch {
          showToast("Perubahan tema tidak tersimpan di server", "error");
          return;
        }

        const updatedList = themes.map((t) => (t.id === selected.id ? { ...updatedTheme, ...savedTheme, config: cleanConfig } : t));
        const appliedTheme = updatedList.find((t) => t.id === selected.id) ?? updatedTheme;
        setThemes(updatedList);
        setSelected(appliedTheme);
        showToast(selected.status === "PUBLISHED" ? "Perubahan tema disimpan dan diterapkan ke checkout live!" : "Perubahan tema berhasil disimpan!", "success");
      }
    } catch {
      showToast("Gagal menyimpan tema", "error");
    } finally {
      setIsSaving(false);
    }
  }

  async function setDefaultTheme(targetId: string) {
    // Try API
    try {
      const response = await fetch(`${API}/${targetId}/set-default`, { method: "POST" });
      await requireApiSuccess(response, "Gagal menetapkan tema default");
    } catch {
      showToast("Tema default tidak berubah di server", "error");
      return;
    }

    const updatedList = themes.map((t) => ({
      ...t,
      is_default: t.id === targetId,
      status: t.id === targetId ? ("PUBLISHED" as const) : t.status,
    }));

    setThemes(updatedList);

    const match = updatedList.find((t) => t.id === targetId);
    if (match) {
      setSelected(match);
      setName(match.name);
      setConfig(cfgFromTheme(match));
    }
    showToast("⭐ Tema berhasil dijadikan Default untuk seluruh checkout!", "success");
  }

  async function publishTheme(targetId: string) {
    try {
      const response = await fetch(`${API}/${targetId}/publish`, { method: "POST" });
      await requireApiSuccess(response, "Gagal mempublish tema");
    } catch {
      showToast("Tema tidak dipublish di server", "error");
      return;
    }

    const updatedList = themes.map((t) =>
      t.id === targetId ? { ...t, status: "PUBLISHED" as const, updated_at: new Date().toISOString() } : t,
    );
    setThemes(updatedList);

    const match = updatedList.find((t) => t.id === targetId);
    if (match) {
      setSelected(match);
    }
    showToast("Tema berhasil dipublish untuk live checkout!", "success");
  }

  async function duplicateTheme(targetId: string) {
    const source = themes.find((t) => t.id === targetId);
    if (!source) return;

    const copy: Theme = {
      ...source,
      id: `theme-${Date.now()}`,
      name: `${source.name} (Salinan)`,
      status: "DRAFT",
      is_default: false,
      version: 1,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    try {
      const response = await fetch(`${API}/${targetId}/duplicate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: copy.name }),
      });
      await requireApiSuccess(response, "Gagal menggandakan tema");
      const apiCopy = (await response.json()) as Partial<Theme>;
      if (apiCopy.id) copy.id = apiCopy.id;
    } catch {
      showToast("Tema tidak berhasil digandakan di server", "error");
      return;
    }

    const updatedList = [copy, ...themes];
    setThemes(updatedList);
    selectTheme(copy);
    showToast("Tema berhasil digandakan!", "success");
  }

  async function deleteTheme(targetId: string) {
    try {
      const response = await fetch(`${API}/${targetId}`, { method: "DELETE" });
      await requireApiSuccess(response, "Gagal menghapus tema");
    } catch {
      showToast("Tema tidak dihapus di server", "error");
      return;
    }

    const updatedList = themes.filter((t) => t.id !== targetId);
    setThemes(updatedList);

    if (selected?.id === targetId) {
      const nextTheme = updatedList.find((t) => t.is_default) ?? updatedList[0] ?? null;
      if (nextTheme) {
        selectTheme(nextTheme);
      } else {
        handleNewTheme();
      }
    }
    showToast("Tema berhasil dihapus", "info");
  }

  const filteredThemes = useMemo(() => {
    return themes.filter((t) => {
      const matchesSearch = t.name.toLowerCase().includes(searchQuery.toLowerCase());
      const matchesStatus = statusFilter === "ALL" || t.status === statusFilter;
      return matchesSearch && matchesStatus;
    });
  }, [themes, searchQuery, statusFilter]);

  return (
    <div className="page theme-builder-page">
      {/* 1. TOP BAR */}
      <header className="theme-studio-topbar">
        <div className="theme-studio-title-area">
          {/* Theme list modal button */}
          <button
            type="button"
            className="theme-list-btn"
            onClick={() => setShowThemeModal(true)}
            title="Buka daftar dan kelola tema tersimpan"
          >
            <FolderOpen size={15} />
            <strong>Daftar Tema Tersimpan</strong>
            <span className="theme-count-badge">{themes.length}</span>
          </button>

          {/* Direct Switcher Dropdown */}
          <div className="theme-selector-wrap">
            <select
              className="theme-select-dropdown"
              value={selected?.id ?? "new"}
              onChange={(e) => {
                if (e.target.value === "new") {
                  handleNewTheme();
                } else {
                  const targetTheme = themes.find((t) => t.id === e.target.value);
                  if (targetTheme) selectTheme(targetTheme);
                }
              }}
            >
              {selected ? null : <option value="new">+ Draft Baru (Belum Disimpan)</option>}
              {themes.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name} [{t.status}] {t.is_default ? "⭐ DEFAULT" : ""}
                </option>
              ))}
            </select>

            <button
              type="button"
              className="button button-new-theme"
              onClick={handleNewTheme}
              title="Mulai buat tema pembayaran baru"
            >
              <Plus size={14} />
              <span>Tema Baru</span>
            </button>
          </div>

          {/* Badges */}
          <div className="theme-studio-badges">
            {selected && (
              <span className={`studio-badge studio-badge-${selected.status.toLowerCase()}`}>
                {selected.status}
              </span>
            )}
            {selected?.is_default && (
              <span className="studio-badge studio-badge-default">
                <Star size={12} fill="currentColor" /> Default
              </span>
            )}
            <span className="studio-badge studio-badge-template">
              Preset: Modern Teal ({currentMode === "dark" ? "Dark Mode" : "Light Mode"})
            </span>
          </div>
        </div>

        {/* Action Controls */}
        <div className="theme-studio-actions">
          {toast && (
            <div className={`studio-toast studio-toast-${toast.type}`}>
              {toast.type === "success" && <CheckCircle2 size={14} />}
              <span>{toast.message}</span>
            </div>
          )}

          {/* SAVE BUTTON */}
          <button
            type="button"
            className="button button-primary button-save-theme"
            onClick={() => void save()}
            disabled={isSaving}
          >
            <Save size={15} />
            <strong>{selected ? (isSaving ? "Menyimpan..." : "Simpan Perubahan") : "Simpan Tema"}</strong>
          </button>

          {/* PUBLISH BUTTON */}
          {selected && selected.status === "DRAFT" && (
            <button
              type="button"
              className="button button-publish-theme"
              onClick={() => void publishTheme(selected.id)}
              title="Publish tema ini untuk digunakan pada live checkout"
            >
              <Upload size={14} />
              Publish
            </button>
          )}

          {/* SET AS DEFAULT BUTTON */}
          {selected && !selected.is_default && (
            <button
              type="button"
              className="button button-default-theme"
              onClick={() => void setDefaultTheme(selected.id)}
              title="Jadikan tema ini sebagai default utama untuk seluruh checkout"
            >
              <Star size={14} />
              Jadikan Default
            </button>
          )}

          {/* DUPLICATE BUTTON */}
          {selected && (
            <button
              type="button"
              className="button"
              style={{ minHeight: "36px", padding: "0 11px" }}
              title="Gandakan tema ini"
              onClick={() => void duplicateTheme(selected.id)}
            >
              <Copy size={14} />
            </button>
          )}

          {/* DELETE BUTTON */}
          {selected && !selected.is_default && (
            <button
              type="button"
              className="button"
              style={{ color: "var(--red)", minHeight: "36px", padding: "0 10px" }}
              title="Hapus tema ini"
              onClick={() => void deleteTheme(selected.id)}
            >
              <Trash2 size={14} />
            </button>
          )}
        </div>
      </header>

      {/* 2. 2-PANE STUDIO MAIN LAYOUT */}
      <div className="theme-studio-2pane">
        {/* PANE 1: INSPECTOR & SETTINGS (LEFT) */}
        <section className="theme-inspector-card">
          <nav className="theme-tab-nav">
            <button
              type="button"
              className={`theme-tab-btn ${activeTab === "layout" ? "active" : ""}`}
              onClick={() => setActiveTab("layout")}
            >
              <Layout size={14} />
              Layout & Mode
            </button>
            <button
              type="button"
              className={`theme-tab-btn ${activeTab === "branding" ? "active" : ""}`}
              onClick={() => setActiveTab("branding")}
            >
              <Layers size={14} />
              Branding
            </button>
            <button
              type="button"
              className={`theme-tab-btn ${activeTab === "colors" ? "active" : ""}`}
              onClick={() => setActiveTab("colors")}
            >
              <Palette size={14} />
              Warna
            </button>
            <button
              type="button"
              className={`theme-tab-btn ${activeTab === "flow" ? "active" : ""}`}
              onClick={() => setActiveTab("flow")}
            >
              <SlidersHorizontal size={14} />
              Flow & Konten
            </button>
          </nav>

          <div className="theme-inspector-content">
            {/* TAB 1: LAYOUT & PRESET MODE */}
            {activeTab === "layout" && (
              <div className="inspector-section">
                <div className="form-field">
                  <label>
                    Nama Tema
                    <span>Identifikasi tema pada daftar</span>
                  </label>
                  <input
                    type="text"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Contoh: Checkout Modern Teal"
                  />
                </div>

                <div className="inspector-group">
                  <p className="inspector-section-title">Pilihan Mode Desain (Modern Teal)</p>

                  <div className="preset-grid-v2" style={{ gridTemplateColumns: "1fr 1fr" }}>
                    <button
                      type="button"
                      className={`preset-card-v2 ${currentMode === "light" ? "selected" : ""}`}
                      onClick={() => switchModernMode("light")}
                    >
                      <div className="preset-swatch-v2">
                        {modernModes.light.swatch.map((c) => (
                          <i key={c} style={{ backgroundColor: c }} />
                        ))}
                      </div>
                      <div className="preset-card-head">
                        <strong>
                          <Sun size={13} style={{ verticalAlign: "-2px", marginRight: "3px" }} />
                          {modernModes.light.label}
                        </strong>
                        <span className="preset-card-tag">{modernModes.light.tag}</span>
                      </div>
                      <small>{modernModes.light.description}</small>
                    </button>

                    <button
                      type="button"
                      className={`preset-card-v2 ${currentMode === "dark" ? "selected" : ""}`}
                      onClick={() => switchModernMode("dark")}
                    >
                      <div className="preset-swatch-v2">
                        {modernModes.dark.swatch.map((c) => (
                          <i key={c} style={{ backgroundColor: c }} />
                        ))}
                      </div>
                      <div className="preset-card-head">
                        <strong>
                          <Moon size={13} style={{ verticalAlign: "-2px", marginRight: "3px" }} />
                          {modernModes.dark.label}
                        </strong>
                        <span className="preset-card-tag">{modernModes.dark.tag}</span>
                      </div>
                      <small>{modernModes.dark.description}</small>
                    </button>
                  </div>
                </div>

                <div className="inspector-group" style={{ marginTop: "6px" }}>
                  <p className="inspector-section-title">Dimensi & Bentuk Kontainer</p>
                  
                  <div className="form-field">
                    <label>
                      Lebar Maksimal ({config.layout?.max_width ?? 520}px)
                      <span>Rentang 400px - 880px</span>
                    </label>
                    <div className="range-group">
                      <input
                        type="range"
                        min="400"
                        max="880"
                        step="20"
                        value={config.layout?.max_width ?? 520}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            layout: { ...c.layout, max_width: Number(e.target.value) },
                          }))
                        }
                      />
                      <span className="range-badge">{config.layout?.max_width ?? 520}px</span>
                    </div>
                  </div>

                  <div className="form-field">
                    <label>
                      Sudut Melengkung / Radius ({config.layout?.radius ?? 16}px)
                      <span>0px (Kotak) - 24px (Rounded)</span>
                    </label>
                    <div className="range-group">
                      <input
                        type="range"
                        min="0"
                        max="24"
                        step="2"
                        value={config.layout?.radius ?? 16}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            layout: { ...c.layout, radius: Number(e.target.value) },
                          }))
                        }
                      />
                      <span className="range-badge">{config.layout?.radius ?? 16}px</span>
                    </div>
                  </div>

                  <div className="form-field">
                    <label>Kepadatan Layout (Density)</label>
                    <select
                      value={config.layout?.density ?? "comfortable"}
                      onChange={(e) =>
                        setConfig((c) => ({
                          ...c,
                          layout: {
                            ...c.layout,
                            density: e.target.value as "comfortable" | "compact",
                          },
                        }))
                      }
                    >
                      <option value="comfortable">Comfortable (Lapang, Bersih & Nyaman)</option>
                      <option value="compact">Compact (Padat & Hemat Ruang)</option>
                    </select>
                  </div>
                </div>
              </div>
            )}

            {/* TAB 2: BRANDING */}
            {activeTab === "branding" && (
              <div className="inspector-section">
                <p className="inspector-section-title">Identitas Bisnis / Merchant</p>
                <div className="form-field">
                  <label>
                    Nama Merchant
                    <span>Ditampilkan pada header checkout</span>
                  </label>
                  <input
                    type="text"
                    value={config.branding?.display_name ?? ""}
                    onChange={(e) =>
                      setConfig((c) => ({
                        ...c,
                        branding: {
                          ...c.branding,
                          display_name: e.target.value,
                        },
                      }))
                    }
                    placeholder="Contoh: Toko Kopi Nusantara"
                  />
                </div>

                <div className="form-field">
                  <label>
                    Slogan / Tagline
                    <span>Subteks singkat di bawah nama merchant</span>
                  </label>
                  <input
                    type="text"
                    value={config.branding?.tagline ?? ""}
                    onChange={(e) =>
                      setConfig((c) => ({
                        ...c,
                        branding: {
                          ...c.branding,
                          tagline: e.target.value,
                        },
                      }))
                    }
                    placeholder="Contoh: Pembayaran resmi & terverifikasi"
                  />
                </div>

                <div className="form-field">
                  <label>
                    Logo Brand / Merchant
                    <span>Format PNG, SVG, JPG, WebP (Maks. 2MB)</span>
                  </label>

                  {/* Hidden native file input */}
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept="image/png,image/jpeg,image/svg+xml,image/webp,image/gif"
                    style={{ display: "none" }}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) handleLogoFile(file);
                      e.target.value = "";
                    }}
                  />

                  {config.branding?.logo_url ? (
                    <div className="logo-card-active">
                      <div className="logo-thumb-wrapper">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img
                          src={config.branding.logo_url}
                          alt="Logo Merchant"
                          className="logo-thumb-img"
                        />
                      </div>
                      <div className="logo-card-info">
                        <strong>Logo Terpasang</strong>
                        <span>
                          {config.branding.logo_url.startsWith("data:")
                            ? "File lokal berhasil diunggah"
                            : config.branding.logo_url}
                        </span>
                      </div>
                      <div className="logo-card-actions">
                        <button
                          type="button"
                          className="button"
                          style={{ minHeight: "32px", padding: "0 9px", fontSize: "11px" }}
                          onClick={() => fileInputRef.current?.click()}
                          title="Ganti file logo"
                        >
                          <UploadCloud size={13} />
                          Ganti
                        </button>
                        <button
                          type="button"
                          className="button"
                          style={{ color: "var(--red)", minHeight: "32px", padding: "0 8px", fontSize: "11px" }}
                          onClick={() =>
                            setConfig((c) => ({
                              ...c,
                              branding: { ...c.branding, logo_url: "" },
                            }))
                          }
                          title="Hapus logo"
                        >
                          <Trash2 size={13} />
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div
                      className={`logo-dropzone ${isDraggingLogo ? "dragging" : ""}`}
                      onClick={() => fileInputRef.current?.click()}
                      onDragOver={(e) => {
                        e.preventDefault();
                        setIsDraggingLogo(true);
                      }}
                      onDragLeave={() => setIsDraggingLogo(false)}
                      onDrop={(e) => {
                        e.preventDefault();
                        setIsDraggingLogo(false);
                        const file = e.dataTransfer.files?.[0];
                        if (file) handleLogoFile(file);
                      }}
                    >
                      <div className="logo-dropzone-icon">
                        <UploadCloud size={20} />
                      </div>
                      <div className="logo-dropzone-text">
                        <strong>Klik untuk unggah atau seret file logo ke sini</strong>
                        <span>Mendukung PNG transparan, SVG, JPG, WebP (Maks. 2MB)</span>
                      </div>
                    </div>
                  )}

                  {/* Optional External URL input toggle */}
                  <div style={{ marginTop: "4px" }}>
                    <button
                      type="button"
                      className="text-button"
                      style={{ fontSize: "11px", color: "var(--muted)" }}
                      onClick={() => setShowUrlInput(!showUrlInput)}
                    >
                      {showUrlInput ? "▾ Sembunyikan Input URL" : "▸ Atau masukkan tautan URL Logo Eksternal"}
                    </button>
                    {showUrlInput && (
                      <div style={{ marginTop: "6px" }}>
                        <input
                          type="url"
                          value={config.branding?.logo_url ?? ""}
                          onChange={(e) =>
                            setConfig((c) => ({
                              ...c,
                              branding: {
                                ...c.branding,
                                logo_url: e.target.value,
                              },
                            }))
                          }
                          placeholder="https://domain-anda.com/logo.png"
                        />
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}

            {/* TAB 3: PALET WARNA */}
            {activeTab === "colors" && (
              <div className="inspector-section">
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                  }}
                >
                  <p className="inspector-section-title" style={{ margin: 0 }}>
                    Palet Warna UI ({modernModes[currentMode].label})
                  </p>
                  <button
                    type="button"
                    className="text-button"
                    style={{ fontSize: "11px", display: "flex", alignItems: "center", gap: "4px" }}
                    onClick={resetColorsToPreset}
                  >
                    <RotateCcw size={12} /> Reset Warna
                  </button>
                </div>

                <div className="color-editor-grid-v2">
                  {colorKeys.map(({ key, label, desc }) => {
                    const currentColor =
                      config.colors?.[key] ?? modernModes[currentMode].colors[key];
                    return (
                      <div key={key} className="color-item-v2">
                        <div className="color-item-info">
                          <strong>{label}</strong>
                          <span>{desc}</span>
                        </div>
                        <div className="color-item-controls">
                          <input
                            type="text"
                            className="color-hex-input"
                            value={currentColor}
                            onChange={(e) => updateColor(key, e.target.value)}
                            maxLength={7}
                          />
                          <label className="color-picker-wrapper" title="Buka color picker">
                            <input
                              type="color"
                              value={currentColor}
                              onChange={(e) => updateColor(key, e.target.value)}
                            />
                          </label>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {/* TAB 4: FLOW, KONTEN & TIMER */}
            {activeTab === "flow" && (
              <div className="inspector-section">
                <p className="inspector-section-title">Visibilitas Komponen</p>
                <div className="inspector-group">
                  <div className="toggle-card">
                    <div className="toggle-card-info">
                      <strong>Tampilkan QR Code QRIS</strong>
                      <small>Sembunyikan jika hanya menampilkan petunjuk pembayaran</small>
                    </div>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={config.payment_visibility?.show_qr ?? true}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            payment_visibility: {
                              ...c.payment_visibility,
                              show_qr: e.target.checked,
                            },
                          }))
                        }
                      />
                      <span className="slider" />
                    </label>
                  </div>

                  <div className="toggle-card">
                    <div className="toggle-card-info">
                      <strong>Tampilkan Nominal Pembayaran</strong>
                      <small>Tampilkan total nominal harga secara jelas</small>
                    </div>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={config.payment_visibility?.show_amount ?? true}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            payment_visibility: {
                              ...c.payment_visibility,
                              show_amount: e.target.checked,
                            },
                          }))
                        }
                      />
                      <span className="slider" />
                    </label>
                  </div>

                  <div className="toggle-card">
                    <div className="toggle-card-info">
                      <strong>Tampilkan Deskripsi Pesanan</strong>
                      <small>Tampilkan rincian invoice/keterangan transaksi</small>
                    </div>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={config.payment_visibility?.show_description ?? true}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            payment_visibility: {
                              ...c.payment_visibility,
                              show_description: e.target.checked,
                            },
                          }))
                        }
                      />
                      <span className="slider" />
                    </label>
                  </div>
                </div>

                <p className="inspector-section-title" style={{ marginTop: "10px" }}>
                  Countdown Batas Waktu Bayar
                </p>
                <div className="inspector-group">
                  <div className="toggle-card">
                    <div className="toggle-card-info">
                      <strong>Aktifkan Batas Waktu Bayar</strong>
                      <small>Tampilkan countdown waktu tersisa sebelum kedaluwarsa</small>
                    </div>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={config.timer?.enabled ?? true}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            timer: {
                              ...c.timer,
                              enabled: e.target.checked,
                            },
                          }))
                        }
                      />
                      <span className="slider" />
                    </label>
                  </div>

                  {config.timer?.enabled && (
                    <div className="form-field">
                      <label>
                        Ambang Peringatan Waktu Kritis ({config.timer?.warning_seconds ?? 120}s)
                        <span>Waktu countdown mulai berkedip merah</span>
                      </label>
                      <div className="range-group">
                        <input
                          type="range"
                          min="30"
                          max="300"
                          step="15"
                          value={config.timer?.warning_seconds ?? 120}
                          onChange={(e) =>
                            setConfig((c) => ({
                              ...c,
                              timer: {
                                ...c.timer,
                                warning_seconds: Number(e.target.value),
                              },
                            }))
                          }
                        />
                        <span className="range-badge">{config.timer?.warning_seconds ?? 120}s</span>
                      </div>
                    </div>
                  )}
                </div>

                <p className="inspector-section-title" style={{ marginTop: "10px" }}>
                  Pesan Sukses & Pengalihan
                </p>
                <div className="inspector-group">
                  <div className="form-field">
                    <label>Judul Pembayaran Berhasil</label>
                    <input
                      type="text"
                      value={config.success_copy?.title ?? ""}
                      onChange={(e) =>
                        setConfig((c) => ({
                          ...c,
                          success_copy: {
                            ...c.success_copy,
                            title: e.target.value,
                          },
                        }))
                      }
                      placeholder="Pembayaran Berhasil"
                    />
                  </div>

                  <div className="form-field">
                    <label>Pesan Tambahan Sukses</label>
                    <textarea
                      value={config.success_copy?.message ?? ""}
                      onChange={(e) =>
                        setConfig((c) => ({
                          ...c,
                          success_copy: {
                            ...c.success_copy,
                            message: e.target.value,
                          },
                        }))
                      }
                      placeholder="Terima kasih, pembayaran Anda telah berhasil kami terima."
                    />
                  </div>

                  <div className="form-field">
                    <label>
                      Jeda Pengalihan Otomatis ({config.redirect_delay ?? 5} detik)
                      <span>Waktu tunggu sebelum kembali ke merchant</span>
                    </label>
                    <div className="range-group">
                      <input
                        type="range"
                        min="0"
                        max="15"
                        step="1"
                        value={config.redirect_delay ?? 5}
                        onChange={(e) =>
                          setConfig((c) => ({
                            ...c,
                            redirect_delay: Number(e.target.value),
                          }))
                        }
                      />
                      <span className="range-badge">{config.redirect_delay ?? 5}s</span>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        </section>

        {/* PANE 2: LIVE PREVIEW CANVAS (RIGHT) */}
        <section className="theme-preview-card">
          <div className="preview-control-bar">
            <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <span style={{ fontSize: "12px", fontWeight: 750, color: "var(--text)" }}>
                Live Interactive Preview
              </span>
              <span
                style={{
                  fontSize: "10px",
                  color: "var(--green)",
                  background: "var(--green-bg)",
                  padding: "2px 6px",
                  borderRadius: "4px",
                  fontWeight: 700,
                }}
              >
                ● Real-time
              </span>
            </div>

            {/* DEVICE TOGGLES */}
            <div className="device-toggle-group" role="radiogroup" aria-label="Device viewport">
              <button
                type="button"
                className={`device-btn ${viewport === "mobile" ? "active" : ""}`}
                onClick={() => setViewport("mobile")}
              >
                <Smartphone size={13} />
                Mobile
              </button>
              <button
                type="button"
                className={`device-btn ${viewport === "tablet" ? "active" : ""}`}
                onClick={() => setViewport("tablet")}
              >
                <Tablet size={13} />
                Tablet
              </button>
              <button
                type="button"
                className={`device-btn ${viewport === "desktop" ? "active" : ""}`}
                onClick={() => setViewport("desktop")}
              >
                <Monitor size={13} />
                Desktop
              </button>
            </div>
          </div>

          {/* SIMULATION STATUS TRAY */}
          <div className="status-simulation-tray">
            <span style={{ fontSize: "11px", fontWeight: 600, color: "var(--muted)", marginRight: "2px" }}>
              Status:
            </span>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "payment_pending" ? "active" : ""}`}
              onClick={() => setPreviewStatus("payment_pending")}
            >
              ⏳ Menunggu
            </button>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "verifying" ? "active" : ""}`}
              onClick={() => setPreviewStatus("verifying")}
            >
              🔄 Verifikasi
            </button>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "paid" ? "active" : ""}`}
              onClick={() => setPreviewStatus("paid")}
            >
              ✅ Sukses
            </button>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "failed" ? "active" : ""}`}
              onClick={() => setPreviewStatus("failed")}
            >
              ❌ Gagal
            </button>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "expired" ? "active" : ""}`}
              onClick={() => setPreviewStatus("expired")}
            >
              ⌛ Kedaluwarsa
            </button>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "cancelled" ? "active" : ""}`}
              onClick={() => setPreviewStatus("cancelled")}
            >
              🚫 Dibatalkan
            </button>
            <button
              type="button"
              className={`status-pill-btn ${previewStatus === "redirecting" ? "active" : ""}`}
              onClick={() => setPreviewStatus("redirecting")}
            >
              ↗️ Redirect
            </button>
          </div>

          {/* STAGE & FRAME */}
          <div className="preview-viewport-stage">
            <div className={`device-frame-${viewport}`}>
              <PaymentPageRenderer
                session={preview}
                remainingSeconds={previewStatus === "expired" ? 0 : 842}
                statusLabel={
                  previewStatus === "verifying"
                    ? "Memverifikasi Pembayaran"
                    : previewStatus === "redirecting" || previewStatus === "paid"
                      ? config.success_copy?.title ?? "Pembayaran Berhasil"
                      : previewStatus === "failed"
                        ? "Pembayaran Gagal"
                        : previewStatus === "expired"
                          ? "Pembayaran Kedaluwarsa"
                          : previewStatus === "cancelled"
                            ? "Pembayaran Dibatalkan"
                            : "Menunggu Pembayaran"
                }
                connected
                cancelling={false}
                onCancel={() => setPreviewStatus("cancelled")}
                onRedirect={() => setPreviewStatus("paid")}
              />
            </div>
          </div>
        </section>
      </div>

      {/* 3. THEME MANAGER MODAL DRAWER */}
      {showThemeModal && (
        <div className="theme-modal-backdrop" onClick={() => setShowThemeModal(false)}>
          <div className="theme-modal-panel" onClick={(e) => e.stopPropagation()}>
            <div className="theme-modal-header">
              <div>
                <h2>Daftar Tema Pembayaran Tersimpan</h2>
                <p style={{ margin: "2px 0 0", fontSize: "12px", color: "var(--muted)" }}>
                  Kelola, switch, jadikan default, dan duplikasi tema checkout Anda ({themes.length} tema)
                </p>
              </div>
              <button
                type="button"
                className="icon-button"
                onClick={() => setShowThemeModal(false)}
                aria-label="Tutup modal"
              >
                <X size={18} />
              </button>
            </div>

            <div className="theme-modal-body">
              <div className="theme-modal-search-bar">
                <input
                  type="text"
                  className="theme-search-input"
                  placeholder="Cari tema pembayaran tersimpan..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                />
                <button
                  type="button"
                  className="button button-primary"
                  style={{ minHeight: "38px", padding: "0 14px", fontSize: "12px", whiteSpace: "nowrap" }}
                  onClick={handleNewTheme}
                >
                  <Plus size={14} /> Buat Tema Baru
                </button>
              </div>

              <div className="theme-filter-tabs">
                {(["ALL", "PUBLISHED", "DRAFT", "ARCHIVED"] as const).map((filterKey) => (
                  <button
                    key={filterKey}
                    type="button"
                    className={`theme-filter-btn ${statusFilter === filterKey ? "active" : ""}`}
                    onClick={() => setStatusFilter(filterKey)}
                  >
                    {filterKey === "ALL"
                      ? `Semua (${themes.length})`
                      : filterKey === "PUBLISHED"
                        ? `Published (${themes.filter((t) => t.status === "PUBLISHED").length})`
                        : filterKey === "DRAFT"
                          ? `Draft (${themes.filter((t) => t.status === "DRAFT").length})`
                          : `Archived (${themes.filter((t) => t.status === "ARCHIVED").length})`}
                  </button>
                ))}
              </div>

              <div className="theme-modal-grid">
                {filteredThemes.map((theme) => {
                  const isCurrent = selected?.id === theme.id;
                  const themePalette = theme.config?.colors ?? modernModes.light.colors;
                  const isDarkTheme = themePalette.background === "#0F1715";

                  return (
                    <div
                      key={theme.id}
                      className={`theme-item-card-rich ${isCurrent ? "selected" : ""}`}
                    >
                      <div className="theme-card-rich-top">
                        <div className="theme-card-rich-swatch">
                          <span style={{ backgroundColor: themePalette.primary }} title="Primary" />
                          <span style={{ backgroundColor: themePalette.background }} title="Background" />
                          <span style={{ backgroundColor: themePalette.surface }} title="Surface" />
                        </div>
                        <div className="theme-card-rich-badges">
                          <span className={`studio-badge studio-badge-${theme.status.toLowerCase()}`}>
                            {theme.status}
                          </span>
                          {theme.is_default && (
                            <span className="studio-badge studio-badge-default">
                              <Star size={11} fill="currentColor" /> Default
                            </span>
                          )}
                        </div>
                      </div>

                      <div className="theme-card-rich-info">
                        <strong>{theme.name}</strong>
                        <span>Preset: Modern Teal ({isDarkTheme ? "Dark Mode" : "Light Mode"}) · v{theme.version || 1}</span>
                      </div>

                      <div className="theme-card-rich-actions">
                        <button
                          type="button"
                          className="button button-primary"
                          style={{ minHeight: "32px", padding: "0 12px", fontSize: "12px" }}
                          onClick={() => {
                            selectTheme(theme);
                            setShowThemeModal(false);
                            showToast(`Tema "${theme.name}" dimuat ke studio`, "info");
                          }}
                        >
                          {isCurrent ? "Sedang Diedit" : "Buka & Edit"}
                        </button>

                        {!theme.is_default && (
                          <button
                            type="button"
                            className="button"
                            style={{ minHeight: "32px", padding: "0 10px", fontSize: "12px" }}
                            onClick={() => void setDefaultTheme(theme.id)}
                            title="Jadikan tema default untuk seluruh checkout"
                          >
                            <Star size={12} /> Set Default
                          </button>
                        )}

                        <button
                          type="button"
                          className="button"
                          style={{ minHeight: "32px", padding: "0 8px" }}
                          onClick={() => void duplicateTheme(theme.id)}
                          title="Duplikasi tema"
                        >
                          <Copy size={13} />
                        </button>

                        {!theme.is_default && (
                          <button
                            type="button"
                            className="button"
                            style={{ color: "var(--red)", minHeight: "32px", padding: "0 8px" }}
                            onClick={() => void deleteTheme(theme.id)}
                            title="Hapus tema"
                          >
                            <Trash2 size={13} />
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}

                {filteredThemes.length === 0 && (
                  <div className="theme-empty-box" style={{ gridColumn: "1 / -1" }}>
                    Tidak ada tema yang sesuai dengan filter atau pencarian Anda.
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

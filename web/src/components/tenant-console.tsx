"use client";

import React, { useState } from "react";
import { BookOpen, Check, Copy, Eye, EyeOff, KeyRound, Monitor, Pencil, Plus, RefreshCw, Server, X } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { formatDate } from "@/lib/format";
import type { MerchantID, Tenant } from "@/lib/types";

type TenantAPI = { id: string; merchant_id?: string; name: string; site_url?: string; callback_url?: string; webhook_url?: string; active: boolean; api_key_recoverable?: boolean; created_at: string };
type CreatedTenant = { tenant: TenantAPI; api_key: string; api_key_visible_once: boolean };
type Modal = { kind: "create" } | { kind: "edit" | "docs"; tenant: Tenant } | null;

function fromAPI(item: TenantAPI): Tenant {
  return { id: item.id, name: item.name, merchantId: item.merchant_id ?? "", siteUrl: item.site_url ?? "", callbackUrl: item.callback_url ?? "", webhookUrl: item.webhook_url ?? "", active: item.active, apiKeyRecoverable: item.api_key_recoverable, createdAt: item.created_at };
}

export function TenantConsole({ initialTenants, merchants }: { initialTenants: Tenant[]; merchants: MerchantID[] }) {
  const [tenants, setTenants] = useState(initialTenants);
  const [modal, setModal] = useState<Modal>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<CreatedTenant | null>(null);
  const [copied, setCopied] = useState<"id" | "key" | "">("");

  function close() { setModal(null); setCreated(null); setError(""); setCopied(""); }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    const input: Record<string, string | boolean> = Object.fromEntries(["name", "merchant_id", "site_url", "callback_url", "webhook_url"].map((key) => [key, String(form.get(key) ?? "").trim()]));
    const editing = modal?.kind === "edit" ? modal.tenant : null;
    if (editing) input.active = form.get("active") === "on";
    try {
      const response = await fetch(editing ? `/api/admin/tenants/${encodeURIComponent(editing.id)}` : "/api/admin/tenants", { method: editing ? "PUT" : "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.error ?? `Tenant gagal disimpan (${response.status}).`);
      if (editing) {
        const updated = fromAPI(payload as TenantAPI);
        setTenants((current) => current.map((item) => item.id === updated.id ? updated : item));
        close();
      } else {
        const result = payload as CreatedTenant;
        setCreated(result);
        setTenants((current) => [fromAPI(result.tenant), ...current]);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Tenant gagal disimpan.");
    } finally { setBusy(false); }
  }

  async function copy(value: string, field: "id" | "key") {
    await navigator.clipboard.writeText(value); setCopied(field); window.setTimeout(() => setCopied(""), 1500);
  }

  const selected = modal?.kind === "edit" || modal?.kind === "docs" ? modal.tenant : null;
  return <div className="page">
    <PageHeader eyebrow="Merchant / API access" title="Tenant ID" description="Kelola identitas aplikasi yang memakai API QRIS dan hubungkan setiap tenant ke browser Merchant untuk rekonsiliasi transaksi." actions={<button className="button button-primary" onClick={() => setModal({ kind: "create" })}><Plus size={17} />Tambah tenant</button>} />
    <section className="section-block">
      <div className="section-heading"><div><span className="section-kicker">API directory</span><h2>Connected Tenant IDs</h2><p>{tenants.length} tenant terdaftar. Super admin dapat melihat kembali atau merotasi API key dari tombol edit.</p></div></div>
      <div className="table-scroll"><table><thead><tr><th>Tenant</th><th>Merchant connection</th><th>Site tujuan</th><th>Callback & webhook</th><th>Dibuat</th><th>Status</th><th>Action</th></tr></thead><tbody>
        {tenants.map((tenant) => <tr key={tenant.id}><td><strong>{tenant.name}</strong><span className="cell-subtitle"><code>{tenant.id}</code></span></td><td><code>{tenant.merchantId || "Belum terhubung"}</code></td><td>{tenant.siteUrl ? <a className="table-link" href={tenant.siteUrl} target="_blank" rel="noreferrer">{new URL(tenant.siteUrl).host}</a> : "Belum diatur"}</td><td><strong>{tenant.callbackUrl ? "Callback aktif" : "Tanpa callback"}</strong><span className="cell-subtitle">{tenant.webhookUrl ? "Webhook aktif" : "Webhook belum diatur"}</span></td><td>{formatDate(tenant.createdAt)}</td><td><StatusBadge status={tenant.active ? "active" : "inactive"} /></td><td><div className="row-actions"><button className="icon-button table-action" title="Edit tenant" aria-label={`Edit ${tenant.name}`} onClick={() => setModal({ kind: "edit", tenant })}><Pencil size={16} /></button><button className="icon-button table-action" title="Dokumentasi API" aria-label={`Dokumentasi ${tenant.name}`} onClick={() => setModal({ kind: "docs", tenant })}><BookOpen size={16} /></button></div></td></tr>)}
      </tbody></table></div>
    </section>
    {modal && <div className="tenant-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}><section className={`tenant-modal ${modal.kind === "docs" ? "tenant-docs-modal" : ""}`} role="dialog" aria-modal="true" aria-labelledby="tenant-dialog-title">
      <header><div><span className="section-kicker">{modal.kind === "docs" ? "Tenant API documentation" : "Tenant API access"}</span><h2 id="tenant-dialog-title">{created ? "Tenant siap digunakan" : modal.kind === "edit" ? `Edit ${selected?.name}` : modal.kind === "docs" ? selected?.name : "Tambah tenant"}</h2></div><button className="icon-button" aria-label="Tutup" onClick={close}><X size={18} /></button></header>
      {modal.kind === "docs" && selected ? <TenantDocumentation tenant={selected} /> : created ? <div className="tenant-credential-result">
        <TenantAccessNotice />
        <p>API key siap dipakai. Super admin dapat melihatnya kembali atau merotasinya dari tombol edit tenant.</p>
        <label>Tenant ID<div className="credential-field"><code>{created.tenant.id}</code><button type="button" className="icon-button" title="Salin Tenant ID" onClick={() => copy(created.tenant.id, "id")}>{copied === "id" ? <Check size={16} /> : <Copy size={16} />}</button></div></label>
        <label>API key<div className="credential-field"><code>{created.api_key}</code><button type="button" className="icon-button" title="Salin API key" onClick={() => copy(created.api_key, "key")}>{copied === "key" ? <Check size={16} /> : <Copy size={16} />}</button></div></label>
        <div className="tenant-api-routes"><KeyRound size={17} /><div><strong>Endpoint rekonsiliasi</strong><code>POST /v1/tenants/{created.tenant.id}/transactions/refresh</code><code>GET /v1/tenants/{created.tenant.id}/transactions</code></div></div>
        <button className="button button-primary button-wide" onClick={close}>Selesai</button>
      </div> : <TenantForm tenant={modal.kind === "edit" ? selected : null} merchants={merchants} busy={busy} error={error} onSubmit={submit} onCancel={close} onCopy={copy} copied={copied} onCredentialRecovered={(tenantID) => setTenants((current) => current.map((item) => item.id === tenantID ? { ...item, apiKeyRecoverable: true } : item))} />}
    </section></div>}
  </div>;
}

function TenantForm({ tenant, merchants, busy, error, onSubmit, onCancel, onCopy, copied, onCredentialRecovered }: { tenant: Tenant | null; merchants: MerchantID[]; busy: boolean; error: string; onSubmit: (event: React.FormEvent<HTMLFormElement>) => void; onCancel: () => void; onCopy: (value: string, field: "id" | "key") => Promise<void>; copied: "id" | "key" | ""; onCredentialRecovered: (tenantID: string) => void }) {
  return <form className="tenant-form" onSubmit={onSubmit}>
    <TenantAccessNotice />
    {tenant && <TenantCredentials tenant={tenant} onCopy={onCopy} copied={copied} onRecovered={onCredentialRecovered} />}
    <label>Nama tenant<input name="name" required maxLength={120} placeholder="Website utama" defaultValue={tenant?.name ?? ""} /></label>
    <label>Merchant connection<select name="merchant_id" required defaultValue={tenant?.merchantId || merchants[0]?.id || ""}><option value="" disabled>Pilih Merchant ID</option>{merchants.filter((merchant) => merchant.active).map((merchant) => <option key={merchant.id} value={merchant.id}>{merchant.name} ({merchant.id})</option>)}</select></label>
    <label>Site tujuan<input name="site_url" type="url" placeholder="https://app.example.com" defaultValue={tenant?.siteUrl ?? ""} /></label>
    <label>Callback URL<input name="callback_url" type="url" placeholder="https://app.example.com/qris/callback" defaultValue={tenant?.callbackUrl ?? ""} /></label>
    <label>Webhook URL<input name="webhook_url" type="url" placeholder="https://app.example.com/webhooks/qris" defaultValue={tenant?.webhookUrl ?? ""} /></label>
    {tenant && <label className="tenant-active-toggle"><input name="active" type="checkbox" defaultChecked={tenant.active} />Tenant dapat menggunakan API</label>}
    {error && <p className="form-error">{error}</p>}
    <div className="tenant-form-actions"><button type="button" className="button" onClick={onCancel}>Batal</button><button className="button button-primary" disabled={busy || merchants.length === 0}>{busy ? "Menyimpan..." : tenant ? "Simpan perubahan" : "Buat tenant & API key"}</button></div>
  </form>;
}

function TenantCredentials({ tenant, onCopy, copied, onRecovered }: { tenant: Tenant; onCopy: (value: string, field: "id" | "key") => Promise<void>; copied: "id" | "key" | ""; onRecovered: (tenantID: string) => void }) {
  const [apiKey, setAPIKey] = useState("");
  const [hidden, setHidden] = useState(true);
  const [rotationRequired, setRotationRequired] = useState(tenant.apiKeyRecoverable === false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function reveal() {
    if (apiKey) { setHidden((value) => !value); return; }
    setBusy(true); setError("");
    try {
      const response = await fetch(`/api/admin/tenants/${encodeURIComponent(tenant.id)}/credentials`, { method: "GET", cache: "no-store" });
      const payload = await response.json().catch(() => ({}));
      if (response.status === 409 || payload.code === "api_key_rotation_required") { setRotationRequired(true); return; }
      if (!response.ok || typeof payload.api_key !== "string") throw new Error(payload.error ?? `API key gagal dibaca (${response.status}).`);
      setAPIKey(payload.api_key); setHidden(false); setRotationRequired(false);
      onRecovered(tenant.id);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "API key gagal dibaca."); }
    finally { setBusy(false); }
  }

  async function rotate() {
    if (!window.confirm("Rotasi API key akan langsung menonaktifkan key lama. Lanjutkan?")) return;
    setBusy(true); setError("");
    try {
      const response = await fetch(`/api/admin/tenants/${encodeURIComponent(tenant.id)}/credentials/rotate`, { method: "POST" });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok || typeof payload.api_key !== "string") throw new Error(payload.error ?? `API key gagal dirotasi (${response.status}).`);
      setAPIKey(payload.api_key); setHidden(false); setRotationRequired(false);
      onRecovered(tenant.id);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "API key gagal dirotasi."); }
    finally { setBusy(false); }
  }

  return <section className="tenant-credentials" aria-label="Kredensial Tenant API">
    <label>Tenant ID<div className="credential-field"><code>{tenant.id}</code><button type="button" className="icon-button" aria-label="Salin Tenant ID" title="Salin Tenant ID" onClick={() => onCopy(tenant.id, "id")}>{copied === "id" ? <Check size={16} /> : <Copy size={16} />}</button></div></label>
    <label>API key<div className="credential-field credential-field-actions"><code>{hidden || !apiKey ? "xl_live_••••••••••••••••" : apiKey}</code><span>
      <button type="button" className="icon-button" aria-label={apiKey && !hidden ? "Sembunyikan API key" : "Lihat API key"} title={apiKey && !hidden ? "Sembunyikan API key" : "Lihat API key"} disabled={busy} onClick={reveal}>{apiKey && !hidden ? <EyeOff size={16} /> : <Eye size={16} />}</button>
      <button type="button" className="icon-button" aria-label="Salin API key" title="Salin API key" disabled={!apiKey || hidden} onClick={() => onCopy(apiKey, "key")}>{copied === "key" ? <Check size={16} /> : <Copy size={16} />}</button>
    </span></div></label>
    {rotationRequired && <div className="tenant-key-rotation"><p>Key lama hanya tersimpan sebagai hash dan perlu dirotasi satu kali agar dapat dilihat kembali.</p><button type="button" className="button" aria-label="Rotasi API key" disabled={busy} onClick={rotate}><RefreshCw size={15} />{busy ? "Merotasi..." : "Rotasi API key"}</button></div>}
    {error && <p className="form-error">{error}</p>}
  </section>;
}

function TenantAccessNotice() {
  return <aside className="tenant-access-notice" aria-label="Pemisahan jalur akses tenant">
    <div><Server size={17} /><p><strong>API tenant</strong><span>Integrasi aplikasi memakai <code>https://api.alpakyros.net</code>. Akses lokal server dipublikasikan pada <code>127.0.0.1:1970</code>.</span></p></div>
    <div><Monitor size={17} /><p><strong>Web admin & browser</strong><span>Console memakai <code>https://pay.alpakyros.net</code> atau <code>127.0.0.1:5656</code>. Port Docker internal tidak dipakai tenant.</span></p></div>
  </aside>;
}

function TenantDocumentation({ tenant }: { tenant: Tenant }) {
  const id = tenant.id;
  const apiBase = "https://api.alpakyros.net";
  const adminBase = "https://pay.alpakyros.net";
  return <div className="tenant-docs">
    <div className="tenant-domain-map" aria-label="Peta arah domain payment gateway">
      <section><span>DOMAIN 1</span><Monitor size={18} /><div><strong>Admin Web</strong><code>{adminBase}</code><p>Hanya untuk login operator dan dashboard. Origin lokal yang dipublikasikan adalah <code>127.0.0.1:5656</code>; jangan gunakan domain ini dari aplikasi tenant.</p></div></section>
      <section><span>DOMAIN 2</span><Server size={18} /><div><strong>Tenant API</strong><code>{apiBase}</code><p>Semua request tenant ke <code>/v1/*</code> memakai domain ini. Origin lokal yang dipublikasikan adalah <code>127.0.0.1:1970</code>.</p></div></section>
      <div className="tenant-domain-flow"><strong>Alur yang benar</strong><code>Aplikasi tenant → {apiBase}/v1/* → Backend API</code><p>Browser worker tidak memiliki domain public dan tidak boleh dipanggil langsung oleh tenant.</p></div>
    </div>
    <div className="tenant-api-routes"><KeyRound size={17} /><div><strong>Autentikasi Alpakyros LITE</strong><code>Tenant ID: {id}</code><code>X-API-Key: YOUR_API_KEY</code><p>Kirim key hanya ke <code>{apiBase}</code>. Portal merchant dan browser worker tetap berada di jaringan internal gateway.</p></div></div>
    <p>Gunakan header <code>X-API-Key: YOUR_API_KEY</code> pada setiap request. Tenant ID aktif untuk integrasi ini adalah <code>{id}</code>; API key dapat dilihat atau dirotasi oleh super admin dari tombol edit tenant.</p>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Buat transaksi QRIS dinamis</strong><code>{apiBase}/v1/tenants/{id}/transactions/qris</code><pre>{`curl -X POST '${apiBase}/v1/tenants/${id}/transactions/qris' \\\n  -H 'Content-Type: application/json' \\\n  -H 'X-API-Key: YOUR_API_KEY' \\\n  -d '{"template_id":"QRIS_TEMPLATE_ID","amount":50000,"idempotency_key":"ORDER_UNIQUE_ID"}'`}</pre><p>Respons menyertakan ID transaksi, payload QRIS, PNG base64, URL status, URL QR, status pending, dan waktu kedaluwarsa. Gunakan idempotency key unik per order.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Status transaksi QRIS</strong><code>GET /v1/tenants/{id}/transactions/qris/{`{transaction_id}`}</code><code>{apiBase}/v1/tenants/{id}/transactions/qris/{`{transaction_id}`}</code><p>Polling endpoint ini untuk status <code>pending</code>, <code>paid</code>, atau <code>expired</code>.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Gambar QR transaksi</strong><code>{apiBase}/v1/tenants/{id}/transactions/qris/{`{transaction_id}`}/qr</code><p>Mengembalikan PNG selama transaksi masih dapat dibayar.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Alias kompatibilitas QRIS</strong><code>{apiBase}/v1/tenants/{id}/qris/dynamic</code><p>Alias kompatibel untuk pembuatan transaksi. Integrasi baru disarankan memakai route <code>/transactions/qris</code>.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Refresh history browser</strong><code>{apiBase}/v1/tenants/{id}/transactions/refresh</code><p>Request masuk melalui API, lalu backend mengantrekan worker browser internal untuk mengambil mutasi terbaru.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Hasil transaksi tenant</strong><code>{apiBase}/v1/tenants/{id}/transactions?limit=100</code><p>Hanya mengembalikan transaksi portal yang berhasil dihubungkan ke invoice tenant ini.</p></div>
  </div>;
}

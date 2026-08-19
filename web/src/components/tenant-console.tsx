"use client";

import React, { useState } from "react";
import { BookOpen, Check, Copy, Eye, EyeOff, KeyRound, Monitor, Pencil, Plus, RefreshCw, Server, Trash2, X } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { formatDate } from "@/lib/format";
import type { MerchantID, QRISTemplate, Tenant } from "@/lib/types";

type TenantAPI = { id: string; merchant_id?: string; name: string; site_url?: string; callback_url?: string; webhook_url?: string; sandbox_mode: boolean; use_unique_amount_code?: boolean; unique_amount_cooldown_minutes?: number; active: boolean; api_key_recoverable?: boolean; webhook_secret_configured?: boolean; created_at: string };
type CreatedTenant = { tenant: TenantAPI; api_key: string; api_key_visible_once: boolean };
type Modal = { kind: "create" } | { kind: "edit" | "docs"; tenant: Tenant } | null;

function fromAPI(item: TenantAPI): Tenant {
  return { id: item.id, name: item.name, merchantId: item.merchant_id ?? "", siteUrl: item.site_url ?? "", callbackUrl: item.callback_url ?? "", webhookUrl: item.webhook_url ?? "", sandboxMode: item.sandbox_mode, useUniqueAmountCode: item.use_unique_amount_code ?? false, uniqueAmountCooldownMinutes: item.unique_amount_cooldown_minutes ?? 30, active: item.active, apiKeyRecoverable: item.api_key_recoverable, webhookSecretConfigured: item.webhook_secret_configured, createdAt: item.created_at };
}

function tenantSiteOrigin(siteURL: string): string {
  try { return new URL(siteURL).origin; } catch { return "Site tujuan"; }
}

function qrisTemplateScope(template: QRISTemplate): "all_tenants" | "selected_tenant" {
  return template.access_scope || (template.tenant_id ? "selected_tenant" : "all_tenants");
}

function tenantCanUseQRSTemplate(template: QRISTemplate, tenantID: string): boolean {
  const scope = qrisTemplateScope(template);
  return template.active && template.static_to_dynamic && (scope === "all_tenants" || (scope === "selected_tenant" && template.tenant_id === tenantID));
}

export function TenantConsole({ initialTenants, merchants, qrisTemplates = [] }: { initialTenants: Tenant[]; merchants: MerchantID[]; qrisTemplates?: QRISTemplate[] }) {
  const [tenants, setTenants] = useState(initialTenants);
  const [modal, setModal] = useState<Modal>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<CreatedTenant | null>(null);
  const [copied, setCopied] = useState<"id" | "key" | "">("");
  const [deleting, setDeleting] = useState("");
  const [deleteError, setDeleteError] = useState("");

  function close() { setModal(null); setCreated(null); setError(""); setCopied(""); }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    const input: Record<string, string | boolean | number> = Object.fromEntries(["name", "merchant_id", "site_url", "callback_url", "webhook_url"].map((key) => [key, String(form.get(key) ?? "").trim()]));
    const editing = modal?.kind === "edit" ? modal.tenant : null;
    if (editing) {
      input.active = form.get("active") === "on";
      input.sandbox_mode = form.get("sandbox_mode") === "on";
      input.use_unique_amount_code = form.get("use_unique_amount_code") === "on";
      input.unique_amount_cooldown_minutes = Number(form.get("unique_amount_cooldown_minutes") ?? 30);
    }
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

  async function removeTenant(tenant: Tenant) {
    if (!window.confirm(`Hapus tenant ${tenant.name}? Tenant akan hilang dari daftar dan API key langsung dinonaktifkan. Histori transaksi tetap tersimpan.`)) return;
    setDeleting(tenant.id); setDeleteError("");
    try {
      const response = await fetch(`/api/admin/tenants/${encodeURIComponent(tenant.id)}`, { method: "DELETE" });
      if (!response.ok) {
        const payload = await response.json().catch(() => ({}));
        throw new Error(payload.error ?? `Tenant gagal dihapus (${response.status}).`);
      }
      setTenants((current) => current.filter((item) => item.id !== tenant.id));
      if (selected?.id === tenant.id) close();
    } catch (cause) {
      setDeleteError(cause instanceof Error ? cause.message : "Tenant gagal dihapus.");
    } finally { setDeleting(""); }
  }

  const selected = modal?.kind === "edit" || modal?.kind === "docs" ? modal.tenant : null;
  return <div className="page">
    <PageHeader eyebrow="Merchant / API access" title="Tenant ID" description="Kelola identitas aplikasi yang memakai API QRIS dan hubungkan setiap tenant ke browser Merchant untuk rekonsiliasi transaksi." actions={<button className="button button-primary" onClick={() => setModal({ kind: "create" })}><Plus size={17} />Tambah tenant</button>} />
    <section className="section-block">
      <div className="section-heading"><div><span className="section-kicker">API directory</span><h2>Connected Tenant IDs</h2><p>{tenants.length} tenant terdaftar. Super admin dapat melihat kembali, merotasi API key, atau menghapus tenant.</p>{deleteError && <p className="form-error" role="alert">{deleteError}</p>}</div></div>
      <div className="table-scroll"><table><thead><tr><th>Tenant</th><th>Merchant connection</th><th>Site tujuan</th><th>Callback & webhook</th><th>Dibuat</th><th>Status</th><th>Action</th></tr></thead><tbody>
        {tenants.map((tenant) => <tr key={tenant.id}><td><strong>{tenant.name}</strong><span className="cell-subtitle"><code>{tenant.id}</code></span></td><td><code>{tenant.merchantId || "Belum terhubung"}</code></td><td>{tenant.siteUrl ? <a className="table-link" href={tenant.siteUrl} target="_blank" rel="noreferrer">{new URL(tenant.siteUrl).host}</a> : "Belum diatur"}</td><td><strong>{tenant.callbackUrl ? "Callback tersimpan" : "Tanpa callback"}</strong><span className="cell-subtitle">{tenant.webhookUrl ? "Webhook tersimpan" : "Webhook belum diatur"}</span></td><td>{formatDate(tenant.createdAt)}</td><td><StatusBadge status={tenant.active ? "active" : "inactive"} /><span className="cell-subtitle">{tenant.sandboxMode ? "Sandbox" : "Production"}</span></td><td><div className="row-actions"><button className="icon-button table-action" title="Edit tenant" aria-label={`Edit ${tenant.name}`} onClick={() => setModal({ kind: "edit", tenant })}><Pencil size={16} /></button><button className="icon-button table-action" title="Dokumentasi API" aria-label={`Dokumentasi ${tenant.name}`} onClick={() => setModal({ kind: "docs", tenant })}><BookOpen size={16} /></button><button className="icon-button table-action tenant-delete-action" title="Hapus tenant" aria-label={`Hapus ${tenant.name}`} disabled={deleting === tenant.id} onClick={() => removeTenant(tenant)}><Trash2 size={16} /></button></div></td></tr>)}
      </tbody></table></div>
    </section>
    {modal && <div className="tenant-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}><section className={`tenant-modal ${modal.kind === "docs" ? "tenant-docs-modal" : ""}`} role="dialog" aria-modal="true" aria-labelledby="tenant-dialog-title">
      <header><div><span className="section-kicker">{modal.kind === "docs" ? "Tenant API documentation" : "Tenant API access"}</span><h2 id="tenant-dialog-title">{created ? "Tenant siap digunakan" : modal.kind === "edit" ? `Edit ${selected?.name}` : modal.kind === "docs" ? selected?.name : "Tambah tenant"}</h2></div><button className="icon-button" aria-label="Tutup" onClick={close}><X size={18} /></button></header>
      {modal.kind === "docs" && selected ? <TenantDocumentation tenant={selected} qrisTemplates={qrisTemplates} /> : created ? <div className="tenant-credential-result">
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
    {tenant && <label className="tenant-active-toggle"><input name="sandbox_mode" type="checkbox" aria-label="Sandbox mode" aria-describedby="tenant-sandbox-description" defaultChecked={tenant.sandboxMode} />Sandbox mode<span id="tenant-sandbox-description" className="cell-subtitle">Izinkan request browser dari origin mana pun. API key tetap wajib.</span></label>}
    {tenant && <label className="tenant-active-toggle"><input name="use_unique_amount_code" type="checkbox" aria-label="Gunakan kode unik nominal" aria-describedby="tenant-unique-amount-description" defaultChecked={tenant.useUniqueAmountCode ?? false} />Gunakan kode unik nominal<span id="tenant-unique-amount-description" className="cell-subtitle">Tambahkan kode unik ke nominal bayar agar transaksi lebih mudah dicocokkan.</span></label>}
    {tenant && <label>Cooldown kode unik (menit)<input name="unique_amount_cooldown_minutes" type="number" min={30} max={60} step={1} defaultValue={tenant.uniqueAmountCooldownMinutes ?? 30} aria-describedby="tenant-cooldown-description" /><span id="tenant-cooldown-description" className="cell-subtitle">Kode dari transaksi Paid atau Expired dikunci 30-60 menit sebelum dapat dipakai lagi.</span></label>}
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
  const [webhookSecret, setWebhookSecret] = useState("");
  const [webhookConfigured, setWebhookConfigured] = useState(tenant.webhookSecretConfigured === true);

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

  async function rotateWebhookSecret() {
    if (!window.confirm("Rotasi webhook secret akan langsung membatalkan secret lama. Simpan secret baru sebelum menutup dialog. Lanjutkan?")) return;
    setBusy(true); setError(""); setWebhookSecret("");
    try {
      const response = await fetch(`/api/admin/tenants/${encodeURIComponent(tenant.id)}/webhook-secret/rotate`, { method: "POST" });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok || typeof payload.webhook_secret !== "string") throw new Error(payload.error ?? `Webhook secret gagal dibuat (${response.status}).`);
      setWebhookSecret(payload.webhook_secret);
      setWebhookConfigured(true);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Webhook secret gagal dibuat."); }
    finally { setBusy(false); }
  }

  return <section className="tenant-credentials" aria-label="Kredensial Tenant API">
    <label>Tenant ID<div className="credential-field"><code>{tenant.id}</code><button type="button" className="icon-button" aria-label="Salin Tenant ID" title="Salin Tenant ID" onClick={() => onCopy(tenant.id, "id")}>{copied === "id" ? <Check size={16} /> : <Copy size={16} />}</button></div></label>
    <label>API key<div className="credential-field credential-field-actions"><code>{hidden || !apiKey ? "xl_live_••••••••••••••••" : apiKey}</code><span>
      <button type="button" className="icon-button" aria-label={apiKey && !hidden ? "Sembunyikan API key" : "Lihat API key"} title={apiKey && !hidden ? "Sembunyikan API key" : "Lihat API key"} disabled={busy} onClick={reveal}>{apiKey && !hidden ? <EyeOff size={16} /> : <Eye size={16} />}</button>
      <button type="button" className="icon-button" aria-label="Salin API key" title="Salin API key" disabled={!apiKey || hidden} onClick={() => onCopy(apiKey, "key")}>{copied === "key" ? <Check size={16} /> : <Copy size={16} />}</button>
    </span></div></label>
    <label>Webhook secret<div className="credential-field credential-field-actions"><code>{webhookSecret || "whsec_••••••••••••••••"}</code><span>
      {webhookSecret && <button type="button" className="icon-button" aria-label="Salin webhook secret" title="Salin webhook secret" onClick={() => navigator.clipboard.writeText(webhookSecret)}><Copy size={16} /></button>}
      <button type="button" className="button button-compact" aria-label={webhookConfigured ? "Rotasi webhook secret" : "Buat webhook secret"} disabled={busy} onClick={rotateWebhookSecret}><RefreshCw size={15} />{busy ? "Memproses..." : webhookConfigured ? "Rotasi" : "Buat secret"}</button>
    </span></div><span className="cell-subtitle">{webhookSecret ? "Tampilkan sekali. Simpan di aplikasi tenant sebelum menutup dialog." : webhookConfigured ? "Secret sudah tersimpan terenkripsi; nilai lama tidak dapat dibaca kembali." : "Belum dikonfigurasi. Webhook belum dapat diverifikasi."}</span></label>
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

function TenantDocumentation({ tenant, qrisTemplates }: { tenant: Tenant; qrisTemplates: QRISTemplate[] }) {
  const id = tenant.id;
  const apiBase = "https://api.alpakyros.net";
  const adminBase = "https://pay.alpakyros.net";
  const availableTemplates = qrisTemplates.filter((template) => tenantCanUseQRSTemplate(template, id));
  const exampleTemplateID = availableTemplates[0]?.id ?? "TEMPLATE_ID_DARI_ENDPOINT_DI_ATAS";
  return <div className="tenant-docs">
    <div className="tenant-domain-map" aria-label="Peta arah domain payment gateway">
      <section><span>DOMAIN 1</span><Monitor size={18} /><div><strong>Admin Web</strong><code>{adminBase}</code><p>Hanya untuk login operator dan dashboard. Origin lokal yang dipublikasikan adalah <code>127.0.0.1:5656</code>; jangan gunakan domain ini dari aplikasi tenant.</p></div></section>
      <section><span>DOMAIN 2</span><Server size={18} /><div><strong>Tenant API</strong><code>{apiBase}</code><p>Semua request tenant ke <code>/v1/*</code> memakai domain ini. Origin lokal yang dipublikasikan adalah <code>127.0.0.1:1970</code>.</p></div></section>
      <div className="tenant-domain-flow"><strong>Alur yang benar</strong><code>Aplikasi tenant → {apiBase}/v1/* → Backend API</code><p>Browser worker tidak memiliki domain public dan tidak boleh dipanggil langsung oleh tenant.</p></div>
    </div>
    <div className="tenant-api-routes"><KeyRound size={17} /><div><strong>Autentikasi Alpakyros LITE</strong><code>Tenant ID: {id}</code><code>X-API-Key: YOUR_API_KEY</code><p>Kirim key hanya ke <code>{apiBase}</code>. Portal merchant dan browser worker tetap berada di jaringan internal gateway.</p></div></div>
    <p>Gunakan header <code>X-API-Key: YOUR_API_KEY</code> pada setiap request. Tenant ID aktif untuk integrasi ini adalah <code>{id}</code>; API key dapat dilihat atau dirotasi oleh super admin dari tombol edit tenant.</p>
    <div className="tenant-doc-endpoint"><strong>Webhook signature</strong><p>Webhook URL hanya aktif setelah super admin membuat webhook secret dari edit Tenant ID. Secret ditampilkan satu kali saat dibuat/dirotasi dan disimpan tenant secara aman. Gunakan secret untuk memverifikasi signature HMAC webhook; jangan menaruhnya di browser atau mengirimkannya kembali ke API.</p><code>X-Xloyal-Signature: sha256=...</code><p>Rotasi langsung membatalkan secret lama. Selama secret belum dikonfigurasi, gunakan polling status sebagai sumber kebenaran.</p></div>
    <div className="tenant-api-routes"><Server size={17} /><div><strong>Mode browser: {tenant.sandboxMode ? "Sandbox" : "Production"}</strong><p>{tenant.sandboxMode ? "Request browser boleh berasal dari origin mana pun, tetapi API key tetap wajib dan tetap diverifikasi." : <>Request browser hanya diterima dari origin <code>{tenantSiteOrigin(tenant.siteUrl)}</code>. Request server-to-server tanpa header Origin tetap diterima.</>}</p></div></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Daftar template QRIS tersedia</strong><code>{apiBase}/v1/tenants/{id}/qris/templates</code><pre>{`curl '${apiBase}/v1/tenants/${id}/qris/templates' \\\n  -H 'X-API-Key: YOUR_API_KEY'`}</pre><p>Ambil nilai <code>id</code> dari respons ini. Endpoint hanya mengembalikan template aktif yang boleh digunakan Tenant ID ini untuk QRIS dinamis.</p>{availableTemplates.length > 0 ? <ul className="tenant-doc-template-list">{availableTemplates.map((template) => <li key={template.id}><strong>{template.name}</strong><code>{template.id}</code><span>{qrisTemplateScope(template) === "all_tenants" ? "Shared" : "Tenant khusus"} · {template.merchant_name || "Merchant QRIS"}</span></li>)}</ul> : <p>Belum ada template QRIS aktif untuk tenant ini. Atur aksesnya dari QRIS Control.</p>}</div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Buat transaksi QRIS dinamis</strong><code>{apiBase}/v1/tenants/{id}/transactions/qris</code><pre>{`curl -X POST '${apiBase}/v1/tenants/${id}/transactions/qris' \\\n  -H 'Content-Type: application/json' \\\n  -H 'X-API-Key: YOUR_API_KEY' \\\n  -d '{"template_id":"${exampleTemplateID}","amount":50000,"idempotency_key":"ORDER_UNIQUE_ID"}'`}</pre><p>Respons menyertakan ID transaksi, payload QRIS, PNG base64, URL status, URL QR, status pending, dan waktu kedaluwarsa. Gunakan idempotency key unik per order.</p><div className="tenant-api-routes"><div><strong>Kode unik nominal: {tenant.useUniqueAmountCode ? "Aktif" : "Nonaktif"}</strong><code>requested_amount: Rp10.000 - nominal awal dari request</code><code>payable_amount: Rp10.037 - nominal yang harus dibayar</code><code>unique_amount_code: 37 - kode unik yang ditambahkan</code><p>Jika fitur aktif, request Rp10.000 dapat menghasilkan payable amount Rp10.037. Bayar tepat Rp10.037 dan gunakan <code>payable_amount</code> untuk tampilan serta instruksi pembayaran.</p></div></div><code>{`const imageSrc = "data:image/png;base64," + response.qr_png_base64;`}</code><p>Tampilkan <code>imageSrc</code> sebagai sumber gambar QR di frontend. Simpan API key dan pemanggilan gateway di backend aplikasi tenant, bukan di JavaScript browser.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Status transaksi QRIS</strong><code>GET /v1/tenants/{id}/transactions/qris/{`{transaction_id}`}</code><code>{apiBase}/v1/tenants/{id}/transactions/qris/{`{transaction_id}`}</code><p>Polling endpoint ini selama status <code>pending</code>. Saat pending, tunggu <code>poll_after_seconds</code> atau header <code>Retry-After</code> sebelum request berikutnya dan jadwalkan request setelah respons selesai. Hentikan polling pada <code>paid</code>, <code>expired</code>, <code>failed</code>, atau <code>cancelled</code>.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Batalkan transaksi QRIS</strong><code>{apiBase}/v1/tenants/{id}/transactions/qris/{`{transaction_id}`}/cancel</code><pre>{`curl -X POST '${apiBase}/v1/tenants/${id}/transactions/qris/TRANSACTION_ID/cancel' \
  -H 'X-API-Key: YOUR_API_KEY'`}</pre><p>Hentikan timer polling lokal sebelum mengirim cancel, tunggu respons server, lalu perbarui UI hanya dari resource transaksi yang dikembalikan. Status <code>pending</code> berubah menjadi <code>cancelled</code>; pemanggilan ulang pada transaksi cancelled tetap menghasilkan HTTP 200.</p><code>{`async function cancelPayment() {
  qrisController.stopPolling();
  const transaction = await qrisController.cancel();
  renderPaymentStatus(transaction.status);
}`}</code><p>Hubungkan fungsi ini ke tombol Cancel, Kembali, Close, atau batal pembayaran. Jangan mengubah state UI menjadi cancelled sebelum request berhasil. HTTP 409 berarti transaksi sudah terminal; gunakan status server sebagai source of truth. Jika jaringan gagal, tampilkan error dan jangan menganggap cancel berhasil.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Gambar QR transaksi</strong><code>{apiBase}/v1/tenants/{id}/transactions/qris/{`{transaction_id}`}/qr</code><p>Mengembalikan PNG selama transaksi masih dapat dibayar. QR transaksi cancelled mengembalikan <code>410 Gone</code>.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Alias kompatibilitas QRIS</strong><code>{apiBase}/v1/tenants/{id}/qris/dynamic</code><p>Alias kompatibel untuk pembuatan transaksi. Integrasi baru disarankan memakai route <code>/transactions/qris</code>.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Refresh history browser</strong><code>{apiBase}/v1/tenants/{id}/transactions/refresh</code><p>Request masuk melalui API, lalu backend mengantrekan worker browser internal untuk mengambil mutasi terbaru.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Hasil transaksi tenant</strong><code>{apiBase}/v1/tenants/{id}/transactions?limit=100</code><p>Hanya mengembalikan transaksi portal yang berhasil dihubungkan ke invoice tenant ini.</p></div>
    <div className="tenant-doc-endpoint">
      <strong>Alur integrasi wajib</strong>
      <ol className="tenant-doc-steps"><li>Ambil template QRIS aktif dari endpoint template.</li><li>Buat satu transaksi dengan nominal order dan idempotency key unik.</li><li>Tampilkan QR serta <code>payable_amount</code>, bukan hanya nominal request.</li><li>Polling URL status sesuai <code>poll_after_seconds</code> selama status masih <code>pending</code>.</li><li>Saat user membatalkan order, hentikan polling lokal lalu panggil endpoint cancel dan tunggu responsnya.</li><li>Finalisasi order client hanya setelah status API menjadi <code>paid</code>; perlakukan <code>expired</code>, <code>failed</code>, dan <code>cancelled</code> sebagai terminal.</li></ol>
      <p>Sumber status order saat ini adalah polling endpoint status. Nilai Callback URL dan Webhook URL tersimpan sebagai konfigurasi tenant; jangan menunggu callback atau webhook untuk menyelesaikan order.</p>
    </div>
    <div className="tenant-doc-endpoint">
      <strong>Kontrak request dan idempotency</strong>
      <code>Idempotency-Key: ORDER_UNIQUE_ID</code>
      <pre>{`curl -X POST '${apiBase}/v1/tenants/${id}/transactions/qris' \\
  -H 'Content-Type: application/json' \\
  -H 'X-API-Key: YOUR_API_KEY' \\
  -H 'Idempotency-Key: ORDER_UNIQUE_ID' \\
  -d '{"template_id":"${exampleTemplateID}","amount":50000,"expires_in_seconds":1800}'`}</pre>
      <p><code>amount</code> memakai satuan Rupiah tanpa pemisah. <code>expires_in_seconds</code> opsional dengan rentang 60-1800. Key yang sama dengan body yang sama mengembalikan transaksi yang sama; body berbeda menghasilkan <code>409</code>.</p>
      <pre>{`{
  "id": "TRANSACTION_ID",
  "status": "pending",
  "requested_amount": 50000,
  "payable_amount": ${tenant.useUniqueAmountCode ? 50037 : 50000},
  "unique_amount_code": ${tenant.useUniqueAmountCode ? 37 : 0},
  "status_url": "/v1/tenants/${id}/transactions/qris/TRANSACTION_ID",
  "qr_url": "/v1/tenants/${id}/transactions/qris/TRANSACTION_ID/qr",
  "poll_after_seconds": 15
}`}</pre>
    </div>
    <div className="tenant-doc-endpoint"><strong>Siklus status dan polling</strong><p>Tunggu <code>poll_after_seconds</code> atau header <code>Retry-After</code> sebelum request berikutnya. Jalankan satu polling berurutan per transaksi, bukan request paralel.</p><ul className="tenant-doc-status-list"><li><code>pending</code><span>Pembayaran belum cocok; lanjut polling.</span></li><li><code>paid</code><span>Status final berhasil; simpan ID transaksi dan hentikan polling.</span></li><li><code>expired</code><span>Status final tidak dibayar; hentikan polling dan buat transaksi baru bila diperlukan.</span></li><li><code>failed</code><span>Status final gagal; hentikan polling dan tampilkan kegagalan dari server.</span></li><li><code>cancelled</code><span>Status final dibatalkan tenant; hentikan polling dan jangan gunakan QR lama.</span></li></ul></div>
    <div className="tenant-doc-endpoint"><strong>Respons error penting</strong><ul className="tenant-doc-status-list"><li><code>400</code><span>Body, nominal, expiry, atau template ID tidak valid.</span></li><li><code>401</code><span>API key hilang atau tidak valid.</span></li><li><code>403</code><span>Origin browser tidak sesuai Site tujuan pada mode Production.</span></li><li><code>404</code><span>Template atau transaksi tidak tersedia untuk Tenant ID ini.</span></li><li><code>409</code><span>Idempotency key dipakai ulang dengan data berbeda, nominal sedang bentrok, atau cancel ditolak karena transaksi sudah paid, expired, atau failed.</span></li><li><code>410</code><span>QR tidak lagi tersedia karena transaksi expired atau cancelled.</span></li><li><code>429</code><span>Rate limit atau kode unik penuh; tunggu header <code>Retry-After</code>.</span></li></ul></div>
  </div>;
}

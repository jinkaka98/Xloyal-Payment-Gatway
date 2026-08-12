"use client";

import { useState } from "react";
import { BookOpen, Check, Copy, KeyRound, Monitor, Pencil, Plus, Server, X } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { formatDate } from "@/lib/format";
import type { MerchantID, Tenant } from "@/lib/types";

type TenantAPI = { id: string; merchant_id?: string; name: string; site_url?: string; callback_url?: string; webhook_url?: string; active: boolean; created_at: string };
type CreatedTenant = { tenant: TenantAPI; api_key: string; api_key_visible_once: boolean };
type Modal = { kind: "create" } | { kind: "edit" | "docs"; tenant: Tenant } | null;

function fromAPI(item: TenantAPI): Tenant {
  return { id: item.id, name: item.name, merchantId: item.merchant_id ?? "", siteUrl: item.site_url ?? "", callbackUrl: item.callback_url ?? "", webhookUrl: item.webhook_url ?? "", active: item.active, createdAt: item.created_at };
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
      <div className="section-heading"><div><span className="section-kicker">API directory</span><h2>Connected Tenant IDs</h2><p>{tenants.length} tenant terdaftar. API key hanya ditampilkan saat tenant dibuat.</p></div></div>
      <div className="table-scroll"><table><thead><tr><th>Tenant</th><th>Merchant connection</th><th>Site tujuan</th><th>Callback & webhook</th><th>Dibuat</th><th>Status</th><th>Action</th></tr></thead><tbody>
        {tenants.map((tenant) => <tr key={tenant.id}><td><strong>{tenant.name}</strong><span className="cell-subtitle"><code>{tenant.id}</code></span></td><td><code>{tenant.merchantId || "Belum terhubung"}</code></td><td>{tenant.siteUrl ? <a className="table-link" href={tenant.siteUrl} target="_blank" rel="noreferrer">{new URL(tenant.siteUrl).host}</a> : "Belum diatur"}</td><td><strong>{tenant.callbackUrl ? "Callback aktif" : "Tanpa callback"}</strong><span className="cell-subtitle">{tenant.webhookUrl ? "Webhook aktif" : "Webhook belum diatur"}</span></td><td>{formatDate(tenant.createdAt)}</td><td><StatusBadge status={tenant.active ? "active" : "inactive"} /></td><td><div className="row-actions"><button className="icon-button table-action" title="Edit tenant" aria-label={`Edit ${tenant.name}`} onClick={() => setModal({ kind: "edit", tenant })}><Pencil size={16} /></button><button className="icon-button table-action" title="Dokumentasi API" aria-label={`Dokumentasi ${tenant.name}`} onClick={() => setModal({ kind: "docs", tenant })}><BookOpen size={16} /></button></div></td></tr>)}
      </tbody></table></div>
    </section>
    {modal && <div className="tenant-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}><section className={`tenant-modal ${modal.kind === "docs" ? "tenant-docs-modal" : ""}`} role="dialog" aria-modal="true" aria-labelledby="tenant-dialog-title">
      <header><div><span className="section-kicker">{modal.kind === "docs" ? "Tenant API documentation" : "Tenant API access"}</span><h2 id="tenant-dialog-title">{created ? "Tenant siap digunakan" : modal.kind === "edit" ? `Edit ${selected?.name}` : modal.kind === "docs" ? selected?.name : "Tambah tenant"}</h2></div><button className="icon-button" aria-label="Tutup" onClick={close}><X size={18} /></button></header>
      {modal.kind === "docs" && selected ? <TenantDocumentation tenant={selected} /> : created ? <div className="tenant-credential-result">
        <TenantAccessNotice />
        <p>Simpan API key sekarang. Nilai ini tidak akan ditampilkan kembali.</p>
        <label>Tenant ID<div className="credential-field"><code>{created.tenant.id}</code><button type="button" className="icon-button" title="Salin Tenant ID" onClick={() => copy(created.tenant.id, "id")}>{copied === "id" ? <Check size={16} /> : <Copy size={16} />}</button></div></label>
        <label>API key<div className="credential-field"><code>{created.api_key}</code><button type="button" className="icon-button" title="Salin API key" onClick={() => copy(created.api_key, "key")}>{copied === "key" ? <Check size={16} /> : <Copy size={16} />}</button></div></label>
        <div className="tenant-api-routes"><KeyRound size={17} /><div><strong>Endpoint rekonsiliasi</strong><code>POST /v1/tenants/{created.tenant.id}/transactions/refresh</code><code>GET /v1/tenants/{created.tenant.id}/transactions</code></div></div>
        <button className="button button-primary button-wide" onClick={close}>Selesai</button>
      </div> : <TenantForm tenant={modal.kind === "edit" ? selected : null} merchants={merchants} busy={busy} error={error} onSubmit={submit} onCancel={close} />}
    </section></div>}
  </div>;
}

function TenantForm({ tenant, merchants, busy, error, onSubmit, onCancel }: { tenant: Tenant | null; merchants: MerchantID[]; busy: boolean; error: string; onSubmit: (event: React.FormEvent<HTMLFormElement>) => void; onCancel: () => void }) {
  return <form className="tenant-form" onSubmit={onSubmit}>
    <TenantAccessNotice />
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

function TenantAccessNotice() {
  return <aside className="tenant-access-notice" aria-label="Pemisahan jalur akses tenant">
    <div><Server size={17} /><p><strong>API tenant</strong><span>Integrasi aplikasi memakai alamat API khusus pada port 8080 atau domain API public yang diarahkan ke backend.</span></p></div>
    <div><Monitor size={17} /><p><strong>Web admin & browser</strong><span>Web admin berjalan terpisah pada port 3000. Browser worker hanya untuk sinkronisasi internal dan bukan endpoint API tenant.</span></p></div>
  </aside>;
}

function TenantDocumentation({ tenant }: { tenant: Tenant }) {
  const id = tenant.id;
  const apiBase = "https://api.payment.example.com";
  return <div className="tenant-docs">
    <div className="tenant-domain-map" aria-label="Peta arah domain payment gateway">
      <section><span>DOMAIN 1</span><Monitor size={18} /><div><strong>Admin Web</strong><code>https://dashboard.payment.example.com</code><p>Hanya untuk login operator dan dashboard. Arahkan ke service web port 3000. Jangan gunakan domain ini dari aplikasi tenant.</p></div></section>
      <section><span>DOMAIN 2</span><Server size={18} /><div><strong>Tenant API</strong><code>{apiBase}</code><p>Semua request tenant ke <code>/v1/*</code> memakai domain ini. Arahkan langsung ke backend API port 8080 melalui HTTPS.</p></div></section>
      <div className="tenant-domain-flow"><strong>Alur yang benar</strong><code>Aplikasi tenant → api.payment.example.com/v1/* → Backend API → Worker browser internal</code><p>Browser worker tidak memiliki domain public dan tidak boleh dipanggil langsung oleh tenant.</p></div>
    </div>
    <p>Ganti kedua domain contoh sesuai domain produksi Anda. Kirim <code>X-API-Key: YOUR_API_KEY</code> hanya ke domain Tenant API; API key tidak dapat dibaca kembali dari console.</p>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Buat QRIS dinamis dari template</strong><code>{apiBase}/v1/tenants/{id}/qris/dynamic</code><pre>{`{\n  "template_id": "QRIS_TEMPLATE_ID",\n  "amount": 50000\n}`}</pre><p>Respons berisi payload dan gambar PNG base64. Template harus aktif, dapat diakses tenant ini, dan mengikuti rate limit yang diatur dari QRIS Control.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Buat invoice QRIS</strong><code>{apiBase}/v1/tenants/{id}/invoices</code><pre>{`{\n  "merchant_account_id": "MERCHANT_ACCOUNT_ID",\n  "idempotency_key": "ORDER_UNIQUE_ID",\n  "amount": 50000,\n  "currency": "IDR",\n  "description": "Pembayaran order"\n}`}</pre></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Check status invoice</strong><code>{apiBase}/v1/invoices/{`{invoice_id}`}/check</code><p>Mengecek apakah saldo sudah masuk. Maksimum satu check per menit per invoice.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>POST</span>Refresh history browser</strong><code>{apiBase}/v1/tenants/{id}/transactions/refresh</code><p>Request masuk melalui API, lalu backend mengantrekan worker browser internal untuk mengambil mutasi terbaru.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Hasil transaksi tenant</strong><code>{apiBase}/v1/tenants/{id}/transactions?limit=100</code><p>Hanya mengembalikan transaksi portal yang berhasil dihubungkan ke invoice tenant ini.</p></div>
    <div className="tenant-doc-endpoint"><strong><span>GET</span>Detail dan QR invoice</strong><code>{apiBase}/v1/invoices/{`{invoice_id}`}</code><code>{apiBase}/v1/invoices/{`{invoice_id}`}/qr</code></div>
  </div>;
}

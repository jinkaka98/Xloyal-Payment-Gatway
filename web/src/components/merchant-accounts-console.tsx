"use client";

import { useState } from "react";
import { KeyRound, LoaderCircle, Pencil, Plus, RefreshCw, X } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/format";
import type { MerchantAccount, MerchantAccountDetail, Tenant } from "@/lib/types";

export function MerchantAccountsConsole({ initialAccounts, tenants }: { initialAccounts: MerchantAccount[]; tenants: Tenant[] }) {
  const [accounts, setAccounts] = useState(initialAccounts);
  const [editing, setEditing] = useState<MerchantAccount | null>(null);
  const [detail, setDetail] = useState<MerchantAccountDetail | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const tenantNames = Object.fromEntries(tenants.map((tenant) => [tenant.id, tenant.name]));

  async function openEdit(account: MerchantAccount) {
    setEditing(account);
    setDetail(null);
    setError("");
    setBusy(true);
    try {
      const response = await fetch(`/api/admin/merchant-accounts/${encodeURIComponent(account.id)}?tenant_id=${encodeURIComponent(account.tenantId)}`);
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.error ?? `Gagal memuat merchant (${response.status}).`);
      setDetail(payload as MerchantAccountDetail);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Gagal memuat merchant.");
    } finally {
      setBusy(false);
    }
  }

  function close() { setEditing(null); setDetail(null); setError(""); }

  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editing) return;
    setBusy(true);
    setError("");
    const form = new FormData(event.currentTarget);
    const body = {
      tenant_id: String(form.get("tenant_id") ?? ""),
      name: String(form.get("name") ?? "").trim(),
      active: form.get("active") === "on",
    };
    try {
      const response = await fetch(`/api/admin/merchant-accounts/${encodeURIComponent(editing.id)}?tenant_id=${encodeURIComponent(editing.tenantId)}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.error ?? `Gagal menyimpan merchant (${response.status}).`);
      const saved = payload as MerchantAccountDetail;
      setAccounts((current) => current.map((item) => item.id === saved.id ? { ...item, name: saved.name, tenantId: saved.tenant_id, tenantName: tenantNames[saved.tenant_id] ?? saved.tenant_id, active: saved.active } : item));
      close();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Gagal menyimpan merchant.");
    } finally {
      setBusy(false);
    }
  }

  return <div className="page">
    <PageHeader eyebrow="Provider setup" title="Merchant accounts" description="Provider credentials and connectivity assigned to each tenant." actions={<button className="button button-primary"><Plus size={17} />Connect account</button>} />
    <section className="section-block">
      <div className="section-heading"><div><h2>Connected accounts</h2><p>Credential values stay encrypted and are never displayed.</p></div><button className="button"><RefreshCw size={16} />Check all</button></div>
      <div className="table-scroll"><table><thead><tr><th>Account</th><th>Tenant</th><th>Provider</th><th>Connectivity</th><th>Success rate</th><th>Enabled</th><th>Action</th></tr></thead><tbody>
        {accounts.map((account) => <tr key={account.id}><td><strong>{account.name}</strong><span className="cell-subtitle">{account.id}</span></td><td>{tenantNames[account.tenantId] ?? account.tenantName}</td><td>{account.provider}</td><td><StatusBadge status={account.providerStatus} /><span className="cell-subtitle">Checked {formatRelativeTime(account.lastCheckedAt)}</span></td><td className="amount-cell">{account.successRate}%</td><td><StatusBadge status={account.active ? "active" : "inactive"} /></td><td><button className="icon-button table-action" title="Edit account" aria-label={`Edit ${account.name}`} onClick={() => openEdit(account)}><Pencil size={16} /></button></td></tr>)}
      </tbody></table></div>
    </section>
    {editing && <div className="tenant-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}><section className="tenant-modal" role="dialog" aria-modal="true" aria-labelledby="merchant-account-title">
      <header><div><span className="section-kicker">Provider credential</span><h2 id="merchant-account-title">Edit {editing.name}</h2></div><button className="icon-button" aria-label="Tutup" onClick={close}><X size={18} /></button></header>
      {detail ? <form className="tenant-form" onSubmit={save}>
        <label>Nama akun<input name="name" maxLength={120} required defaultValue={detail.name} /></label>
        <label>Tenant<select name="tenant_id" required defaultValue={detail.tenant_id}>{tenants.filter((tenant) => tenant.active).map((tenant) => <option key={tenant.id} value={tenant.id}>{tenant.name} ({tenant.id})</option>)}</select></label>
        <div className="tenant-api-routes"><KeyRound size={17} /><div><strong>Data merchant pihak ketiga (InterActive)</strong><code>merchant_id: {detail.merchant_id || "-"}</code><code>base_url: {detail.base_url || "-"}</code>{detail.create_path ? <code>create_path: {detail.create_path}</code> : null}{detail.check_path ? <code>check_path: {detail.check_path}</code> : null}</div></div>
        <p className="cell-subtitle">API key tetap terenkripsi dan tidak pernah ditampilkan.</p>
        <label className="tenant-active-toggle"><input name="active" type="checkbox" defaultChecked={detail.active} />Akun aktif dan dapat digunakan untuk invoice</label>
        {error && <p className="form-error">{error}</p>}
        <div className="tenant-form-actions"><button type="button" className="button" onClick={close}>Batal</button><button className="button button-primary" disabled={busy}>{busy ? <LoaderCircle className="spin" size={17} /> : <Pencil size={17} />}{busy ? "Menyimpan..." : "Simpan perubahan"}</button></div>
      </form> : <div className="tenant-form"><p className="form-error">{error || "Memuat data merchant..."}</p></div>}
    </section></div>}
  </div>;
}

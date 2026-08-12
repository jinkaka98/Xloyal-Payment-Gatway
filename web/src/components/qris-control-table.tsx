"use client";

import Image from "next/image";
import { ChangeEvent, FormEvent, useState } from "react";
import { CheckCircle2, LoaderCircle, Pencil, Plus, Upload, X } from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { formatDate } from "@/lib/format";
import type { QRISTemplate, Tenant } from "@/lib/types";

type Editor = "create" | QRISTemplate | null;

export function QRISControlTable({ initialTemplates, tenants }: { initialTemplates: QRISTemplate[]; tenants: Tenant[] }) {
  const [templates, setTemplates] = useState(initialTemplates);
  const [editor, setEditor] = useState<Editor>(null);
  const [accessScope, setAccessScope] = useState<QRISTemplate["access_scope"]>("all_tenants");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const tenantNames = Object.fromEntries(tenants.map((tenant) => [tenant.id, tenant.name]));
  const editing = typeof editor === "object" && editor !== null ? editor : null;

  function openCreate() { setEditor("create"); setAccessScope("all_tenants"); setFile(null); setError(""); }
  function openEdit(template: QRISTemplate) { setEditor(template); setAccessScope(template.access_scope || (template.tenant_id ? "selected_tenant" : "all_tenants")); setFile(null); setError(""); }
  function close() { setEditor(null); setFile(null); setError(""); }
  function chooseFile(event: ChangeEvent<HTMLInputElement>) { setFile(event.target.files?.[0] ?? null); }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editing && !file) return;
    setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    try {
      let response: Response;
      if (editing) {
        const body = {
          name: String(form.get("name") ?? "").trim(),
          access_scope: accessScope,
          tenant_id: accessScope === "selected_tenant" ? String(form.get("tenant_id") ?? "") : "",
          static_to_dynamic: form.get("static_to_dynamic") === "on",
          max_requests_per_minute: Number(form.get("max_requests_per_minute")),
          active: form.get("active") === "on",
        };
        response = await fetch(`/api/qris/templates/${encodeURIComponent(editing.id)}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      } else {
        form.set("image", file as File);
        form.set("access_scope", accessScope);
        form.set("tenant_id", accessScope === "selected_tenant" ? String(form.get("tenant_id") ?? "") : "");
        form.set("static_to_dynamic", form.get("static_to_dynamic") === "on" ? "true" : "false");
        response = await fetch("/api/qris/templates", { method: "POST", body: form });
      }
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.error ?? `QRIS gagal disimpan (${response.status}).`);
      const saved = payload as QRISTemplate;
      setTemplates((current) => editing ? current.map((item) => item.id === saved.id ? saved : item) : [saved, ...current]);
      close();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "QRIS gagal disimpan.");
    } finally { setBusy(false); }
  }

  return <div className="page">
    <PageHeader eyebrow="QRIS Control / Templates" title="QRIS Statis ke Dinamis" description="Simpan QRIS statis tervalidasi, atur akses tenant, dan batasi request pembuatan QR dinamis." actions={<button className="button button-primary" onClick={openCreate}><Plus size={17} />Tambah QRIS</button>} />
    <section className="section-block">
      <div className="section-heading"><div><span className="section-kicker">QRIS template directory</span><h2>QRIS statis tersimpan</h2><p>{templates.length} payload dan gambar tersimpan di PostgreSQL.</p></div></div>
      {templates.length === 0 ? <EmptyState title="Belum ada QRIS statis" description="Upload gambar QRIS statis pertama untuk mengaktifkan pembuatan kode pembayaran tenant." /> : <div className="table-scroll"><table><thead><tr><th>QRIS</th><th>Akses Tenant ID</th><th>Merchant QRIS</th><th>Mode</th><th>Rate limit</th><th>Validasi</th><th>Dibuat</th><th>Status</th><th>Action</th></tr></thead><tbody>
        {templates.map((template) => <tr key={template.id}><td><div className="qris-table-name"><Image src={`/api/qris/templates/${template.id}/image`} width={48} height={48} alt={`QRIS ${template.name}`} unoptimized /><span><strong>{template.name}</strong><small><code>{template.id.slice(0, 12)}</code></small></span></div></td><td>{template.access_scope === "all_tenants" ? <><strong>Semua tenant</strong><span className="cell-subtitle">Shared API template</span></> : template.tenant_id ? <><strong>{tenantNames[template.tenant_id] || template.tenant_id}</strong><span className="cell-subtitle"><code>{template.tenant_id}</code></span></> : "Tenant belum dipilih"}</td><td><strong>{template.merchant_name || "Tidak terbaca"}</strong><span className="cell-subtitle">{template.merchant_city || "Kota tidak tersedia"}</span></td><td>{template.static_to_dynamic ? <span className="qris-mode enabled">Statis ke dinamis</span> : <span className="qris-mode">Statis saja</span>}</td><td><strong>{template.max_requests_per_minute || 60} request</strong><span className="cell-subtitle">per menit / tenant</span></td><td><span className="qris-validated"><CheckCircle2 size={15} />Payload + CRC valid</span></td><td>{formatDate(template.created_at)}</td><td><StatusBadge status={template.active ? "active" : "inactive"} /></td><td><button className="icon-button table-action" title="Edit QRIS" aria-label={`Edit ${template.name}`} onClick={() => openEdit(template)}><Pencil size={16} /></button></td></tr>)}
      </tbody></table></div>}
    </section>
    {editor && <div className="tenant-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}><section className="tenant-modal" role="dialog" aria-modal="true" aria-labelledby="qris-upload-title">
      <header><div><span className="section-kicker">QRIS template control</span><h2 id="qris-upload-title">{editing ? `Edit ${editing.name}` : "Upload QRIS statis"}</h2></div><button className="icon-button" aria-label="Tutup" onClick={close}><X size={18} /></button></header>
      <form className="tenant-form qris-control-form" onSubmit={save}>
        <label>Nama QRIS<input name="name" maxLength={100} required placeholder="QRIS toko utama" defaultValue={editing?.name ?? ""} /></label>
        <label>Akses Tenant ID<select name="access_scope" value={accessScope} onChange={(event) => setAccessScope(event.target.value as QRISTemplate["access_scope"])}><option value="all_tenants">Semua tenant</option><option value="selected_tenant">Tenant tertentu</option></select></label>
        {accessScope === "selected_tenant" && <label>Tenant pengguna<select name="tenant_id" required defaultValue={editing?.tenant_id ?? ""}><option value="" disabled>Pilih tenant pengguna</option>{tenants.filter((tenant) => tenant.active).map((tenant) => <option key={tenant.id} value={tenant.id}>{tenant.name} ({tenant.id})</option>)}</select></label>}
        {!editing && <><label className="qris-upload-drop" htmlFor="qris-control-image"><Upload size={22} /><strong>{file?.name ?? "Pilih gambar QRIS"}</strong><span>PNG/JPG maksimal 5 MB. Payload akan didecode dan CRC divalidasi.</span></label><input className="sr-only" id="qris-control-image" type="file" accept="image/png,image/jpeg" onChange={chooseFile} required /></>}
        <label>Maksimum request per menit<input name="max_requests_per_minute" type="number" min="1" max="10000" required defaultValue={editing?.max_requests_per_minute || 60} /></label>
        <label className="tenant-active-toggle"><input name="static_to_dynamic" type="checkbox" defaultChecked={editing?.static_to_dynamic ?? true} />Izinkan API membuat QRIS dinamis dan gambar QR berdasarkan nominal</label>
        {editing && <label className="tenant-active-toggle"><input name="active" type="checkbox" defaultChecked={editing.active} />Template aktif dan dapat digunakan</label>}
        {error && <p className="form-error">{error}</p>}
        <div className="tenant-form-actions"><button type="button" className="button" onClick={close}>Batal</button><button className="button button-primary" disabled={(!editing && !file) || busy}>{busy ? <LoaderCircle className="spin" size={17} /> : editing ? <Pencil size={17} /> : <Upload size={17} />}{busy ? "Menyimpan..." : editing ? "Simpan perubahan" : "Validasi & simpan"}</button></div>
      </form>
    </section></div>}
  </div>;
}

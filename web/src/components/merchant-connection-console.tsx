"use client";

import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Bot, KeyRound, Plus, RefreshCw, ShieldCheck, X } from "lucide-react";
import type { MerchantConnection, MerchantID } from "@/lib/types";

type Notice = { tone: "success" | "error"; text: string } | null;

type ConnectionForm = {
  id: string;
  name: string;
  interactiveMerchantID: string;
  email: string;
  password: string;
};

const emptyForm: ConnectionForm = { id: "", name: "", interactiveMerchantID: "", email: "", password: "" };

async function postJSON(path: string, body: unknown) {
  const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error ?? `Request gagal (${response.status}).`);
  return payload;
}

async function getConnection(merchantID: string) {
  const response = await fetch(`/api/admin/merchant-ids/${encodeURIComponent(merchantID)}/connection`, { cache: "no-store" });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error ?? `Status koneksi gagal dibaca (${response.status}).`);
  return payload as MerchantConnection;
}

function resultMessage(connection: MerchantConnection) {
  if (connection.status === "connected") return { tone: "success" as const, text: "Browser terhubung dan sinkronisasi transaksi selesai." };
  const error = connection.last_error || "Koneksi browser gagal.";
  return { tone: "error" as const, text: error.length > 220 ? `${error.slice(0, 220)}...` : error };
}

function profileState(connection?: MerchantConnection | null) {
  if (connection?.status === "connected") return { label: "Terhubung", detail: "Profile browser aktif dan session siap digunakan." };
  if (connection?.status === "reconnect_required") return { label: "Reconnect terjadwal", detail: "Worker akan membuka profile dan login ulang secara otomatis." };
  return { label: "Belum terhubung", detail: "Simpan kredensial portal lalu jalankan koneksi otomatis." };
}

function connectionID(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}

export function MerchantConnectionConsole({ merchants, connections }: { merchants: MerchantID[]; connections: Array<MerchantConnection | null | undefined> }) {
  const merchant = useMemo(() => merchants.find((item) => item.id === "interactive-browser") ?? merchants[0], [merchants]);
  const connection = merchant ? connections[merchants.findIndex((item) => item.id === merchant.id)] : undefined;
  const [notice, setNotice] = useState<Notice>(null);
  const [busy, setBusy] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [manualLoginOpen, setManualLoginOpen] = useState(false);
  const [form, setForm] = useState<ConnectionForm>(emptyForm);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const profile = profileState(connection);

  useEffect(() => {
    if (!formOpen) return;
    closeButtonRef.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") setFormOpen(false); };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [formOpen]);
  const isConnected = connection?.status === "connected";

  async function startManualLogin() {
    if (!merchant) return;
    setBusy(true);
    setNotice(null);
    try {
      await postJSON(`/api/admin/merchant-ids/${encodeURIComponent(merchant.id)}/connection/manual-login`, {});
      setManualLoginOpen(true);
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "Browser login manual gagal dibuka." });
    } finally {
      setBusy(false);
    }
  }

  async function connect(merchantID = merchant?.id) {
    if (!merchantID) return;
    setBusy(true); setNotice(null);
    try {
      await postJSON(`/api/admin/merchant-ids/${encodeURIComponent(merchantID)}/sync`, {});
      setNotice({ tone: "success", text: "Worker sedang membuka browser dan masuk ke portal..." });
      for (let attempt = 0; attempt < 30; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 2000));
        const current = await getConnection(merchantID);
        if (current.last_error !== "Browser connection queued") {
          setNotice(resultMessage(current));
          window.setTimeout(() => window.location.reload(), 1800);
          return;
        }
      }
      setNotice({ tone: "error", text: "Worker belum memberi hasil setelah 60 detik. Memuat ulang status koneksi..." });
      window.setTimeout(() => window.location.reload(), 1000);
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "Worker gagal dijalankan." });
    } finally { setBusy(false); }
  }

  async function createConnection(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const id = form.id || connectionID(form.name || form.interactiveMerchantID);
    if (!id) return;
    setBusy(true); setNotice(null);
    try {
      await postJSON("/api/admin/merchant-ids", {
        id,
        name: form.name,
        interactive_merchant_id: form.interactiveMerchantID,
        browser_email: form.email,
        browser_password: form.password,
      });
      setFormOpen(false);
      setForm(emptyForm);
      await connect(id);
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "Kredensial portal gagal disimpan." });
      setBusy(false);
    }
  }

  return <section className="connection-console">
    <div className="connection-console-head"><div><span>Browser connector</span><h2>{merchant?.name ?? "InterActive QRIS"}</h2><p>Kredensial portal disimpan terenkripsi dan hanya digunakan worker saat membuka sesi browser.</p></div><Bot size={22} /></div>
    <div className="connection-steps">
      <div className="connection-step"><KeyRound size={17} /><div><span>Browser profile</span><strong>{profile.label}</strong><p>{profile.detail}</p></div></div>
      <div className="connection-step"><Bot size={17} /><div><span>Session lifecycle</span><strong>Otomatis</strong><p>Worker login ulang saat session portal kedaluwarsa.</p></div></div>
      <div className="connection-step"><ShieldCheck size={17} /><div><span>Transaction sync</span><strong>{connection?.last_synced_at ? "Aktif setiap 5 menit" : "Menunggu koneksi"}</strong><p>Transaksi yang terbaca disimpan langsung ke Global Log.</p></div></div>
    </div>
    <div className="connection-actions">{merchant ? <><button className="button button-primary" disabled={busy} onClick={() => void connect()}>{busy ? <RefreshCw className="spin" size={17} /> : isConnected ? <RefreshCw size={17} /> : <Bot size={17} />}{busy ? "Menjalankan..." : isConnected ? "Sync sekarang" : "Hubungkan otomatis"}</button>{connection?.status === "reconnect_required" && <button className="button" disabled={busy} onClick={() => void startManualLogin()}><ShieldCheck size={17} />Selesaikan reCAPTCHA</button>}</> : <button className="button button-primary" disabled={busy} onClick={() => setFormOpen(true)}><Plus size={17} />Tambah koneksi portal</button>}</div>
    {notice && <p className={`connection-notice ${notice.tone}`}>{notice.text}</p>}
    {manualLoginOpen && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setManualLoginOpen(false); }}><section className="modal-panel" role="dialog" aria-modal="true" aria-labelledby="manual-login-title"><header><div><span className="section-kicker">Manual verification</span><h2 id="manual-login-title">Selesaikan reCAPTCHA di browser</h2></div><button type="button" className="icon-button modal-close" aria-label="Tutup dialog" onClick={() => setManualLoginOpen(false)}><X size={18} /></button></header><div className="modal-panel-body"><p className="modal-intro">Browser InterActive QRIS sudah dibuka pada komputer ini dengan profil koneksi yang sama.</p><ol className="manual-login-steps"><li>Selesaikan reCAPTCHA dan klik Masuk pada jendela browser.</li><li>Pastikan dashboard atau riwayat transaksi portal sudah terbuka.</li><li>Kembali ke sini lalu jalankan sinkronisasi.</li></ol></div><footer className="modal-actions"><button type="button" className="button" onClick={() => setManualLoginOpen(false)}>Selesaikan nanti</button><button type="button" className="button button-primary" onClick={() => { setManualLoginOpen(false); void connect(); }}><RefreshCw size={17} />Saya sudah login, sync</button></footer></section></div>}
    {formOpen && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setFormOpen(false); }}><form className="modal-panel" role="dialog" aria-modal="true" aria-labelledby="merchant-connection-title" onSubmit={createConnection}><header><div><span className="section-kicker">Portal credentials</span><h2 id="merchant-connection-title">Hubungkan Merchant ID</h2></div><button ref={closeButtonRef} type="button" className="icon-button modal-close" aria-label="Tutup dialog" onClick={() => setFormOpen(false)}><X size={18} /></button></header><div className="modal-panel-body"><p className="modal-intro">Masukkan kredensial portal InterActive. Password dienkripsi sebelum disimpan dan tidak pernah ditampilkan kembali.</p><div className="merchant-connection-form"><label>Nama koneksi<input required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Contoh: Outlet Jakarta" /></label><label>Interactive Merchant ID<input required value={form.interactiveMerchantID} onChange={(event) => setForm({ ...form, interactiveMerchantID: event.target.value })} placeholder="Merchant ID dari portal" /></label><label>Email portal<input required type="email" autoComplete="username" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} placeholder="email@merchant.id" /></label><label>Password portal<input required type="password" autoComplete="current-password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /></label></div></div><footer className="modal-actions"><button type="button" className="button" onClick={() => setFormOpen(false)}>Batal</button><button type="submit" className="button button-primary" disabled={busy}>{busy ? "Menyimpan..." : "Simpan dan hubungkan"}</button></footer></form></div>}
  </section>;
}

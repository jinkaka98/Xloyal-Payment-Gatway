"use client";

import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Bot, ExternalLink, KeyRound, Plus, RefreshCw, ShieldCheck, X } from "lucide-react";
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

function profileState(connection?: MerchantConnection | null) {
  if (connection?.status === "connected") return { label: "Terhubung", detail: "Profile browser aktif dan session siap digunakan." };
  if (connection?.status === "reconnect_required") return { label: "Reconnect terjadwal", detail: "Worker akan membuka profile dan login ulang secara otomatis." };
  return { label: "Belum terhubung", detail: "Simpan kredensial portal lalu jalankan koneksi otomatis." };
}

function connectionID(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}

export function MerchantConnectionConsole({ merchants, connections, nekoURL }: { merchants: MerchantID[]; connections: Array<MerchantConnection | null | undefined>; nekoURL: string }) {
  const merchant = useMemo(() => merchants.find((item) => item.id === "interactive-browser") ?? merchants[0], [merchants]);
  const connection = merchant ? connections[merchants.findIndex((item) => item.id === merchant.id)] : undefined;
  const [notice, setNotice] = useState<Notice>(null);
  const [busy, setBusy] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState<ConnectionForm>(emptyForm);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const loginPopupRef = useRef<Window | null>(null);
  const profile = profileState(connection);

  useEffect(() => {
    if (!formOpen) return;
    closeButtonRef.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") setFormOpen(false); };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [formOpen]);
  const isConnected = connection?.status === "connected";

  useEffect(() => {
    const handlePopupMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== loginPopupRef.current) return;
      const data = event.data as { type?: string; merchantID?: string; status?: string };
      if (data.type !== "merchant-connecting" || data.merchantID !== merchant?.id) return;
      loginPopupRef.current = null;
      if (data.status === "connected") {
        setNotice({ tone: "success", text: "Browser terhubung. Memuat ulang status koneksi..." });
        window.setTimeout(() => window.location.reload(), 700);
      } else if (data.status === "failed") {
        setNotice({ tone: "error", text: "Browser login manual gagal." });
      }
    };
    window.addEventListener("message", handlePopupMessage);
    return () => window.removeEventListener("message", handlePopupMessage);
  }, [merchant?.id]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (loginPopupRef.current?.closed) {
        loginPopupRef.current = null;
        setNotice({ tone: "error", text: "Popup login ditutup sebelum koneksi selesai." });
      }
    }, 500);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => () => { loginPopupRef.current?.close(); }, []);

  function openLoginPopup(merchantID: string, target?: Window | null) {
    setNotice(null);
    const width = Math.min(520, window.screen.availWidth);
    const height = Math.min(720, window.screen.availHeight);
    const left = Math.max(0, Math.round((window.screen.availWidth - width) / 2));
    const top = Math.max(0, Math.round((window.screen.availHeight - height) / 2));
    const popup = target ?? window.open(
      "",
      `merchant-login-${merchantID}`,
      `popup=yes,width=${width},height=${height},left=${left},top=${top},resizable=yes,scrollbars=yes`,
    );
    if (!popup) {
      setNotice({ tone: "error", text: "Popup diblokir browser. Izinkan popup untuk console ini lalu coba lagi." });
      return null;
    }
    popup.location.href = `/merchant-connecting/login?merchant_id=${encodeURIComponent(merchantID)}`;
    loginPopupRef.current = popup;
    popup.focus();
    return popup;
  }

  function startManualLogin() {
    if (merchant) openLoginPopup(merchant.id);
  }

  async function connect() {
    if (!merchant) return;
    if (isConnected) {
      setBusy(true);
      setNotice(null);
      try {
        await postJSON(`/api/admin/merchant-ids/${encodeURIComponent(merchant.id)}/sync`, {});
        setNotice({ tone: "success", text: "Sinkronisasi transaksi sudah masuk antrean." });
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "Sinkronisasi gagal dijalankan." });
      } finally {
        setBusy(false);
      }
      return;
    }
    openLoginPopup(merchant.id);
  }

  async function createConnection(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const id = form.id || connectionID(form.name || form.interactiveMerchantID);
    if (!id) return;
    const popup = window.open("", `merchant-login-${id}`, "popup=yes,width=520,height=720,resizable=yes,scrollbars=yes");
    if (!popup) {
      setNotice({ tone: "error", text: "Popup diblokir browser. Izinkan popup sebelum menyimpan koneksi." });
      return;
    }
    popup.document.title = "Menyiapkan Merchant Connecting";
    popup.document.body.textContent = "Menyimpan koneksi dan menyiapkan browser login...";
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
      openLoginPopup(id, popup);
    } catch (error) {
      popup.close();
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "Kredensial portal gagal disimpan." });
    } finally {
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
    <div className="connection-actions">{merchant ? <><button className="button button-primary" disabled={busy} onClick={() => void connect()}>{busy ? <RefreshCw className="spin" size={17} /> : isConnected ? <RefreshCw size={17} /> : <Bot size={17} />}{busy ? "Menjalankan..." : isConnected ? "Sync sekarang" : "Hubungkan otomatis"}</button>{connection?.status === "reconnect_required" && <button className="button" disabled={busy} onClick={() => void startManualLogin()}><ShieldCheck size={17} />Selesaikan reCAPTCHA</button>}{nekoURL && <a className="button" href={nekoURL} target="neko-visual-browser" rel="noreferrer"><ExternalLink size={17} />Buka Neko visual</a>}</> : <button className="button button-primary" disabled={busy} onClick={() => setFormOpen(true)}><Plus size={17} />Tambah koneksi portal</button>}</div>
    {nekoURL && <p className="connection-hint">Neko adalah browser visual terpisah. Login di Neko tidak mengubah sesi Webwright dan tidak menandai Merchant Connecting sebagai terhubung.</p>}
    {notice && <p className={`connection-notice ${notice.tone}`}>{notice.text}</p>}

    {formOpen && <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setFormOpen(false); }}><form className="modal-panel" role="dialog" aria-modal="true" aria-labelledby="merchant-connection-title" onSubmit={createConnection}><header><div><span className="section-kicker">Portal credentials</span><h2 id="merchant-connection-title">Hubungkan Merchant ID</h2></div><button ref={closeButtonRef} type="button" className="icon-button modal-close" aria-label="Tutup dialog" onClick={() => setFormOpen(false)}><X size={18} /></button></header><div className="modal-panel-body"><p className="modal-intro">Masukkan kredensial portal InterActive. Password dienkripsi sebelum disimpan dan tidak pernah ditampilkan kembali.</p><div className="merchant-connection-form"><label>Nama koneksi<input required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Contoh: Outlet Jakarta" /></label><label>Interactive Merchant ID<input required value={form.interactiveMerchantID} onChange={(event) => setForm({ ...form, interactiveMerchantID: event.target.value })} placeholder="Merchant ID dari portal" /></label><label>Email portal<input required type="email" autoComplete="username" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} placeholder="email@merchant.id" /></label><label>Password portal<input required type="password" autoComplete="current-password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /></label></div></div><footer className="modal-actions"><button type="button" className="button" onClick={() => setFormOpen(false)}>Batal</button><button type="submit" className="button button-primary" disabled={busy}>{busy ? "Menyimpan..." : "Simpan dan hubungkan"}</button></footer></form></div>}
  </section>;
}

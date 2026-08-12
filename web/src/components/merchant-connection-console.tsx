"use client";

import { useMemo, useState } from "react";
import { Bot, KeyRound, RefreshCw, ShieldCheck } from "lucide-react";
import type { MerchantConnection, MerchantID } from "@/lib/types";

type Notice = { tone: "success" | "error"; text: string } | null;

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
  if (connection.status === "connected") return { tone: "success" as const, text: "Camoufox terhubung dan sinkronisasi transaksi selesai." };
  const error = connection.last_error || "Koneksi browser gagal.";
  if (error.includes("history request failed (400)")) return { tone: "error" as const, text: "Session browser terbentuk, tetapi portal menolak request riwayat transaksi (HTTP 400)." };
  return { tone: "error" as const, text: error.length > 220 ? `${error.slice(0, 220)}...` : error };
}

function profileState(connection?: MerchantConnection | null) {
  if (connection?.status === "connected") return { label: "Terhubung", detail: "Profile browser aktif dan session siap digunakan." };
  if (connection?.status === "reconnect_required") return { label: "Reconnect terjadwal", detail: "Worker akan membuka profile dan login ulang secara otomatis." };
  return { label: "Belum terhubung", detail: "Jalankan koneksi otomatis untuk membuat profile browser." };
}

export function MerchantConnectionConsole({ merchants, connections }: { merchants: MerchantID[]; connections: Array<MerchantConnection | null | undefined> }) {
  const merchant = useMemo(() => merchants.find((item) => item.id === "interactive-browser") ?? merchants[0], [merchants]);
  const connection = merchant ? connections[merchants.findIndex((item) => item.id === merchant.id)] : undefined;
  const [notice, setNotice] = useState<Notice>(null);
  const [busy, setBusy] = useState(false);
  const profile = profileState(connection);
  const isConnected = connection?.status === "connected";

  async function connect() {
    if (!merchant) return;
    setBusy(true); setNotice(null);
    try {
      await postJSON(`/api/admin/merchant-ids/${encodeURIComponent(merchant.id)}/sync`, {});
      setNotice({ tone: "success", text: "Worker sedang membuka Camoufox..." });
      for (let attempt = 0; attempt < 30; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 2000));
        const current = await getConnection(merchant.id);
        if (current.last_error !== "Browser connection queued") {
          setNotice(resultMessage(current));
          window.setTimeout(() => window.location.reload(), 1800);
          return;
        }
      }
      setNotice({ tone: "error", text: "Worker belum memberi hasil setelah 60 detik. Status akan diperbarui pada tabel saat proses selesai." });
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "Worker gagal dijalankan." });
    } finally { setBusy(false); }
  }

  return <section className="connection-console">
    <div className="connection-console-head"><div><span>Camoufox connector</span><h2>{merchant?.name ?? "InterActive QRIS"}</h2><p>Connector browser berjalan di backend dan mengelola session tanpa file dari operator.</p></div><Bot size={22} /></div>
    <div className="connection-steps">
      <div className="connection-step"><KeyRound size={17} /><div><span>Browser profile</span><strong>{profile.label}</strong><p>{profile.detail}</p></div></div>
      <div className="connection-step"><Bot size={17} /><div><span>Session lifecycle</span><strong>Otomatis</strong><p>Worker login ulang saat session portal kedaluwarsa.</p></div></div>
      <div className="connection-step"><ShieldCheck size={17} /><div><span>Transaction sync</span><strong>{connection?.last_synced_at ? "Aktif setiap 5 menit" : "Menunggu koneksi"}</strong><p>Transaksi yang terbaca disimpan langsung ke Global Log.</p></div></div>
    </div>
    <div className="connection-actions"><button className="button button-primary" disabled={!merchant || busy} onClick={connect}>{busy ? <RefreshCw className="spin" size={17} /> : isConnected ? <RefreshCw size={17} /> : <Bot size={17} />}{busy ? "Menjalankan..." : isConnected ? "Sync sekarang" : "Hubungkan otomatis"}</button></div>
    {notice && <p className={`connection-notice ${notice.tone}`}>{notice.text}</p>}
  </section>;
}

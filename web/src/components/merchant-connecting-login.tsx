"use client";

import { CheckCircle2, LoaderCircle, ShieldCheck, TriangleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { MerchantConnection } from "@/lib/types";

type Phase = "starting" | "login" | "syncing" | "complete" | "error";

async function postJSON(path: string) {
  const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error ?? `Request gagal (${response.status}).`);
}

async function getConnection(merchantID: string) {
  const response = await fetch(`/api/admin/merchant-ids/${encodeURIComponent(merchantID)}/connection`, { cache: "no-store" });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error ?? `Status koneksi gagal dibaca (${response.status}).`);
  return payload as MerchantConnection;
}

export function MerchantConnectingLogin({ merchantID }: { merchantID: string }) {
  const [phase, setPhase] = useState<Phase>("starting");
  const [detail, setDetail] = useState("Menyiapkan browser login aman...");
  const startedRef = useRef(false);

  useEffect(function startManualLogin() {
    if (startedRef.current) return;
    startedRef.current = true;
    let cancelled = false;
    let timer: number | undefined;

    async function start() {
      try {
        await postJSON(`/api/admin/merchant-ids/${encodeURIComponent(merchantID)}/connection/manual-login`);
        if (cancelled) return;
        setPhase("login");
        setDetail("Selesaikan login dan reCAPTCHA pada jendela browser portal yang terbuka.");
        const poll = async () => {
          if (cancelled) return;
          await checkConnection();
          if (!cancelled) timer = window.setTimeout(poll, 1500);
        };
        void poll();
      } catch (error) {
        if (cancelled) return;
        setPhase("error");
        setDetail(error instanceof Error ? error.message : "Browser login gagal dimulai.");
      }
    }

    async function checkConnection() {
      try {
        const connection = await getConnection(merchantID);
        if (cancelled) return;

        if (connection.status === "connected") {
          if (timer) window.clearTimeout(timer);
          setPhase("complete");
          setDetail("Login berhasil dan Merchant Connecting sudah stabil.");
          window.history.replaceState(null, "", `/merchant-connecting/login?merchant_id=${encodeURIComponent(merchantID)}&completed=1`);
          window.opener?.postMessage(
            { type: "merchant-connecting", merchantID, status: "connected" },
            window.location.origin,
          );
          window.setTimeout(() => window.close(), 1400);
          return;
        }

        if (connection.last_error === "Browser connection queued") {
          setPhase("syncing");
          setDetail("Login terverifikasi. Menstabilkan sesi dan menyinkronkan transaksi...");
          return;
        }

        if (connection.last_error?.startsWith("Manual browser login failed:")) {
          if (timer) window.clearTimeout(timer);
          setPhase("error");
          setDetail(connection.last_error);
        }
      } catch {
        setDetail("Menunggu respons Merchant Connecting...");
      }
    }

    void start();
    return function stopPolling() {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [merchantID]);

  const active = phase === "starting" || phase === "login" || phase === "syncing";

  return <main className="merchant-popup-shell">
    <section className="merchant-popup-panel" aria-labelledby="merchant-popup-title" aria-live="polite">
      <div className={`merchant-popup-status ${phase}`}>
        {active ? <LoaderCircle className="spin" size={26} /> : phase === "complete" ? <CheckCircle2 size={26} /> : <TriangleAlert size={26} />}
      </div>
      <span className="section-kicker">Secure browser connection</span>
      <h1 id="merchant-popup-title">{phase === "complete" ? "Koneksi berhasil" : phase === "error" ? "Koneksi belum berhasil" : "Selesaikan login portal"}</h1>
      <p>{detail}</p>
      <div className="merchant-popup-steps">
        <div className={phase !== "starting" ? "done" : "active"}><span>1</span><p><strong>Membuka browser</strong><small>Profil persistent merchant disiapkan.</small></p></div>
        <div className={phase === "login" ? "active" : phase === "syncing" || phase === "complete" ? "done" : ""}><span>2</span><p><strong>Login dan reCAPTCHA</strong><small>Diselesaikan langsung pada portal resmi.</small></p></div>
        <div className={phase === "syncing" ? "active" : phase === "complete" ? "done" : ""}><span>3</span><p><strong>Verifikasi koneksi</strong><small>Worker memastikan sesi dapat digunakan.</small></p></div>
      </div>
      {phase === "error" ? <button className="button button-primary" type="button" onClick={() => window.location.reload()}><ShieldCheck size={17} />Coba lagi</button> : null}
      {phase === "login" ? <p className="merchant-popup-hint">Jangan tutup popup ini. Popup akan menutup otomatis setelah login berhasil.</p> : null}
    </section>
  </main>;
}

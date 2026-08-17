"use client";

import Image from "next/image";
import { ChangeEvent, FormEvent, useEffect, useRef, useState } from "react";
import { Check, Copy, LoaderCircle, QrCode, Upload } from "lucide-react";
import { formatCurrency, formatDate } from "@/lib/format";
import type { QRISTemplate, TestPayment } from "@/lib/types";
import { StatusBadge } from "./status-badge";

async function jsonRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { ...init, cache: "no-store" });
  const payload = await response.json();
  if (!response.ok) throw new Error(payload.error ?? `Request failed (${response.status})`);
  return payload as T;
}

export function QRISTestLab() {
  const [templates, setTemplates] = useState<QRISTemplate[]>([]);
  const [payments, setPayments] = useState<TestPayment[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [amount, setAmount] = useState("1000");
  const [file, setFile] = useState<File | null>(null);
  const [templateName, setTemplateName] = useState("");
  const [busy, setBusy] = useState<"upload" | "generate" | "">("");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const reloadInFlight = useRef<Promise<void> | null>(null);

  const latest = payments[0];
  async function reload() {
    if (reloadInFlight.current) return reloadInFlight.current;
    const request = (async () => {
      const [nextTemplates, nextPayments] = await Promise.all([
        jsonRequest<QRISTemplate[]>("/api/qris/templates"),
        jsonRequest<TestPayment[]>("/api/qris/test-payments"),
      ]);
      setTemplates(nextTemplates);
      setPayments(nextPayments);
      setSelectedID((current) => current || nextTemplates[0]?.id || "");
    })();
    reloadInFlight.current = request;
    try {
      await request;
    } finally {
      if (reloadInFlight.current === request) reloadInFlight.current = null;
    }
  }

  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        await reload();
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : "Polling failed");
      } finally {
        if (!cancelled) timer = window.setTimeout(poll, 30_000);
      }
    };
    void poll();
    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, []);

  async function upload(event: FormEvent) {
    event.preventDefault();
    if (!file) return;
    setBusy("upload");
    setError("");
    const data = new FormData();
    data.set("name", templateName || file.name);
    data.set("image", file);
    try {
      const created = await jsonRequest<QRISTemplate>("/api/qris/templates", { method: "POST", body: data });
      await reload();
      setSelectedID(created.id);
      setFile(null);
      setTemplateName("");
      const input = document.querySelector<HTMLInputElement>("#qris-file");
      if (input) input.value = "";
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Upload failed");
    } finally {
      setBusy("");
    }
  }

  async function generate(event: FormEvent) {
    event.preventDefault();
    setBusy("generate");
    setError("");
    try {
      await jsonRequest<TestPayment>("/api/qris/test-payments", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ qris_template_id: selectedID, amount: Number(amount) }),
      });
      await reload();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "QR generation failed");
    } finally {
      setBusy("");
    }
  }

  async function copyPayload() {
    if (!latest) return;
    await navigator.clipboard.writeText(latest.dynamic_payload);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  }

  function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const next = event.target.files?.[0] ?? null;
    setFile(next);
    if (next && !templateName) setTemplateName(next.name.replace(/\.[^.]+$/, ""));
  }

  return <div className="qris-lab">
    {error && <div className="qris-error" role="alert">{error}</div>}
    <section className="qris-workbench">
      <div className="qris-controls">
        <div className="lab-step"><span>1</span><div><strong>Upload QRIS statis</strong><p>PNG atau JPG, maksimum 5 MB. Payload dan gambar disimpan di PostgreSQL.</p></div></div>
        <form className="qris-form" onSubmit={upload}>
          <label>Nama template<input value={templateName} onChange={(event) => setTemplateName(event.target.value)} placeholder="QRIS toko utama" maxLength={100} /></label>
          <label className="file-drop" htmlFor="qris-file"><Upload size={22} /><strong>{file?.name ?? "Pilih gambar QRIS"}</strong><span>QR akan didecode dan CRC divalidasi</span></label>
          <input className="sr-only" id="qris-file" type="file" accept="image/png,image/jpeg" onChange={chooseFile} required />
          <button className="button button-primary button-wide" disabled={!file || busy !== ""}>{busy === "upload" ? <LoaderCircle className="spin" size={17} /> : <Upload size={17} />}Simpan QRIS statis</button>
        </form>

        <div className="lab-divider" />
        <div className="lab-step"><span>2</span><div><strong>Tetapkan nominal</strong><p>Tag 01 diubah ke dinamis, tag 54 diisi, lalu CRC dihitung ulang.</p></div></div>
        <form className="qris-form" onSubmit={generate}>
          <label>Template<select value={selectedID} onChange={(event) => setSelectedID(event.target.value)} required><option value="">Pilih QRIS</option>{templates.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
          <label>Nominal pembayaran<div className="amount-input"><span>Rp</span><input type="number" min="1" max="100000000" value={amount} onChange={(event) => setAmount(event.target.value)} required /></div></label>
          <button className="button button-primary button-wide" disabled={!selectedID || busy !== ""}>{busy === "generate" ? <LoaderCircle className="spin" size={17} /> : <QrCode size={17} />}Buat QRIS dinamis</button>
        </form>
      </div>

      <div className="qris-stage">
        <div className="stage-header"><div><span>PAYMENT TEST / LIVE QR</span><strong>{latest ? formatCurrency(latest.amount) : "Belum ada QR dinamis"}</strong></div>{latest && <StatusBadge status={latest.status} />}</div>
        {latest ? <>
          <div className={`live-qr ${latest.status !== "pending" ? "closed" : ""}`}><Image src={`/api/qris/test-payments/${latest.id}/qr`} alt={`QRIS dinamis ${formatCurrency(latest.amount)}`} width={320} height={320} priority unoptimized />{latest.status !== "pending" && <span>{latest.status === "paid" ? "PAID" : "EXPIRED"}</span>}</div>
          <div className="stage-meta"><div><span>Template</span><strong>{templates.find((item) => item.id === latest.qris_template_id)?.name ?? latest.qris_template_id}</strong></div><div><span>Kedaluwarsa</span><strong>{formatDate(latest.expires_at)}</strong></div><div><span>Merchant ID</span><strong>{latest.merchant_id || "Belum terhubung"}</strong></div><div><span>Validasi</span><strong>{latest.match_confidence.replaceAll("_", " ")}</strong></div></div>
          <button className="button button-wide" onClick={copyPayload}>{copied ? <Check size={16} /> : <Copy size={16} />}{copied ? "Payload disalin" : "Salin payload"}</button>
          <code className="dynamic-payload">{latest.dynamic_payload}</code>
        </> : <div className="qris-stage-empty"><QrCode size={52} /><strong>QR dinamis muncul di sini</strong><p>Upload QRIS statis, pilih nominal, lalu generate.</p></div>}
        <div className="reconcile-note"><strong>Status pembayaran</strong><p>{latest?.status === "expired" ? "Batas pembayaran telah berakhir dan request tidak lagi diperiksa." : latest?.status === "paid" ? "Transaksi cocok ditemukan pada riwayat Merchant." : "Machine Checker memeriksa riwayat Merchant berkala sampai transaksi cocok atau batas waktu berakhir."}</p></div>
      </div>
    </section>

    <section className="section-block template-library">
      <div className="section-heading"><div><h2>QRIS tersimpan</h2><p>{templates.length} template tersimpan di PostgreSQL.</p></div></div>
      {templates.length === 0 ? <div className="qris-library-empty">Upload QRIS statis pertama untuk memulai.</div> : <div className="template-grid">{templates.map((item) => <button type="button" key={item.id} className={item.id === selectedID ? "template-card selected" : "template-card"} onClick={() => setSelectedID(item.id)}>
        <Image src={`/api/qris/templates/${item.id}/image`} alt={item.name} width={96} height={96} unoptimized />
        <span><strong>{item.name}</strong><small>{item.merchant_name || "Merchant QRIS"} · {item.merchant_city || "Indonesia"}</small></span>
      </button>)}</div>}
    </section>

    <section className="section-block test-history">
      <div className="section-heading"><div><h2>Transaksi percobaan</h2><p>QR yang dibuat dari template statis.</p></div></div>
      {payments.length === 0 ? <div className="qris-library-empty">Belum ada transaksi percobaan.</div> : <div className="table-scroll"><table><thead><tr><th>ID</th><th>Merchant ID</th><th>Nominal</th><th>Status</th><th>Check terakhir</th><th>Validasi</th><th>Dibuat</th><th>Kedaluwarsa</th></tr></thead><tbody>{payments.map((item) => <tr key={item.id}><td><code>{item.id.slice(0, 12)}</code></td><td><code>{item.merchant_id || "Belum terhubung"}</code></td><td className="amount-cell">{formatCurrency(item.amount)}</td><td><StatusBadge status={item.status} /><span className="cell-subtitle">{item.check_count || 0} kali check</span></td><td>{item.last_checked_at ? formatDate(item.last_checked_at) : "Belum dicek"}</td><td>{item.match_confidence.replaceAll("_", " ")}</td><td>{formatDate(item.created_at)}</td><td>{formatDate(item.expires_at)}</td></tr>)}</tbody></table></div>}
    </section>
  </div>;
}

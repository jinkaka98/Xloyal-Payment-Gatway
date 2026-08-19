"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  ArrowRight,
  Check,
  Code2,
  Copy,
  ExternalLink,
  FileCode,
  FileText,
  Key,
  Layers,
  Lock,
  Paintbrush,
  PlayCircle,
  Radio,
  RefreshCw,
  Server,
  ShieldCheck,
  Terminal,
  Zap,
} from "lucide-react";

type CodeLang = "curl" | "node" | "php" | "python" | "go";

interface EndpointDoc {
  id: string;
  method: "GET" | "POST" | "PUT" | "DELETE";
  path: string;
  badge: string;
  title: string;
  description: string;
  auth: "API Key (Server)" | "Public Token (Browser / Client)" | "Admin Token";
  headers: Array<{ key: string; required: boolean; desc: string }>;
  params?: Array<{ name: string; type: string; required: boolean; desc: string }>;
  bodyExample?: string;
  responseSuccess: { status: number; body: string };
  responseError?: { status: number; body: string };
  sampleLinks?: Array<{ label: string; url: string; external?: boolean }>;
  codeSnippets: Record<CodeLang, string>;
}

const endpoints: EndpointDoc[] = [
  {
    id: "create-payment-session",
    method: "POST",
    path: "/v1/payment-sessions",
    badge: "Core Workflow",
    title: "Buat Sesi Checkout Pembayaran (Create PaymentSession)",
    description:
      "Endpoint utama yang dipanggil oleh backend merchant untuk membuat URL checkout hosted (`checkout_url`). Sesi ini akan menampilkan halaman pembayaran QRIS interaktif dengan tema aktif.",
    auth: "API Key (Server)",
    headers: [
      { key: "Content-Type", required: true, desc: "application/json" },
      { key: "X-API-Key", required: true, desc: "API Key rahasia merchant/tenant (misal: xl_live_xxx)" },
      { key: "Idempotency-Key", required: false, desc: "Kunci unik pencegah duplikasi transaksi pada saat retry" },
    ],
    params: [
      { name: "invoice_id", type: "string", required: true, desc: "ID Invoice yang sudah dibuat melalui endpoint invoice tenant" },
      { name: "theme_id", type: "string", required: false, desc: "ID tema kustom (opsional, default memakai tema aktif)" },
      { name: "success_url", type: "string", required: false, desc: "URL redirect ketika pembayaran berhasil" },
      { name: "cancel_url", type: "string", required: false, desc: "URL redirect ketika pembayaran dibatalkan" },
      { name: "failed_url", type: "string", required: false, desc: "URL redirect ketika pembayaran gagal" },
      { name: "expired_url", type: "string", required: false, desc: "URL redirect ketika sesi kedaluwarsa" },
    ],
    bodyExample: JSON.stringify(
      {
        invoice_id: "inv_01HQXLOYAL2026",
        success_url: "https://toko-anda.com/order/success",
        cancel_url: "https://toko-anda.com/order/cancelled",
      },
      null,
      2,
    ),
    responseSuccess: {
      status: 201,
      body: JSON.stringify(
        {
          session_id: "sess_98f12a819c4d",
          invoice_id: "INV-2026-0819-001",
          status: "payment_pending",
          amount: 50000,
          currency: "IDR",
          checkout_url: "{{PAYMENT_BASE_URL}}/pay/tok_pub_77a19c4d92fa1b88e09f4",
          qr_payload: "00020101021226590014ID.LINKAJA.WWW01189360091100215000102150005802ID5913Demo Merchant6007Jakarta6304ABCD",
          expires_at: "2026-08-19T16:30:00Z",
        },
        null,
        2,
      ),
    },
    responseError: {
      status: 400,
      body: JSON.stringify(
        {
          code: "invalid_request",
          title: "Bad Request",
          status: 400,
          detail: "Amount minimal pembayaran adalah 1000 IDR",
        },
        null,
        2,
      ),
    },
    sampleLinks: [
      { label: "Uji Live Simulator Sesi Ini", url: "/payment-page/testing" },
      { label: "Lihat Contoh Tampilan Checkout", url: "/custom-web-payment" },
    ],
    codeSnippets: {
      curl: `curl -X POST '{{API_BASE_URL}}/v1/payment-sessions' \\
  -H 'Content-Type: application/json' \\
  -H 'X-API-Key: xl_live_xxxxxxxxxxxxxx' \\
  -H 'Idempotency-Key: ord-unique-9921' \\
  -d '{
    "invoice_id": "inv_01HQXLOYAL2026",
    "success_url": "https://toko-anda.com/order/success",
    "cancel_url": "https://toko-anda.com/order/cancelled"
  }'`,
      node: `// Node.js (TypeScript / JavaScript Server-Side)
const response = await fetch('{{API_BASE_URL}}/v1/payment-sessions', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-API-Key': process.env.XLOYAL_API_KEY,
    'Idempotency-Key': 'ord-unique-9921'
  },
  body: JSON.stringify({
    invoice_id: 'inv_01HQXLOYAL2026',
    success_url: 'https://toko-anda.com/order/success',
    cancel_url: 'https://toko-anda.com/order/cancelled'
  })
});

const session = await response.json();
console.log('Redirect Customer to:', session.checkout_url);`,
      php: `<?php
// PHP cURL
$ch = curl_init('{{API_BASE_URL}}/v1/payment-sessions');
$payload = json_encode([
    'invoice_id' => 'inv_01HQXLOYAL2026',
    'success_url' => 'https://toko-anda.com/order/success',
    'cancel_url' => 'https://toko-anda.com/order/cancelled'
]);

curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);
curl_setopt($ch, CURLOPT_HTTPHEADER, [
    'Content-Type: application/json',
    'X-API-Key: ' . getenv('XLOYAL_API_KEY'),
    'Idempotency-Key: ord-unique-9921'
]);
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
$response = json_decode(curl_exec($ch), true);
curl_close($ch);

header('Location: ' . $response['checkout_url']);
exit;`,
      python: `import requests
import os

url = "{{API_BASE_URL}}/v1/payment-sessions"
headers = {
    "Content-Type": "application/json",
    "X-API-Key": os.getenv("XLOYAL_API_KEY"),
    "Idempotency-Key": "ord-unique-9921"
}
payload = {
    "invoice_id": "inv_01HQXLOYAL2026",
    "success_url": "https://toko-anda.com/order/success",
    "cancel_url": "https://toko-anda.com/order/cancelled"
}

res = requests.post(url, json=payload, headers=headers)
session = res.json()
print("Redirect URL:", session["checkout_url"])`,
      go: `package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func main() {
	payload, _ := json.Marshal(map[string]interface{}{
		"invoice_id":  "inv_01HQXLOYAL2026",
		"success_url": "https://toko-anda.com/order/success",
		"cancel_url":  "https://toko-anda.com/order/cancelled",
	})

	req, _ := http.NewRequest("POST", "{{API_BASE_URL}}/v1/payment-sessions", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", os.Getenv("XLOYAL_API_KEY"))
	req.Header.Set("Idempotency-Key", "ord-unique-9921")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Println("Checkout URL:", result["checkout_url"])
}`,
    },
  },
  {
    id: "get-payment-session",
    method: "GET",
    path: "/v1/payment-sessions/{public_token}",
    badge: "Public Snapshot",
    title: "Ambil Snapshot & Status Sesi Pembayaran",
    description:
      "Endpoint publik tanpa API Key rahasia yang digunakan oleh frontend checkout atau backend untuk memeriksa status transaksi terkini.",
    auth: "Public Token (Browser / Client)",
    headers: [{ key: "Accept", required: true, desc: "application/json" }],
    params: [{ name: "public_token", type: "string (path)", required: true, desc: "Token sesi unik yang diterima saat pembuatan sesi" }],
    responseSuccess: {
      status: 200,
      body: JSON.stringify(
        {
          session_id: "sess_98f12a819c4d",
          invoice_id: "INV-2026-0819-001",
          status: "paid",
          amount: 50000,
          currency: "IDR",
          description: "Pembelian Langganan VIP 1 Bulan",
          paid_at: "2026-08-19T16:04:12Z",
          redirect: {
            success_url: "https://toko-anda.com/order/success",
          },
        },
        null,
        2,
      ),
    },
    codeSnippets: {
      curl: `curl -X GET '{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN'`,
      node: `const res = await fetch('{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN');
const data = await res.json();
console.log('Status Pembayaran:', data.status); // "paid" / "payment_pending"`,
      php: `$res = file_get_contents('{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN');
$data = json_decode($res, true);
echo 'Status: ' . $data['status'];`,
      python: `import requests
res = requests.get("{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN")
print("Status:", res.json()["status"])`,
      go: `resp, _ := http.Get("{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN")
// parse json response`,
    },
  },
  {
    id: "sse-events-stream",
    method: "GET",
    path: "/v1/payment-sessions/{public_token}/events",
    badge: "Real-time Live Stream",
    title: "Server-Sent Events (SSE) Live Payment Stream",
    description:
      "Koneksi streaming HTTP persistent untuk mendeteksi perubahan status transaksi (pending -> verifying -> paid) secara instan tanpa polling berulang.",
    auth: "Public Token (Browser / Client)",
    headers: [{ key: "Accept", required: true, desc: "text/event-stream" }],
    params: [
      { name: "public_token", type: "string (path)", required: true, desc: "Token publik sesi pembayaran" },
      { name: "after_sequence", type: "number (query)", required: false, desc: "Sequence index terakhir untuk melanjutkan koneksi yang terputus" },
    ],
    responseSuccess: {
      status: 200,
      body: `event: payment.paid
id: 3
data: {"event_id":"evt_88a91b2c","payment_session_id":"sess_98f12a819c4d","invoice_id":"INV-2026-0819-001","status":"paid","sequence":3,"occurred_at":"2026-08-19T16:04:12Z"}`,
    },
    codeSnippets: {
      curl: `curl -N -H 'Accept: text/event-stream' \\
  '{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN/events?after_sequence=0'`,
      node: `// Client Browser / Node EventSource
const eventSource = new EventSource(
  '{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN/events'
);

eventSource.addEventListener('payment.paid', (event) => {
  const data = JSON.parse(event.data);
  console.log('🎉 Pembayaran Berhasil Dikonfirmasi!', data);
  // Tampilkan layar sukses atau trigger redirect
});`,
      php: `// Direkomendasikan menggunakan EventSource di browser JavaScript client`,
      python: `import requests
url = "{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN/events"
with requests.get(url, stream=True) as r:
    for line in r.iter_lines():
        if line:
            print("SSE Event:", line.decode("utf-8"))`,
      go: `// Gunakan streaming HTTP client di Go untuk membaca text/event-stream`,
    },
  },
  {
    id: "cancel-payment-session",
    method: "POST",
    path: "/v1/payment-sessions/{public_token}/cancel",
    badge: "Cancellation",
    title: "Batalkan Sesi Checkout (Cancel PaymentSession)",
    description:
      "Membatalkan sesi pembayaran yang sedang aktif. Jika transaksi sudah terlanjur dibayar atau kedaluwarsa, server akan mengembalikan respon terminal 409 Conflict secara aman.",
    auth: "Public Token (Browser / Client)",
    headers: [{ key: "Content-Type", required: true, desc: "application/json" }],
    params: [{ name: "public_token", type: "string (path)", required: true, desc: "Token publik sesi yang akan dibatalkan" }],
    responseSuccess: {
      status: 200,
      body: JSON.stringify(
        {
          session_id: "sess_98f12a819c4d",
          status: "cancelled",
          message: "Payment session cancelled successfully",
        },
        null,
        2,
      ),
    },
    codeSnippets: {
      curl: `curl -X POST '{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN/cancel'`,
      node: `const res = await fetch('{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN/cancel', {
  method: 'POST'
});
const result = await res.json();
console.log('Status Batal:', result.status);`,
      php: `// POST cancel request via cURL`,
      python: `import requests
res = requests.post("{{API_BASE_URL}}/v1/payment-sessions/PUBLIC_TOKEN/cancel")
print(res.json())`,
      go: `// POST cancel request`,
    },
  },
  {
    id: "create-dynamic-qris",
    method: "POST",
    path: "/v1/tenants/{tenant_id}/transactions/qris",
    badge: "Direct QRIS Engine",
    title: "Generate QRIS Dinamis Langsung (Direct QRIS API)",
    description:
      "Digunakan jika merchant ingin menampilkan QRIS secara custom langsung di aplikasi Android/iOS/POS milik sendiri tanpa menggunakan hosted web checkout.",
    auth: "API Key (Server)",
    headers: [
      { key: "Content-Type", required: true, desc: "application/json" },
      { key: "X-API-Key", required: true, desc: "API Key tenant" },
      { key: "Idempotency-Key", required: false, desc: "Kunci idempotensi transaksi" },
    ],
    params: [
      { name: "tenant_id", type: "string (path)", required: true, desc: "ID Tenant Anda" },
      { name: "amount", type: "number", required: true, desc: "Nominal bayar (Rupiah)" },
      { name: "template_id", type: "string", required: true, desc: "ID template dari GET /v1/tenants/{tenant_id}/qris/templates" },
      { name: "idempotency_key", type: "string", required: false, desc: "Kunci idempotensi; dapat dikirim sebagai body atau header Idempotency-Key" },
      { name: "expires_in_seconds", type: "number", required: false, desc: "Masa berlaku 60-1800 detik, default 1800" },
    ],
    bodyExample: JSON.stringify(
      {
        amount: 25000,
        template_id: "QRIS_TEMPLATE_ID",
        idempotency_key: "POS-10023",
        expires_in_seconds: 1800,
      },
      null,
      2,
    ),
    responseSuccess: {
      status: 201,
      body: JSON.stringify(
        {
          id: "trx_qris_99210ab",
          tenant_id: "tenant_demo",
          amount: 25000,
          payable_amount: 25001,
          unique_amount_code: 1,
          status: "pending",
          qris_template_id: "QRIS_TEMPLATE_ID",
          qr_payload: "00020101021226590014ID.LINKAJA.WWW...",
          qr_url: "{{API_BASE_URL}}/v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID/qr",
          status_url: "{{API_BASE_URL}}/v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID",
          expires_at: "2026-08-19T16:30:00Z",
        },
        null,
        2,
      ),
    },
    codeSnippets: {
      curl: `curl -X POST '{{API_BASE_URL}}/v1/tenants/TENANT_ID/transactions/qris' \\
  -H 'Content-Type: application/json' \\
  -H 'X-API-Key: {{TENANT_API_KEY}}' \\
  -H 'Idempotency-Key: POS-10023' \\
  -d '{"template_id":"QRIS_TEMPLATE_ID","amount":25000,"expires_in_seconds":1800}'`,
      node: `const res = await fetch('{{API_BASE_URL}}/v1/tenants/TENANT_ID/transactions/qris', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-API-Key': process.env.XLOYAL_API_KEY,
    'Idempotency-Key': 'POS-10023'
  },
  body: JSON.stringify({ template_id: 'QRIS_TEMPLATE_ID', amount: 25000, expires_in_seconds: 1800 })
});
const qr = await res.json();
console.log('QR Payload:', qr.qr_payload);`,
      php: `// PHP implementation`,
      python: `import os, requests
res = requests.post("{{API_BASE_URL}}/v1/tenants/TENANT_ID/transactions/qris", json={"template_id":"QRIS_TEMPLATE_ID","amount":25000,"expires_in_seconds":1800}, headers={"X-API-Key": os.environ["XLOYAL_API_KEY"], "Idempotency-Key":"POS-10023"})`,
      go: `// POST JSON ke endpoint yang sama dan baca qr_payload/status_url`,
    },
  },
];

export default function ApiDocsPage() {
  const [selectedLang, setSelectedLang] = useState<CodeLang>("curl");
  const [activeEndpointId, setActiveEndpointId] = useState<string>("create-payment-session");
  const [copiedCodeId, setCopiedCodeId] = useState<string | null>(null);

  const activeEndpoint: EndpointDoc =
    endpoints.find((e) => e.id === activeEndpointId) ?? endpoints[0]!;

  const handleCopyCode = async (codeText: string, id: string) => {
    try {
      await navigator.clipboard.writeText(codeText);
      setCopiedCodeId(id);
      setTimeout(() => setCopiedCodeId(null), 2000);
    } catch {
      // ignore
    }
  };

  return (
    <div className="page api-docs-v2-page">
      {/* 1. HERO HEADER */}
      <header className="docs-hero-banner">
        <div className="docs-hero-content">
          <div className="docs-hero-badge">
            <ShieldCheck size={14} /> Dokumentasi Resmi API alpakyros.com (v1.0)
          </div>
          <h1>Integrasi Payment Gateway & Hosted QRIS Checkout</h1>
          <p>
            Panduan lengkap REST API untuk integrasi backend merchant: Pembuatan Sesi Pembayaran,
            Kustomisasi Tema, Streaming Real-time (SSE), dan Verifikasi Webhook Idempotent.
          </p>

          <div className="docs-hero-action-buttons">
            <Link href="/payment-page/testing" className="button button-primary hero-btn">
              <PlayCircle size={15} /> Uji Simulator di Testing Mode
            </Link>
            <Link href="/custom-web-payment" className="button hero-btn-secondary">
              <Paintbrush size={15} /> Studio Desain Web Payment
            </Link>
            <a
              href="/openapi/openapi.yaml"
              target="_blank"
              rel="noreferrer"
              className="button hero-btn-secondary"
            >
              <FileCode size={15} /> OpenAPI 3.1 Contract
            </a>
          </div>
        </div>

        <div className="docs-hero-card">
          <div className="docs-server-pill">
            <span className="server-dot" /> Base API Endpoint
          </div>
          <code className="docs-server-url">{"{{API_BASE_URL}}"}</code>
          <div className="docs-specs-list">
            <div>
              <strong>Format Respon</strong>
              <span>JSON (RFC 7807 Problem)</span>
            </div>
            <div>
              <strong>Autentikasi</strong>
              <span>Header <code>X-API-Key</code></span>
            </div>
            <div>
              <strong>Real-Time Stream</strong>
              <span>Server-Sent Events (SSE)</span>
            </div>
            <div>
              <strong>Idempotensi</strong>
              <span>Header <code>Idempotency-Key</code></span>
            </div>
          </div>
        </div>
      </header>

      {/* 2. WORKFLOW ARCHITECTURE FLOWCHART */}
      <section className="docs-workflow-card">
        <div className="docs-workflow-head">
          <Zap size={16} style={{ color: "var(--accent)" }} />
          <h2>Alur Transaksi Pembayaran (End-to-End Workflow)</h2>
        </div>

        <div className="docs-workflow-steps-grid">
          <div className="workflow-step-box">
            <span className="step-num">1</span>
            <strong>Merchant Backend</strong>
            <p>Customer checkout barang. Backend merchant memanggil <code>POST /v1/payment-sessions</code> dengan nominal pesanan.</p>
          </div>
          <div className="workflow-step-arrow">→</div>

          <div className="workflow-step-box">
            <span className="step-num">2</span>
            <strong>Dapatkan Checkout URL</strong>
            <p>Server mengembalikan <code>checkout_url</code> dan QR payload unik. Merchant mengalihkan (*redirect*) customer ke URL tersebut.</p>
          </div>
          <div className="workflow-step-arrow">→</div>

          <div className="workflow-step-box highlight">
            <span className="step-num">3</span>
            <strong>Customer Scan QRIS</strong>
            <p>Customer membuka layar checkout berlogo merchant dan memindai QR code atau klik simpan gambar.</p>
          </div>
          <div className="workflow-step-arrow">→</div>

          <div className="workflow-step-box">
            <span className="step-num">4</span>
            <strong>Real-time & Webhook</strong>
            <p>Setelah dibayar, sistem mengirimkan notifikasi <strong>Webhook</strong> instan ke server Anda dan customer diarahkan ke <code>success_url</code>.</p>
          </div>
        </div>
      </section>

      {/* 3. MAIN INTERACTIVE 2-COLUMN API EXPLORER */}
      <div className="docs-main-container">
        {/* SIDEBAR NAVIGATION OF ENDPOINTS */}
        <aside className="docs-nav-sidebar">
          <div className="docs-nav-title">DAFTAR ENDPOINT API</div>
          <div className="docs-nav-items">
            {endpoints.map((e) => (
              <button
                key={e.id}
                type="button"
                className={`docs-nav-endpoint-btn ${activeEndpointId === e.id ? "active" : ""}`}
                onClick={() => setActiveEndpointId(e.id)}
              >
                <span className={`method-badge method-${e.method.toLowerCase()}`}>{e.method}</span>
                <div className="docs-nav-endpoint-info">
                  <strong>{e.title}</strong>
                  <code>{e.path}</code>
                </div>
              </button>
            ))}
          </div>

          <div className="docs-sidebar-notice">
            <Lock size={14} />
            <div>
              <strong>Keamanan Kredensial</strong>
              <p>Simpan <code>X-API-Key</code> di server backend Anda. Jangan pernah menyertakannya pada JavaScript browser publik.</p>
            </div>
          </div>

          <div className="docs-sidebar-notice">
            <RefreshCw size={14} />
            <div>
              <strong>Siklus Direct QRIS</strong>
              <p>Ambil template sebelum membuat QR:</p>
              <code>GET /v1/tenants/TENANT_ID/qris/templates</code>
              <p>Setelah membuat transaksi, gunakan:</p>
              <code>GET /v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID</code>
              <code>GET /v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID/qr</code>
              <code>POST /v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID/cancel</code>
              <p>Cancel menghasilkan <code>200</code> saat pending, <code>409</code> bila sudah terminal, dan QR terminal menghasilkan <code>410 Gone</code>.</p>
            </div>
          </div>
        </aside>

        {/* ACTIVE ENDPOINT DOCUMENTATION & CODE PLAYGROUND */}
        <main className="docs-detail-panel">
          {/* HEADER OF SELECTED ENDPOINT */}
          <div className="endpoint-detail-header">
            <div className="endpoint-meta-top">
              <span className={`method-pill method-${activeEndpoint.method.toLowerCase()}`}>
                {activeEndpoint.method}
              </span>
              <code className="endpoint-path-badge">{activeEndpoint.path}</code>
              <span className="auth-badge">
                <Key size={12} /> {activeEndpoint.auth}
              </span>
            </div>
            <h2>{activeEndpoint.title}</h2>
            <p className="endpoint-desc">{activeEndpoint.description}</p>

            {activeEndpoint.sampleLinks && (
              <div className="endpoint-sample-links">
                {activeEndpoint.sampleLinks.map((link) => (
                  <Link key={link.url} href={link.url} className="sample-link-btn">
                    <ExternalLink size={13} /> {link.label}
                  </Link>
                ))}
              </div>
            )}
          </div>

          {/* REQUEST HEADERS */}
          <div className="doc-section-block">
            <h3>Request Headers</h3>
            <div className="table-responsive">
              <table className="docs-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Wajib</th>
                    <th>Deskripsi</th>
                  </tr>
                </thead>
                <tbody>
                  {activeEndpoint.headers.map((h) => (
                    <tr key={h.key}>
                      <td><code>{h.key}</code></td>
                      <td>{h.required ? <span className="tag-required">Wajib</span> : <span className="tag-optional">Opsional</span>}</td>
                      <td>{h.desc}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* PARAMETERS / REQUEST BODY */}
          {activeEndpoint.params && (
            <div className="doc-section-block">
              <h3>Parameters & Request Body</h3>
              <div className="table-responsive">
                <table className="docs-table">
                  <thead>
                    <tr>
                      <th>Parameter</th>
                      <th>Tipe</th>
                      <th>Wajib</th>
                      <th>Keterangan</th>
                    </tr>
                  </thead>
                  <tbody>
                    {activeEndpoint.params.map((p) => (
                      <tr key={p.name}>
                        <td><code>{p.name}</code></td>
                        <td><span className="type-badge">{p.type}</span></td>
                        <td>{p.required ? <span className="tag-required">Wajib</span> : <span className="tag-optional">Opsional</span>}</td>
                        <td>{p.desc}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* CODE EXAMPLES WITH LANGUAGE SWITCHER */}
          <div className="doc-section-block">
            <div className="code-box-header">
              <div className="code-lang-tabs">
                {(["curl", "node", "php", "python", "go"] as const).map((lang) => (
                  <button
                    key={lang}
                    type="button"
                    className={`lang-tab-btn ${selectedLang === lang ? "active" : ""}`}
                    onClick={() => setSelectedLang(lang)}
                  >
                    {lang === "curl"
                      ? "cURL"
                      : lang === "node"
                        ? "Node.js (TS)"
                        : lang === "php"
                          ? "PHP"
                          : lang === "python"
                            ? "Python"
                            : "Go"}
                  </button>
                ))}
              </div>

              <button
                type="button"
                className="copy-code-btn"
                onClick={() =>
                  void handleCopyCode(
                    activeEndpoint.codeSnippets[selectedLang],
                    `${activeEndpoint.id}-${selectedLang}`,
                  )
                }
              >
                {copiedCodeId === `${activeEndpoint.id}-${selectedLang}` ? (
                  <>
                    <Check size={13} /> Tersalin!
                  </>
                ) : (
                  <>
                    <Copy size={13} /> Salin Kode
                  </>
                )}
              </button>
            </div>

            <pre className="docs-code-snippet">
              <code>{activeEndpoint.codeSnippets[selectedLang]}</code>
            </pre>
          </div>

          {/* SAMPLE RESPONSE PREVIEW */}
          <div className="doc-section-block">
            <div className="response-header-row">
              <h3>Contoh Respon Server</h3>
              <span className="status-code-badge status-200">
                HTTP {activeEndpoint.responseSuccess.status} OK
              </span>
            </div>
            <pre className="docs-code-snippet response-snippet">
              <code>{activeEndpoint.responseSuccess.body}</code>
            </pre>
          </div>

          {activeEndpoint.responseError && (
            <div className="doc-section-block">
              <div className="response-header-row">
                <h3>Contoh Respon Error (Problem Details)</h3>
                <span className="status-code-badge status-400">
                  HTTP {activeEndpoint.responseError.status}
                </span>
              </div>
              <pre className="docs-code-snippet response-snippet-err">
                <code>{activeEndpoint.responseError.body}</code>
              </pre>
            </div>
          )}
        </main>
      </div>

      {/* 4. WEBHOOK SECTION & HMAC VERIFICATION */}
      <section className="docs-webhook-guide-card" id="webhook-guide">
        <div className="docs-workflow-head">
          <Radio size={18} style={{ color: "#059669" }} />
          <h2>Integrasi Webhook Notifikasi & Verifikasi Tanda Tangan</h2>
        </div>

        <p style={{ fontSize: "13.5px", color: "var(--muted)", lineHeight: 1.5, margin: "6px 0 16px" }}>
          Ketika customer menyelesaikan pembayaran QRIS, sistem alpakyros.com akan mengirimkan HTTP POST
          notifikasi secara asynchronous ke URL Webhook Anda. Pastikan server Anda merespon dengan status <code>200 OK</code>.
        </p>

        <div className="webhook-headers-grid">
          <div className="webhook-header-card">
            <code>X-Xloyal-Signature</code>
            <span>Tanda tangan HMAC SHA-256 payload transaksi untuk memvalidasi keaslian pengirim.</span>
          </div>
          <div className="webhook-header-card">
            <code>X-Xloyal-Timestamp</code>
            <span>Waktu pengiriman event (Unix Epoch timestamp) untuk mencegah replay attack.</span>
          </div>
          <div className="webhook-header-card">
            <code>X-Xloyal-Event-ID</code>
            <span>ID unik event transaksi untuk penerapan mekanisme idempotency di server Anda.</span>
          </div>
        </div>

        <div className="doc-section-block" style={{ marginTop: "16px" }}>
          <div className="code-box-header">
            <strong>Contoh Payload Webhook (Event: payment.paid)</strong>
          </div>
          <pre className="docs-code-snippet">
            <code>{JSON.stringify(
              {
                event_id: "evt_99182390abcf1",
                event: "payment.paid",
                occurred_at: "2026-08-19T16:04:12Z",
                data: {
                  session_id: "sess_98f12a819c4d",
                  invoice_id: "INV-2026-0819-001",
                  amount: 50000,
                  currency: "IDR",
                  status: "paid",
                  payment_method: "QRIS",
                  merchant_id: "merchant_main",
                },
              },
              null,
              2,
            )}</code>
          </pre>
        </div>
      </section>

      {/* 5. SAMPLE URLS, LIVE REALITY SANDBOX & REDIRECT LIFECYCLE */}
      <section className="docs-reality-sandbox-card" id="sample-urls-reality">
        <div className="docs-workflow-head">
          <Terminal size={18} style={{ color: "var(--accent)" }} />
          <div>
            <h2>Contoh Live URL Checkout & Alur Pengalihan (Customer Experience)</h2>
            <p style={{ margin: "2px 0 0", fontSize: "12px", color: "var(--muted)" }}>
              Coba langsung URL transaksi asli dari sisi pengguna untuk melihat tampilan nyata, simulasi tombol salin nominal, unduh barcode, hingga pengalihan sukses dan pembatalan pesanan.
            </p>
          </div>
        </div>

        <div className="reality-links-grid">
          {/* Card 1: Live Hosted Pay URL */}
          <div className="reality-link-card highlight-card">
            <div className="reality-card-top">
              <span className="reality-tag reality-tag-live">● Live Checkout Customer</span>
              <span className="reality-method">GET</span>
            </div>
            <strong>Contoh Tautan Checkout Pengguna (Hosted Pay URL)</strong>
            <p>
              Tautan pembayaran yang dikirimkan ke customer. Customer akan melihat logo Anda, nominal presisi, timer countdown, dan barcode QRIS.
            </p>
            <div className="reality-url-box">
              <code>{"{{PAYMENT_BASE_URL}}/pay/demo-preview-checkout"}</code>
              <button
                type="button"
                className="reality-copy-btn"
                onClick={() =>
                  void handleCopyCode(
                    `${typeof window !== "undefined" ? window.location.origin : "{{PAYMENT_BASE_URL}}"}/pay/demo-preview-checkout`,
                    "url-pay",
                  )
                }
              >
                {copiedCodeId === "url-pay" ? <Check size={12} /> : <Copy size={12} />}
              </button>
            </div>
            <div className="reality-actions">
              <a
                href="/pay/demo-preview-checkout"
                target="_blank"
                rel="noreferrer"
                className="button button-primary"
                style={{ fontSize: "12px", minHeight: "34px", display: "inline-flex", alignItems: "center", gap: "6px" }}
              >
                <ExternalLink size={13} /> Buka Layar Checkout Customer
              </a>
            </div>
          </div>

          {/* Card 2: Success Redirect URL */}
          <div className="reality-link-card">
            <div className="reality-card-top">
              <span className="reality-tag reality-tag-success">✓ Success Redirect</span>
              <span className="reality-param">Query Params</span>
            </div>
            <strong>Contoh Link Redirect Pembayaran Sukses (success_url)</strong>
            <p>
              URL toko merchant yang dituju setelah customer berhasil membayar (otomatis dialihkan setelah countdown 5 detik atau klik manual).
            </p>
            <div className="reality-url-box">
              <code>https://toko-anda.com/order/success?order_id=ORD-9921&status=paid</code>
              <button
                type="button"
                className="reality-copy-btn"
                onClick={() =>
                  void handleCopyCode(
                    "https://toko-anda.com/order/success?order_id=ORD-9921&status=paid",
                    "url-success",
                  )
                }
              >
                {copiedCodeId === "url-success" ? <Check size={12} /> : <Copy size={12} />}
              </button>
            </div>
            <div className="reality-query-spec">
              <span>Parameter yang dikirim: <code>order_id</code>, <code>status=paid</code>, <code>session_id</code></span>
            </div>
          </div>

          {/* Card 3: Cancel URL */}
          <div className="reality-link-card">
            <div className="reality-card-top">
              <span className="reality-tag reality-tag-cancel">✕ Cancel Order</span>
              <span className="reality-param">Customer Action</span>
            </div>
            <strong>Contoh Link Pembatalan Pesanan (cancel_url)</strong>
            <p>
              URL yang dituju jika customer menekan tombol <em>&ldquo;Cancle Order&rdquo;</em> pada layar pembayaran atau membatalkan transaksi.
            </p>
            <div className="reality-url-box">
              <code>https://toko-anda.com/order/cancelled?order_id=ORD-9921&status=cancelled</code>
              <button
                type="button"
                className="reality-copy-btn"
                onClick={() =>
                  void handleCopyCode(
                    "https://toko-anda.com/order/cancelled?order_id=ORD-9921&status=cancelled",
                    "url-cancel",
                  )
                }
              >
                {copiedCodeId === "url-cancel" ? <Check size={12} /> : <Copy size={12} />}
              </button>
            </div>
            <div className="reality-query-spec">
              <span>Parameter yang dikirim: <code>order_id</code>, <code>status=cancelled</code></span>
            </div>
          </div>

          {/* Card 4: Expired / Failed URL */}
          <div className="reality-link-card">
            <div className="reality-card-top">
              <span className="reality-tag reality-tag-expired">⌛ Expired / Failed</span>
              <span className="reality-param">Timeout</span>
            </div>
            <strong>Contoh Link Kedaluwarsa / Gagal (expired_url & failed_url)</strong>
            <p>
              URL tujuan jika waktu countdown pembayaran habis (mencapai 00:00) atau transaksi ditolak oleh bank pemroses.
            </p>
            <div className="reality-url-box">
              <code>https://toko-anda.com/order/expired?order_id=ORD-9921&status=expired</code>
              <button
                type="button"
                className="reality-copy-btn"
                onClick={() =>
                  void handleCopyCode(
                    "https://toko-anda.com/order/expired?order_id=ORD-9921&status=expired",
                    "url-expired",
                  )
                }
              >
                {copiedCodeId === "url-expired" ? <Check size={12} /> : <Copy size={12} />}
              </button>
            </div>
            <div className="reality-query-spec">
              <span>Parameter yang dikirim: <code>order_id</code>, <code>status=expired</code></span>
            </div>
          </div>
        </div>

        {/* Quick Testing Actions Footer */}
        <div className="reality-sandbox-footer">
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <PlayCircle size={16} style={{ color: "var(--accent)" }} />
            <strong>Pusat Pengujian Cepat Simulator:</strong>
          </div>
          <div style={{ display: "flex", gap: "10px", flexWrap: "wrap" }}>
            <Link href="/payment-page/testing" className="button button-primary" style={{ fontSize: "12px", minHeight: "34px" }}>
              Buka Testing Mode Simulator
            </Link>
            <Link href="/custom-web-payment" className="button" style={{ fontSize: "12px", minHeight: "34px" }}>
              Kustomisasi Tema & Logo Checkout
            </Link>
          </div>
        </div>
      </section>
    </div>
  );
}

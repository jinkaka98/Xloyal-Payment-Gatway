# Integrasi Direct QRIS API Merchant Web dengan Xloyal

Dokumen ini khusus untuk website merchant eksternal yang membuat, menampilkan, memantau, dan membatalkan transaksi QRIS melalui Direct QRIS API Xloyal. Dokumen ini tidak menggunakan `payment-sessions` atau hosted checkout redirect.

## 1. Konfigurasi tenant

Admin memberikan nilai berikut dari menu **Tenant ID**:

- `TENANT_ID`
- `X-API-Key`
- `API_BASE_URL`: `https://api.alpakyros.net`
- `SITE_URL`: URL website merchant yang terdaftar pada tenant

Jangan menaruh API key di JavaScript browser. Simpan API key hanya di backend merchant.

Jika memakai webhook:

1. Admin mengisi `Webhook URL` pada Tenant ID.
2. Admin memilih **Buat secret**.
3. Secret ditampilkan satu kali. Simpan di secret manager/backend merchant.
4. Rotasi secret membatalkan secret lama.

Tanpa webhook secret, gunakan polling status sebagai sumber kebenaran.

## 2. Autentikasi dan origin

Semua request tenant menggunakan:

```http
X-API-Key: TENANT_API_KEY
Content-Type: application/json
```

Pada mode Production, request browser harus berasal dari origin `SITE_URL`. Pola yang disarankan adalah:

```text
Browser customer -> Backend merchant -> https://api.alpakyros.net
```

Jangan memanggil API gateway langsung dari browser jika itu menyebabkan API key terekspos.

## 3. Ambil template QRIS

```http
GET /v1/tenants/TENANT_ID/qris/templates
X-API-Key: TENANT_API_KEY
```

Contoh response:

```json
[
  {
    "id": "qris_template_id",
    "name": "QRIS Merchant",
    "active": true,
    "static_to_dynamic": true
  }
]
```

Gunakan `id` dari response. Jangan menebak atau mengisi `template_id` sembarang.

## 4. Buat transaksi QRIS dinamis

```http
POST /v1/tenants/TENANT_ID/transactions/qris
X-API-Key: TENANT_API_KEY
Idempotency-Key: ORDER_UNIQUE_ID
Content-Type: application/json
```

Body minimal:

```json
{
  "template_id": "qris_template_id",
  "amount": 10000,
  "idempotency_key": "ORDER_UNIQUE_ID"
}
```

Contoh response penting:

```json
{
  "id": "transaction_id",
  "status": "pending",
  "requested_amount": 10000,
  "payable_amount": 10037,
  "unique_amount_code": 37,
  "qr_payload": "...",
  "qr_png_base64": "...",
  "status_url": "/v1/tenants/TENANT_ID/transactions/qris/transaction_id",
  "qr_url": "/v1/tenants/TENANT_ID/transactions/qris/transaction_id/qr",
  "poll_after_seconds": 15,
  "expires_at": "2026-08-20T12:30:00Z"
}
```

Jika `unique_amount_code` aktif, customer harus membayar `payable_amount`, bukan `requested_amount`.

Simpan `transaction_id` dan `idempotency_key` di database merchant. Key yang sama dengan body yang sama aman untuk retry; body berbeda dengan key yang sama menghasilkan `409`.

## 5. Tampilkan QR

Jika response berisi PNG base64:

```html
<img src="data:image/png;base64,QR_PNG_BASE64" alt="QRIS pembayaran" />
```

Atau gunakan:

```text
GET https://api.alpakyros.net/v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID/qr
```

QR yang sudah `expired` atau `cancelled` tidak dapat digunakan lagi dan mengembalikan `410 Gone`.

## 6. Polling status

```http
GET /v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID
X-API-Key: TENANT_API_KEY
```

Polling hanya ketika status `pending`. Tunggu `poll_after_seconds` atau header `Retry-After`, dan pastikan tidak ada dua request status paralel untuk transaksi yang sama.

Hentikan polling pada status:

- `paid`: pembayaran berhasil.
- `expired`: batas waktu habis.
- `failed`: transaksi gagal.
- `cancelled`: dibatalkan merchant/customer.

## 7. Batalkan transaksi

Saat customer menekan Cancel, Kembali, Close, atau batal pembayaran:

1. Hentikan timer polling lokal.
2. Panggil endpoint cancel.
3. Gunakan response server sebagai sumber kebenaran.

```http
POST /v1/tenants/TENANT_ID/transactions/qris/TRANSACTION_ID/cancel
X-API-Key: TENANT_API_KEY
```

Hasil:

| Kondisi | HTTP | Hasil |
|---|---:|---|
| `pending` -> `cancelled` | 200 | Response transaksi terbaru |
| `cancelled` -> `cancelled` | 200 | Idempotent |
| `paid`, `expired`, `failed` | 409 | Cancel ditolak; ikuti status server |
| Transaksi tidak ditemukan/milik tenant lain | 404 | Tidak tersedia |

Jika network error, jangan mengubah UI menjadi `cancelled` sebelum status server berhasil diterima.

## 8. Webhook HMAC

Webhook dikirim sebagai `POST` JSON ke `Webhook URL` dengan header:

```http
X-Xloyal-Event: payment.paid
X-Xloyal-Event-ID: event_id
X-Xloyal-Timestamp: 1723939200
X-Xloyal-Signature: sha256=HEX_DIGEST
```

Payload harus dibaca sebagai **raw bytes** sebelum JSON parsing:

```json
{
  "event_id": "event_id",
  "event": "payment.paid",
  "timestamp": "2026-08-20T12:00:00Z",
  "data": {
    "payment_session_id": "payment_session_id",
    "invoice_id": "invoice_id",
    "status": "paid",
    "amount": 10000,
    "currency": "IDR",
    "paid_at": "2026-08-20T12:00:00Z"
  }
}
```

Verifikasi signature:

```text
canonical = X-Xloyal-Timestamp + "." + raw_request_body
expected = HMAC-SHA256(TENANT_WEBHOOK_SECRET, canonical)
```

Bandingkan `expected` dengan nilai setelah prefix `sha256=` menggunakan constant-time comparison. Tolak timestamp di luar replay window dan tolak event ID yang sudah diproses. Balas HTTP `2xx` setelah event berhasil disimpan; balasan non-2xx dapat dicoba ulang oleh gateway.

Contoh Node.js/TypeScript:

```ts
import crypto from "node:crypto";

function verifyXloyalWebhook(rawBody: Buffer, timestamp: string, signature: string, secret: string) {
  const age = Math.abs(Math.floor(Date.now() / 1000) - Number(timestamp));
  if (!Number.isFinite(age) || age > 300) return false;
  const expected = crypto.createHmac("sha256", secret).update(`${timestamp}.${rawBody.toString("utf8")}`).digest("hex");
  const provided = signature.replace(/^sha256=/, "");
  return provided.length === expected.length && crypto.timingSafeEqual(Buffer.from(provided), Buffer.from(expected));
}
```

## 9. Error handling

| HTTP | Arti | Tindakan |
|---:|---|---|
| 400 | Request/nominal/template tidak valid | Perbaiki request |
| 401 | API key hilang/tidak valid | Periksa secret backend |
| 403 | Origin tidak diizinkan | Sesuaikan Site URL tenant |
| 404 | Template/transaksi tidak ditemukan | Ambil template dan transaction ID dari API |
| 409 | Idempotency bentrok atau cancel status terminal | Jangan membuat duplikasi; baca status server |
| 410 | QR sudah tidak berlaku | Hentikan pembayaran dan buat transaksi baru |
| 429 | Rate limit/kode unik penuh | Tunggu `Retry-After` |
| 5xx | Gangguan server | Retry dengan backoff dan idempotency key yang sama |

## 10. Checklist produksi

- API key dan webhook secret hanya berada di backend/secret manager.
- `Webhook URL` memakai HTTPS.
- `template_id` diambil dari endpoint template.
- `Idempotency-Key` unik per order dan disimpan.
- Hanya satu polling aktif per transaksi.
- UI berhenti polling pada status terminal.
- Cancel selalu memanggil server.
- Webhook diverifikasi dari raw body, timestamp, dan HMAC.
- Event webhook diproses idempotent berdasarkan `event_id`.
- Status final berasal dari server, bukan dari state UI lokal.

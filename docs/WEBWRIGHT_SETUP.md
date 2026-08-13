# Webwright-Clockbrowser Setup

Dokumen ini menjelaskan setup Webwright-Clockbrowser sebagai browser checker untuk worker Xloyal, termasuk kontrak adapter dan alur pemulihan koneksi.

## Status Integrasi

Runtime Go menjalankan command dari environment `CAMOUFOX_CHECKER_CMD`. Adapter Webwright tersedia di `backend/tools/webwright_checker.py`; adapter ini menjadi proses eksternal yang dipanggil worker. Agar Webwright digunakan, jalankan adapter tersebut dan pastikan dependency Python tersedia. Adapter:

1. Membaca satu objek JSON dari stdin.
2. Membuka atau menggunakan sesi portal Merchant ID.
3. Mengambil transaksi portal.
4. Menulis satu objek JSON ke stdout.
5. Menulis log diagnostik ke stderr, bukan stdout.

Backend tidak membutuhkan nama executable tertentu. Nilai `CAMOUFOX_CHECKER_CMD` hanya harus menunjuk ke adapter yang kompatibel.

## Kontrak Adapter

Input yang dikirim worker:

```json
{
  "merchant_id": "merchant_123",
  "cookies": []
}
```

`cookies` berupa JSON cookies hasil dekripsi `SessionCiphertext`. Jika belum ada sesi tersimpan, nilainya adalah array kosong.

Output yang wajib dikembalikan adapter:

```json
{
  "transactions": [
    {
      "reference": "portal-reference",
      "amount": 25000,
      "status": "paid",
      "paid_at": "2026-08-12T12:00:00Z"
    }
  ]
}
```

Field transaksi yang dipakai worker adalah `reference`, `amount`, `status`, dan `paid_at`. Worker menambahkan `merchant_id`, `source`, `id`, dan timestamp internal sebelum menyimpan hasil.

Jika adapter gagal, keluar dengan exit code bukan nol dan tulis pesan singkat ke stderr atau stdout. Worker akan menyimpan error tersebut pada koneksi Merchant ID dan mengubah statusnya menjadi `reconnect_required`.

## Instalasi Webwright Lokal

Jalankan dari PowerShell:

```powershell
Push-Location backend/tools/Webwright-Clockbrowser
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install --upgrade pip
.\.venv\Scripts\python.exe -m pip install -e .
.\.venv\Scripts\python.exe -m playwright install
Pop-Location
```

Paket project memiliki executable `webwright` dan dependency CloakBrowser. Instalasi ini hanya menyiapkan tool; belum menghubungkannya ke worker.

## Environment Backend

Jangan commit file `.env` atau credentials. Gunakan `.env.example` sebagai dasar dan isi nilai rahasia secara lokal atau melalui secret manager.

```dotenv
DATABASE_URL=postgres://xloyal:change-me@localhost:5432/xloyal?sslmode=disable
CREDENTIAL_ENCRYPTION_KEY=replace-with-unpadded-base64-32-byte-key
CAMOUFOX_CHECKER_CMD=python C:/path/to/Xloyal\ Payment\ Gatway/backend/tools/webwright_checker.py
WEBWRIGHT_PORTAL_URL=https://merchant.qris.interactive.co.id
WEBWRIGHT_HISTORY_PATH=/v2/m/kontenr.php?idir=pages/historytrx.php
WEBWRIGHT_TRANSACTION_ROW_SELECTOR=[data-transaction-row]
```

`WEBWRIGHT_PORTAL_URL` adalah portal browser Merchant (`https://merchant.qris.interactive.co.id`), bukan base URL API QRIS. Base URL API provider adalah `https://qris.interactive.co.id` (lihat Runbook → Third-party endpoints).

`CREDENTIAL_ENCRYPTION_KEY` harus berupa base64 tanpa padding yang setelah decode berukuran 32 byte. API dan worker harus memakai key yang sama.

Untuk Docker, command harus menunjuk path yang tersedia di dalam container worker. Contoh:

```dotenv
CAMOUFOX_CHECKER_CMD=python /app/webwright_checker.py
```

Pastikan adapter, package Python, dan konfigurasi browser ikut tersedia di image worker. Mengatur environment saja tidak cukup jika file adapter tidak ada di container.

## Menjalankan Stack

Dengan PostgreSQL dan dependency deployment tersedia:

```powershell
docker compose up --build
```

Endpoint pemeriksaan:

- Web: `http://localhost:3000`
- API health: `http://localhost:8080/v1/health`

Untuk menjalankan API secara langsung di host, set `DATABASE_URL` dan `CREDENTIAL_ENCRYPTION_KEY` terlebih dahulu. API akan berhenti pada startup jika `DATABASE_URL` tidak ada.

Worker adalah proses terpisah:

```powershell
Push-Location backend
go run ./cmd/worker
Pop-Location
```

Worker baru akan menjalankan browser sync bila `CAMOUFOX_CHECKER_CMD` terisi. Tanpa variable tersebut, worker tetap dapat hidup tetapi tidak memiliki fungsi `MerchantSync`.

## Alur Reconnect

1. Import HAR atau revoke/upload session menandai koneksi sebagai `reconnect_required` atau `disconnected`.
2. Permintaan sync dari admin mengatur `updated_at` ke epoch agar koneksi langsung masuk antrean.
3. Worker mengambil koneksi yang sudah jatuh tempo setiap 5 detik.
4. Worker memanggil adapter dengan timeout maksimum 150 detik.
5. Jika berhasil, transaksi disimpan dan koneksi menjadi `connected`.
6. Jika gagal, koneksi tetap `reconnect_required` dan `last_error` menyimpan diagnostik.

Endpoint yang dapat dipakai UI/admin untuk mengantrekan sync:

```http
POST /admin/merchants/{merchant_id}/sync
```

Gunakan token admin sesuai konfigurasi `ADMIN_TOKENS_JSON`.

## Verifikasi Adapter

Uji adapter tanpa menjalankan worker penuh:

```powershell
'{"merchant_id":"merchant_123","cookies":[]}' | python C:/path/to/webwright_checker.py
```

Pastikan hasilnya adalah JSON valid dan hanya berisi payload transaksi. Contoh validasi minimal:

```powershell
$result = '{"merchant_id":"merchant_123","cookies":[]}' | python C:/path/to/webwright_checker.py | ConvertFrom-Json
$result.transactions
```

Jika adapter memakai konfigurasi Webwright, jalankan smoke test dari directory tool:

```powershell
Push-Location backend/tools/Webwright-Clockbrowser
.\.venv\Scripts\python.exe examples/cloak_smoke_test.py
Pop-Location
```

## Diagnostik Umum

- `DATABASE_URL is required`: environment API/worker belum dimuat.
- `CAMOUFOX_CHECKER_CMD is not configured`: worker tidak mendapat adapter.
- `ModuleNotFoundError: No module named 'webwright'`: package belum dipasang atau executable Python yang dipakai bukan `.venv` project.
- `browser sync failed`: lihat `last_error` pada koneksi Merchant ID dan log adapter.
- Output JSON gagal diparse: adapter mencetak log ke stdout atau format output tidak sesuai kontrak.
- Session tidak pulih: upload cookies JSON yang valid, pastikan `CREDENTIAL_ENCRYPTION_KEY` tidak berubah, lalu antrekan sync ulang.

## Catatan Keamanan

Webwright/CloakBrowser tidak boleh menerima credentials melalui source code atau command-line arguments. Gunakan environment/secret mount, redaksi log, profile terisolasi per Merchant ID, dan jangan mencetak cookies ke stdout.

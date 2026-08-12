# HTTP Toolkit untuk Merchant Browser Connection

HTTP Toolkit dipakai sebagai observability saat menyambungkan portal InterActive QRIS. Ia merekam request/response browser untuk menemukan route, parameter, status autentikasi, dan bentuk data transaksi. Ia tidak menggantikan cookie/session portal.

## Port lokal

- `http://127.0.0.1:8001`: proxy HTTP/HTTPS untuk Camoufox.
- `127.0.0.1:45456`: Mockttp control API.
- `127.0.0.1:45457`: HTTP Toolkit server/control API.
- `8000` bukan proxy aktif pada instalasi ini.

## Mengamati browser melalui HTTP Toolkit

PowerShell lokal:

```powershell
$env:HTTP_TOOLKIT_PROXY_URL = "http://127.0.0.1:8001"
```

Proxy ini hanya digunakan ketika service `camofox-browser` sengaja dijalankan dalam mode inspeksi lokal. Docker Compose produksi tidak bergantung pada HTTP Toolkit.

Runtime transaksi menggunakan `backend/tools/camofox_browser_checker.mjs` untuk mengontrol service `backend/tools/camofox-browser` melalui REST. Profile, cookie, local storage, dan lifecycle browser dimiliki service tersebut, bukan worker Go dan bukan HTTP Toolkit.

## Alur integrasi yang direkomendasikan

1. Developer membuka HTTP Toolkit dan mengaktifkan capture saat kontrak portal perlu dianalisis ulang.
2. Jalankan service browser dengan proxy HTTP Toolkit pada environment development.
3. Jalankan `Sync sekarang` dan validasi halaman history serta request `proses.php?required=getTransactions`.
4. Matikan proxy setelah analisis; worker tetap memakai service browser secara langsung setiap lima menit.
5. Hasil normalisasi disimpan ke Global Log dengan sumber `browser`.

Jangan menyimpan password portal atau menyalin cookie ke log. Credential login dipasang sebagai secret backend, sedangkan session baru disimpan otomatis dalam volume profile `camofox-browser`.

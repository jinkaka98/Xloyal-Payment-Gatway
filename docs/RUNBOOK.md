# Xloyal Runbook

## Development

1. Copy `.env.example` to `.env` and replace every placeholder.
2. Generate `CREDENTIAL_ENCRYPTION_KEY` with `openssl rand -base64 32 | tr -d '='`. Keep the decoded value exactly 32 bytes.
3. Start the stack with `docker compose up --build`.
4. Check `http://localhost:8080/v1/health` and `http://localhost:3000`.
5. Run `powershell -File deploy/smoke.ps1` before submitting infrastructure changes.

## Database Migration

`deploy-tidakdepploykedoscker/postgres/init/001_init.sql` runs only when PostgreSQL creates an empty data volume. For an existing environment created before the `creating` invoice state was added, apply `deploy-tidakdepploykedoscker/postgres/migrations/002_invoice_creating_status.sql` before starting the new API. Back up the database first; never delete the named volume as a migration strategy.

## Admin Console

The admin console uses the real API by default. `ADMIN_API_TOKEN` must match a key in `ADMIN_TOKENS_JSON`. `ADMIN_CONSOLE_PASSWORD` protects interactive sign-in and `CONSOLE_SESSION_SECRET` signs the eight-hour HttpOnly console session. Set `NEXT_PUBLIC_USE_MOCK_API=true` only for isolated UI development.

## Secrets

- Keep `.env`, provider credentials, API keys, encryption keys, and admin bearer tokens out of Git and logs.
- Store production values in the deployment platform's secret manager.
- `ADMIN_TOKENS_JSON` maps opaque tokens to `viewer`, `operator`, or `super_admin`.
- Rotating `CREDENTIAL_ENCRYPTION_KEY` requires re-encrypting stored merchant credentials; changing it directly makes existing ciphertext unreadable.
- Rotate tenant API keys by hashing the replacement with the application-compatible SHA-256 format and updating the tenant atomically.

## Browser Worker

The `camofox-browser` service owns the Camoufox engine and persistent browser profiles. The Go `worker` calls `backend/tools/camofox_browser_checker.mjs`, which controls that service through its private REST API. Neither the API nor worker image contains a Python browser runtime. `merchant_browser_profiles` persists one isolated browser profile per Merchant ID across service restarts.

Set `CAMOFOX_BROWSER_API_KEY` to a long random value and provide the same value to the worker and browser service as defined in `compose.yaml`. Set `CAMOUFOX_CREDENTIAL_FILE` to a file mounted from the deployment platform's secret manager; it contains the portal email on the first line and password on the second. Leave the default checker command and browser URL unchanged. Do not bake credentials or profile contents into an image. HTTP Toolkit remains optional local diagnostics and is not part of the deployment runtime.

## Status Polling Limits

The worker polls once per minute, processes at most 100 due invoices per pass, and stops checking an invoice after 30 checks, 30 minutes, or its expiry time. Manual `POST /v1/invoices/{invoice_id}/check` calls also increment `check_count`; clients should avoid tight loops and use intervals of at least 60 seconds. Treat `paid`, `expired`, and `failed` as terminal.

## Provider Boundary

The runtime accepts only the credential-backed `interactive_qris` provider. Merchant credentials use `base_url`, `merchant_id`, `api_key`, and optional `create_path`/`check_path`. Production credentials must use `https://qris.interactive.co.id`; test clients may inject a local fixture server.

## QRIS Test Lab

`/qris-test` accepts a PNG/JPEG static QRIS image up to 5 MB. The API decodes the QR, requires static mode (`01=11`), IDR currency (`53=360`), and a valid CRC, then stores the original image and payload in PostgreSQL. Creating a test payment switches the payload to dynamic mode (`01=12`), injects tag `54`, recalculates CRC-16, and stores the generated payload in `test_payments`. The request queues the linked Merchant ID browser sync, is checked every minute against unique amount/time matches, and persists `paid` or `expired` after the 30-minute deadline. Its source, check count, validation result, and deadline appear in Global Log Transaksi.

The generated QR can carry a real payment to the merchant. Automatic status confirmation is separate: locally generated test IDs are not InterActive QRIS invoice IDs. Keep them `pending` until an InterActive QRIS status response confirms the payment. Apply `deploy-tidakdepploykedoscker/postgres/migrations/003_qris_test_workflow.sql` to existing databases.

## Public Portal Capture

The HAR at `bahan/merchant.qris.interactive.co.id.har` records authenticated page titles for settlement, summary, history, and verification screens, but its 396 entries contain only GET asset traffic. It contains no document requests, request bodies, cookie headers, login POST, payment creation call, or payment-status XHR.

The observed portal routes are useful only for understanding the user interface. Xloyal uses the documented InterActive QRIS endpoints for invoice creation and status checks because the HAR does not expose a stable transaction contract.

# Hosted Payment Integration Test Runbook

Phase 8 uses a fresh PostgreSQL instance and the real API, worker, webhook,
SSE, and Next.js processes. The memory repository tests remain useful for
deterministic domain and concurrency checks, but they are not a replacement for
the database run.

## Prerequisites

- Docker Desktop with the Linux engine running.
- Go 1.23 or newer.
- Node.js and the checked-in `web/node_modules` (or `npm ci`).
- A test-only `DATABASE_URL`; never point this at development or production.
- Playwright is optional. The repository currently has no dedicated Playwright
  dependency, so browser E2E is reported as skipped until the existing runtime
  provides it.

## Database migration check

Start the isolated PostgreSQL 17 test project. Its database uses tmpfs and its
name/port are separate from CasaOS and development data:

```powershell
docker compose -f deploy/compose.integration.yaml up --build --abort-on-container-exit migrate
```

For manual inspection, keep `postgres` running and apply migrations in lexical
order:

```powershell
$env:DATABASE_URL = "postgres://xloyal:xloyal@127.0.0.1:55432/xloyal?sslmode=disable"
Get-ChildItem backend/migrations/*.sql | Sort-Object Name | ForEach-Object {
  go run ./backend/cmd/migrate $_.FullName
}
```

The expected latest migration is `020_payment_theme_builder.sql`. It depends
on the previous hosted-payment migrations `018_hosted_payment_foundation.sql`
and `019_webhook_deliveries.sql`. Verify the tables, composite foreign keys,
immutable-version trigger, unique default index, outbox lease indexes, and
webhook delivery identity constraint before creating application data.

## Application flow

Run the API and worker against that database, create a test tenant/invoice,
then exercise:

1. invoice and payment session creation;
2. public snapshot and SSE cursor replay;
3. paid, expired, failed, and cancelled transitions;
4. outbox claim/recovery and webhook delivery/retry;
5. theme draft, publish, default, duplicate, archive, and version snapshot;
6. tenant isolation and secret-response scans.

The focused automated commands are:

```powershell
Push-Location backend
go test ./... -count=1
go vet ./...
Pop-Location
Push-Location web
npm.cmd test -- --run
npm.cmd run typecheck
npm.cmd run build
Pop-Location
```

## Current environment result

The Phase 8 run in this workspace cannot start PostgreSQL because Docker
Desktop's Linux engine is unavailable (`docker version` cannot connect to
`dockerDesktopLinuxEngine`). Therefore fresh-database migration assertions,
PostgreSQL row-lock/constraint races, real webhook HTTP delivery, and hosted
page browser E2E are **NOT RUN**, rather than simulated with the memory store.

The available deterministic checks cover payment-session transitions, atomic
event/outbox behavior in the memory test double, SSE public DTO and slow-client
handling, webhook signature/replay-window/SSRF policy, QRIS worker polling,
150-client validation batching, theme lifecycle/RBAC/tenant isolation, and
frontend production build/tests.

Known limitation retained: if a successful payment-session create response is
lost, the opaque plaintext checkout token cannot be reconstructed from the
database. The system does not store that token in plaintext.

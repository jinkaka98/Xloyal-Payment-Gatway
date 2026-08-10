# Xloyal Payment Gateway

Multi-tenant QRIS gateway with a Go API and polling worker, PostgreSQL persistence, and a Next.js administration console.

## Quick Start

```powershell
Copy-Item .env.example .env
# Replace all placeholder values in .env.
docker compose up --build
```

- Admin console: `http://localhost:3000`
- QRIS upload and transaction test: `http://localhost:3000/qris-test`
- API health: `http://localhost:8080/v1/health`
- API contract: [`openapi/openapi.yaml`](openapi/openapi.yaml)
- Operations: [`docs/RUNBOOK.md`](docs/RUNBOOK.md)

Public payment endpoints require `X-API-Key`. Administration endpoints require an `Authorization: Bearer <token>` value configured in `ADMIN_TOKENS_JSON`.

## Local Verification

```powershell
powershell -File deploy-tidakdepploykedoscker/smoke.ps1
Push-Location backend; go test ./...; Pop-Location
Push-Location web; npm test; npm run typecheck; npm run build; Pop-Location
```

The initial schema is mounted from `deploy/postgres/init/001_init.sql` and is applied only to a new PostgreSQL data volume.


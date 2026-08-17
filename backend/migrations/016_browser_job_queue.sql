CREATE TABLE IF NOT EXISTS browser_jobs (
    id TEXT PRIMARY KEY,
    resource_key TEXT NOT NULL,
    merchant_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL CHECK (kind IN ('manual_login', 'merchant_sync', 'payment_validation')),
    priority INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed')),
    not_before TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_count INTEGER NOT NULL DEFAULT 1 CHECK (request_count > 0),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS browser_jobs_queued_coalesce_idx
    ON browser_jobs(resource_key, merchant_id, kind)
    WHERE state = 'queued';

CREATE UNIQUE INDEX IF NOT EXISTS browser_jobs_running_resource_idx
    ON browser_jobs(resource_key)
    WHERE state = 'running';

CREATE INDEX IF NOT EXISTS browser_jobs_claim_idx
    ON browser_jobs(priority DESC, not_before, requested_at, id)
    WHERE state = 'queued';

CREATE INDEX IF NOT EXISTS browser_jobs_lease_recovery_idx
    ON browser_jobs(lease_until)
    WHERE state = 'running';

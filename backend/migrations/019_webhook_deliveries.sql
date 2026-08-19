CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL REFERENCES payment_events(event_id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payment_session_id TEXT NOT NULL REFERENCES payment_sessions(id) ON DELETE CASCADE,
    invoice_id TEXT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','DELIVERING','RETRYING','DELIVERED','FAILED')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT NOT NULL DEFAULT '',
    last_status_code INTEGER NOT NULL DEFAULT 0,
    locked_at TIMESTAMPTZ,
    locked_by TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT webhook_deliveries_event_fk FOREIGN KEY (event_id) REFERENCES payment_events(event_id) ON DELETE CASCADE,
    CONSTRAINT webhook_deliveries_session_tenant_fk FOREIGN KEY (tenant_id,payment_session_id) REFERENCES payment_sessions(tenant_id,id) ON DELETE CASCADE,
    CONSTRAINT webhook_deliveries_identity_unique UNIQUE (tenant_id,event_id,endpoint)
);
CREATE INDEX IF NOT EXISTS webhook_deliveries_due_idx ON webhook_deliveries(status,next_attempt_at,created_at);
CREATE INDEX IF NOT EXISTS webhook_deliveries_lease_idx ON webhook_deliveries(status,locked_at);

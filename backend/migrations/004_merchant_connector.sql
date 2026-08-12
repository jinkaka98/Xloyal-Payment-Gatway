ALTER TABLE tenants ADD COLUMN IF NOT EXISTS merchant_id TEXT;

CREATE TABLE IF NOT EXISTS merchant_ids (
    id TEXT PRIMARY KEY,
    interactive_merchant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    credential_ciphertext TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS merchant_connections (
    merchant_id TEXT PRIMARY KEY REFERENCES merchant_ids(id) ON DELETE CASCADE,
    session_ciphertext TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('disconnected', 'connected', 'expired', 'reconnect_required')),
    last_synced_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS portal_transactions (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchant_ids(id) ON DELETE CASCADE,
    tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL,
    reference TEXT NOT NULL,
    amount BIGINT NOT NULL,
    status TEXT NOT NULL,
    paid_at TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL,
    match_confidence TEXT NOT NULL,
    invoice_id TEXT REFERENCES invoices(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (merchant_id, reference)
);

CREATE INDEX IF NOT EXISTS portal_transactions_merchant_paid_at_idx ON portal_transactions (merchant_id, paid_at DESC);
CREATE INDEX IF NOT EXISTS portal_transactions_tenant_paid_at_idx ON portal_transactions (tenant_id, paid_at DESC);

CREATE TABLE IF NOT EXISTS tariffs (
    merchant_id TEXT PRIMARY KEY REFERENCES merchant_ids(id) ON DELETE CASCADE,
    basis_points BIGINT NOT NULL DEFAULT 0,
    fixed_fee BIGINT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL
);

BEGIN;

CREATE TABLE merchant_ids (
    id text PRIMARY KEY,
    interactive_merchant_id text NOT NULL UNIQUE,
    name text NOT NULL,
    credential_ciphertext text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS merchant_id text REFERENCES merchant_ids(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS tenants_merchant_id_idx ON tenants(merchant_id);

CREATE TABLE merchant_connections (
    merchant_id text PRIMARY KEY REFERENCES merchant_ids(id) ON DELETE CASCADE,
    session_ciphertext text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('disconnected','connected','expired','reconnect_required')),
    last_synced_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchant_tariffs (
    merchant_id text PRIMARY KEY REFERENCES merchant_ids(id) ON DELETE CASCADE,
    basis_points bigint NOT NULL DEFAULT 0 CHECK (basis_points >= 0),
    fixed_fee bigint NOT NULL DEFAULT 0 CHECK (fixed_fee >= 0),
    active boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE portal_transactions (
    id text PRIMARY KEY,
    merchant_id text NOT NULL REFERENCES merchant_ids(id) ON DELETE RESTRICT,
    tenant_id text REFERENCES tenants(id) ON DELETE SET NULL,
    reference text NOT NULL,
    amount bigint NOT NULL CHECK (amount > 0),
    status text NOT NULL,
    paid_at timestamptz NOT NULL,
    source text NOT NULL,
    match_confidence text NOT NULL,
    invoice_id text REFERENCES invoices(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(merchant_id, reference)
);
CREATE INDEX portal_transactions_merchant_paid_idx ON portal_transactions(merchant_id, paid_at DESC);
CREATE INDEX portal_transactions_tenant_paid_idx ON portal_transactions(tenant_id, paid_at DESC);

COMMIT;

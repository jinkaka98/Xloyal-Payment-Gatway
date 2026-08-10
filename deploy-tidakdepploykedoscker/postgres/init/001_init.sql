BEGIN;

CREATE TABLE tenants (
    id text PRIMARY KEY,
    name text NOT NULL,
    api_key_hash text NOT NULL UNIQUE,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchant_accounts (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    provider text NOT NULL,
    name text NOT NULL,
    credential_ciphertext text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id)
);

CREATE TABLE invoices (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    merchant_account_id text NOT NULL,
    idempotency_key text NOT NULL,
    amount bigint NOT NULL CHECK (amount > 0),
    currency text NOT NULL DEFAULT 'IDR',
    description text NOT NULL DEFAULT '',
    provider_reference text NOT NULL DEFAULT '',
    provider_request_date text NOT NULL DEFAULT '',
    qr_payload text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('creating', 'pending', 'paid', 'expired', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_checked_at timestamptz,
    check_count integer NOT NULL DEFAULT 0 CHECK (check_count >= 0),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, merchant_account_id)
        REFERENCES merchant_accounts(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX invoices_pending_poll_idx
    ON invoices (last_checked_at, created_at) WHERE status = 'pending';
CREATE INDEX invoices_tenant_created_idx ON invoices (tenant_id, created_at DESC);

CREATE TABLE audit_events (
    id text PRIMARY KEY,
    tenant_id text REFERENCES tenants(id) ON DELETE SET NULL,
    actor text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_tenant_created_idx ON audit_events (tenant_id, created_at DESC);

CREATE TABLE qris_templates (
    id text PRIMARY KEY,
    name text NOT NULL,
    static_payload text NOT NULL,
    image_mime text NOT NULL,
    image_data bytea NOT NULL,
    merchant_name text NOT NULL DEFAULT '',
    merchant_city text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE test_payments (
    id text PRIMARY KEY,
    qris_template_id text NOT NULL REFERENCES qris_templates(id) ON DELETE RESTRICT,
    amount bigint NOT NULL CHECK (amount > 0),
    dynamic_payload text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'paid', 'expired', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX test_payments_created_idx ON test_payments(created_at DESC);

COMMIT;


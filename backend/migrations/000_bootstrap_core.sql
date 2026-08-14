CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    merchant_id TEXT,
    name TEXT NOT NULL,
    site_url TEXT,
    callback_url TEXT,
    webhook_url TEXT,
    sandbox_mode BOOLEAN NOT NULL DEFAULT FALSE,
    api_key_hash TEXT NOT NULL UNIQUE,
    api_key_ciphertext TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS merchant_ids (
    id TEXT PRIMARY KEY,
    interactive_merchant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    credential_ciphertext TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS merchant_accounts (
    id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL, name TEXT NOT NULL, credential_ciphertext TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), UNIQUE (tenant_id, id)
);
CREATE TABLE IF NOT EXISTS invoices (
    id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    merchant_account_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, amount BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'IDR', description TEXT NOT NULL DEFAULT '', provider_reference TEXT NOT NULL DEFAULT '',
    provider_request_date TEXT NOT NULL DEFAULT '', qr_payload TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_checked_at TIMESTAMPTZ, check_count INTEGER NOT NULL DEFAULT 0, UNIQUE (tenant_id, idempotency_key)
);
CREATE TABLE IF NOT EXISTS merchant_connections (
    merchant_id TEXT PRIMARY KEY REFERENCES merchant_ids(id) ON DELETE CASCADE, session_ciphertext TEXT NOT NULL DEFAULT '',
    browser_credential_ciphertext TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'disconnected', last_synced_at TIMESTAMPTZ,
    history_backfilled_at TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '', updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS portal_transactions (
    id TEXT PRIMARY KEY, merchant_id TEXT NOT NULL REFERENCES merchant_ids(id) ON DELETE CASCADE,
    tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL, reference TEXT NOT NULL, amount BIGINT NOT NULL, status TEXT NOT NULL,
    paid_at TIMESTAMPTZ NOT NULL, source TEXT NOT NULL, match_confidence TEXT NOT NULL, invoice_id TEXT REFERENCES invoices(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), UNIQUE (merchant_id, reference)
);
CREATE TABLE IF NOT EXISTS tariffs (merchant_id TEXT PRIMARY KEY REFERENCES merchant_ids(id) ON DELETE CASCADE, basis_points BIGINT NOT NULL DEFAULT 0, fixed_fee BIGINT NOT NULL DEFAULT 0, active BOOLEAN NOT NULL DEFAULT TRUE, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS audit_events (id TEXT PRIMARY KEY, tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL, actor TEXT NOT NULL, action TEXT NOT NULL, resource_type TEXT NOT NULL, resource_id TEXT NOT NULL, metadata JSONB NOT NULL DEFAULT '{}'::JSONB, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS qris_templates (id TEXT PRIMARY KEY, tenant_id TEXT REFERENCES tenants(id) ON DELETE RESTRICT, name TEXT NOT NULL, static_payload TEXT NOT NULL, image_mime TEXT NOT NULL, image_data BYTEA NOT NULL, merchant_name TEXT NOT NULL DEFAULT '', merchant_city TEXT NOT NULL DEFAULT '', access_scope TEXT NOT NULL DEFAULT 'all_tenants', static_to_dynamic BOOLEAN NOT NULL DEFAULT FALSE, max_requests_per_minute INTEGER NOT NULL DEFAULT 60, active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS test_payments (id TEXT PRIMARY KEY, qris_template_id TEXT NOT NULL REFERENCES qris_templates(id) ON DELETE RESTRICT, merchant_id TEXT REFERENCES merchant_ids(id) ON DELETE SET NULL, tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL, amount BIGINT NOT NULL CHECK (amount > 0), dynamic_payload TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', request_source TEXT NOT NULL DEFAULT 'admin', match_confidence TEXT NOT NULL DEFAULT 'pending', matched_transaction_id TEXT REFERENCES portal_transactions(id) ON DELETE SET NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_checked_at TIMESTAMPTZ, next_check_at TIMESTAMPTZ, check_count INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS qris_template_rate_limits (template_id TEXT NOT NULL REFERENCES qris_templates(id) ON DELETE CASCADE, tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE, window_started_at TIMESTAMPTZ NOT NULL, request_count INTEGER NOT NULL, PRIMARY KEY (template_id, tenant_id));

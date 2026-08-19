-- Hosted payment foundation (Phase 2).
--
-- This migration only adds durable storage.  Payment/session transitions and
-- event/outbox insertion must be performed by the application in one DB
-- transaction.  No existing invoice or QRIS state is changed here.

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS webhook_secret_ciphertext TEXT;

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS webhook_replay_window_seconds INTEGER NOT NULL DEFAULT 300;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'invoices_tenant_id_id_unique') THEN
        ALTER TABLE invoices ADD CONSTRAINT invoices_tenant_id_id_unique UNIQUE (tenant_id, id);
    END IF;
END;
$$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenants_webhook_replay_window_check') THEN
        ALTER TABLE tenants ADD CONSTRAINT tenants_webhook_replay_window_check
            CHECK (webhook_replay_window_seconds BETWEEN 60 AND 3600);
    END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS payment_themes (
    id TEXT PRIMARY KEY,
    tenant_id TEXT REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT',
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_themes_status_check
        CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED'))
);

CREATE INDEX IF NOT EXISTS payment_themes_tenant_status_idx
    ON payment_themes(tenant_id, status, updated_at DESC);

CREATE OR REPLACE FUNCTION payment_theme_object_has_only_keys(value JSONB, allowed_keys TEXT[])
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    key_name TEXT;
BEGIN
    IF value IS NULL OR jsonb_typeof(value) <> 'object' THEN RETURN FALSE; END IF;
    FOR key_name IN SELECT jsonb_object_keys(value) LOOP
        IF NOT (key_name = ANY(allowed_keys)) THEN RETURN FALSE; END IF;
    END LOOP;
    RETURN TRUE;
END;
$$;

CREATE OR REPLACE FUNCTION payment_theme_config_is_valid(config JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    key_name TEXT;
    template_key TEXT;
BEGIN
    IF jsonb_typeof(config) <> 'object' THEN RETURN FALSE; END IF;
    FOR key_name IN SELECT jsonb_object_keys(config) LOOP
        IF key_name NOT IN ('schema_version', 'template_key', 'branding', 'colors', 'layout', 'payment_visibility', 'timer', 'success_copy', 'redirect_delay') THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    IF config ? 'schema_version' AND jsonb_typeof(config->'schema_version') <> 'number' THEN RETURN FALSE; END IF;
    IF config ? 'template_key' THEN
        IF jsonb_typeof(config->'template_key') <> 'string' THEN RETURN FALSE; END IF;
        template_key := config->>'template_key';
        IF template_key NOT IN ('modern', 'minimal', 'dark', 'corporate', 'compact') THEN RETURN FALSE; END IF;
    END IF;
    IF config ? 'branding' AND jsonb_typeof(config->'branding') <> 'object' THEN RETURN FALSE; END IF;
    IF config ? 'colors' AND jsonb_typeof(config->'colors') <> 'object' THEN RETURN FALSE; END IF;
    IF config ? 'layout' AND jsonb_typeof(config->'layout') <> 'object' THEN RETURN FALSE; END IF;
    IF config ? 'payment_visibility' AND jsonb_typeof(config->'payment_visibility') <> 'object' THEN RETURN FALSE; END IF;
    IF config ? 'timer' AND jsonb_typeof(config->'timer') <> 'object' THEN RETURN FALSE; END IF;
    IF config ? 'success_copy' AND jsonb_typeof(config->'success_copy') <> 'object' THEN RETURN FALSE; END IF;
    IF config ? 'branding' AND NOT payment_theme_object_has_only_keys(config->'branding', ARRAY['display_name','logo_url','tagline']) THEN RETURN FALSE; END IF;
    IF config ? 'colors' AND NOT payment_theme_object_has_only_keys(config->'colors', ARRAY['primary','background','surface','text','muted','success','danger']) THEN RETURN FALSE; END IF;
    IF config ? 'layout' AND NOT payment_theme_object_has_only_keys(config->'layout', ARRAY['max_width','radius','density']) THEN RETURN FALSE; END IF;
    IF config ? 'payment_visibility' AND NOT payment_theme_object_has_only_keys(config->'payment_visibility', ARRAY['show_qr','show_amount','show_description','show_reference']) THEN RETURN FALSE; END IF;
    IF config ? 'timer' AND NOT payment_theme_object_has_only_keys(config->'timer', ARRAY['enabled','warning_seconds']) THEN RETURN FALSE; END IF;
    IF config ? 'success_copy' AND NOT payment_theme_object_has_only_keys(config->'success_copy', ARRAY['title','message']) THEN RETURN FALSE; END IF;
    IF config ? 'branding' THEN
        IF EXISTS (SELECT 1 FROM jsonb_each(config->'branding') p WHERE jsonb_typeof(p.value) <> 'string') THEN RETURN FALSE; END IF;
        IF config->'branding' ? 'logo_url' AND (config->'branding'->>'logo_url' !~ '^https://[^[:space:]<>]+$') THEN RETURN FALSE; END IF;
        IF EXISTS (SELECT 1 FROM jsonb_each_text(config->'branding') p WHERE p.value ~ '[<>]') THEN RETURN FALSE; END IF;
    END IF;
    IF config ? 'colors' AND EXISTS (SELECT 1 FROM jsonb_each_text(config->'colors') p WHERE p.value !~ '^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$') THEN RETURN FALSE; END IF;
    IF config ? 'layout' THEN
        IF config->'layout' ? 'max_width' AND jsonb_typeof(config->'layout'->'max_width') <> 'number' THEN RETURN FALSE; END IF;
        IF config->'layout' ? 'radius' AND jsonb_typeof(config->'layout'->'radius') <> 'number' THEN RETURN FALSE; END IF;
        IF config->'layout' ? 'density' AND (jsonb_typeof(config->'layout'->'density') <> 'string' OR config->'layout'->>'density' NOT IN ('comfortable','compact')) THEN RETURN FALSE; END IF;
    END IF;
    IF config ? 'payment_visibility' AND EXISTS (SELECT 1 FROM jsonb_each(config->'payment_visibility') p WHERE jsonb_typeof(p.value) <> 'boolean') THEN RETURN FALSE; END IF;
    IF config ? 'timer' THEN
        IF config->'timer' ? 'enabled' AND jsonb_typeof(config->'timer'->'enabled') <> 'boolean' THEN RETURN FALSE; END IF;
        IF config->'timer' ? 'warning_seconds' AND jsonb_typeof(config->'timer'->'warning_seconds') <> 'number' THEN RETURN FALSE; END IF;
        IF config->'timer' ? 'warning_seconds' AND (config->'timer'->>'warning_seconds')::numeric < 0 THEN RETURN FALSE; END IF;
    END IF;
    IF config ? 'success_copy' THEN
        IF EXISTS (SELECT 1 FROM jsonb_each(config->'success_copy') p WHERE jsonb_typeof(p.value) <> 'string') THEN RETURN FALSE; END IF;
        IF EXISTS (SELECT 1 FROM jsonb_each_text(config->'success_copy') p WHERE p.value ~ '[<>]') THEN RETURN FALSE; END IF;
    END IF;
    IF config ? 'redirect_delay' AND (jsonb_typeof(config->'redirect_delay') <> 'number' OR (config->>'redirect_delay')::numeric < 0 OR (config->>'redirect_delay')::numeric > 30) THEN RETURN FALSE; END IF;
    RETURN TRUE;
END;
$$;

-- A version is an immutable JSON snapshot.  Publishing creates the next
-- version; application code must never update an existing version row.
CREATE TABLE IF NOT EXISTS payment_theme_versions (
    id TEXT PRIMARY KEY,
    theme_id TEXT NOT NULL REFERENCES payment_themes(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    config JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PUBLISHED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_theme_versions_status_check
        CHECK (status IN ('PUBLISHED', 'ARCHIVED')),
    CONSTRAINT payment_theme_versions_config_size_check
        CHECK (octet_length(config::TEXT) <= 65536),
    CONSTRAINT payment_theme_versions_config_schema_check
        CHECK (payment_theme_config_is_valid(config)),
    CONSTRAINT payment_theme_versions_theme_version_unique
        UNIQUE (theme_id, version)
);

CREATE INDEX IF NOT EXISTS payment_theme_versions_published_idx
    ON payment_theme_versions(theme_id, version DESC)
    WHERE status = 'PUBLISHED';

CREATE OR REPLACE FUNCTION prevent_payment_theme_version_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'payment_theme_versions are immutable';
    END IF;
    IF NEW.id <> OLD.id
        OR NEW.theme_id <> OLD.theme_id
        OR NEW.version <> OLD.version
        OR NEW.schema_version <> OLD.schema_version
        OR NEW.config <> OLD.config
        OR NEW.created_at <> OLD.created_at
        OR OLD.status <> 'PUBLISHED'
        OR NEW.status <> 'ARCHIVED' THEN
        RAISE EXCEPTION 'payment_theme_versions are immutable';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS payment_theme_versions_immutable ON payment_theme_versions;
CREATE TRIGGER payment_theme_versions_immutable
    BEFORE UPDATE OR DELETE ON payment_theme_versions
    FOR EACH ROW EXECUTE FUNCTION prevent_payment_theme_version_mutation();

INSERT INTO payment_themes(id, tenant_id, name, status, is_default)
VALUES ('system-default-theme', NULL, 'System Default', 'PUBLISHED', TRUE)
ON CONFLICT (id) DO NOTHING;

INSERT INTO payment_theme_versions(id, theme_id, version, schema_version, config, status)
VALUES ('system-default-theme-v1', 'system-default-theme', 1, 1,
        '{"schema_version":1,"template_key":"modern"}'::JSONB, 'PUBLISHED')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS tenant_allowed_redirect_urls (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    type TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT tenant_allowed_redirect_urls_type_check
        CHECK (type IN ('SUCCESS', 'CANCEL', 'FAILED', 'EXPIRED')),
    CONSTRAINT tenant_allowed_redirect_urls_url_not_blank
        CHECK (length(btrim(url)) > 0),
    CONSTRAINT tenant_allowed_redirect_urls_unique
        UNIQUE (tenant_id, type, url)
);

CREATE INDEX IF NOT EXISTS tenant_allowed_redirect_urls_lookup_idx
    ON tenant_allowed_redirect_urls(tenant_id, type, active);

CREATE TABLE IF NOT EXISTS payment_sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    invoice_id TEXT NOT NULL,
    public_token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'OPEN',
    theme_id TEXT REFERENCES payment_themes(id) ON DELETE RESTRICT,
    theme_version INTEGER,
    return_url TEXT,
    success_url TEXT,
    cancel_url TEXT,
    failed_url TEXT,
    expired_url TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_sessions_status_check
        CHECK (status IN ('OPEN', 'PAYMENT_PENDING', 'PAID', 'CANCELLED',
                          'EXPIRED', 'FAILED', 'REDIRECTING', 'CLOSED')),
    CONSTRAINT payment_sessions_theme_version_pair_check
        CHECK ((theme_id IS NULL AND theme_version IS NULL)
            OR (theme_id IS NOT NULL AND theme_version IS NOT NULL AND theme_version > 0)),
    CONSTRAINT payment_sessions_public_token_hash_not_blank
        CHECK (length(btrim(public_token_hash)) > 0),
    CONSTRAINT payment_sessions_tenant_invoice_fk
        FOREIGN KEY (tenant_id, invoice_id) REFERENCES invoices(tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payment_sessions_tenant_id_unique
        UNIQUE (tenant_id, id),
    CONSTRAINT payment_sessions_expiry_check
        CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS payment_sessions_tenant_status_idx
    ON payment_sessions(tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS payment_sessions_invoice_idx
    ON payment_sessions(invoice_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS payment_sessions_one_active_per_invoice_idx
    ON payment_sessions(tenant_id, invoice_id)
    WHERE status IN ('OPEN', 'PAYMENT_PENDING');
CREATE INDEX IF NOT EXISTS payment_sessions_expiry_idx
    ON payment_sessions(expires_at)
    WHERE status IN ('OPEN', 'PAYMENT_PENDING');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'payment_sessions_theme_snapshot_fk'
    ) THEN
        ALTER TABLE payment_sessions
            ADD CONSTRAINT payment_sessions_theme_snapshot_fk
            FOREIGN KEY (theme_id, theme_version)
            REFERENCES payment_theme_versions(theme_id, version)
            ON DELETE RESTRICT;
    END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS payment_events (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    invoice_id TEXT NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    payment_session_id TEXT NOT NULL REFERENCES payment_sessions(id) ON DELETE CASCADE,
    sequence_number BIGINT NOT NULL CHECK (sequence_number > 0),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_events_type_check
        CHECK (event_type IN ('payment.created', 'payment.pending', 'payment.verifying',
                              'payment.paid', 'payment.failed', 'payment.expired',
                              'payment.cancelled', 'payment.redirecting', 'payment.closed')),
    CONSTRAINT payment_events_sequence_unique
        UNIQUE (payment_session_id, sequence_number),
    CONSTRAINT payment_events_tenant_invoice_fk
        FOREIGN KEY (tenant_id, invoice_id)
        REFERENCES invoices(tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT payment_events_tenant_session_fk
        FOREIGN KEY (tenant_id, payment_session_id)
        REFERENCES payment_sessions(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS payment_events_session_order_idx
    ON payment_events(payment_session_id, sequence_number);
CREATE INDEX IF NOT EXISTS payment_events_invoice_idx
    ON payment_events(invoice_id, sequence_number);
CREATE INDEX IF NOT EXISTS payment_events_tenant_time_idx
    ON payment_events(tenant_id, occurred_at DESC);

-- Payment events are an append-only audit stream.  The trigger protects the
-- invariant at the database boundary as well as in repository code.
CREATE OR REPLACE FUNCTION prevent_payment_event_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'payment_events are immutable';
END;
$$;

DROP TRIGGER IF EXISTS payment_events_immutable ON payment_events;
CREATE TRIGGER payment_events_immutable
    BEFORE UPDATE OR DELETE ON payment_events
    FOR EACH ROW EXECUTE FUNCTION prevent_payment_event_mutation();

CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE REFERENCES payment_events(event_id) ON DELETE RESTRICT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ,
    locked_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT outbox_events_status_check
        CHECK (status IN ('PENDING', 'PROCESSING', 'DELIVERED', 'FAILED')),
    CONSTRAINT outbox_events_lock_consistency_check
        CHECK ((status = 'PROCESSING' AND locked_at IS NOT NULL)
            OR status <> 'PROCESSING')
);

CREATE INDEX IF NOT EXISTS outbox_events_claim_idx
    ON outbox_events(next_attempt_at, created_at, id)
    WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS outbox_events_lease_recovery_idx
    ON outbox_events(locked_at)
    WHERE status = 'PROCESSING';
CREATE INDEX IF NOT EXISTS outbox_events_aggregate_idx
    ON outbox_events(aggregate_type, aggregate_id, created_at);
CREATE INDEX IF NOT EXISTS outbox_events_tenant_idx
    ON outbox_events(tenant_id, created_at);

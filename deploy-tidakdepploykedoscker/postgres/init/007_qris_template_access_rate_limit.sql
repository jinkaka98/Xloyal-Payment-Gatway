ALTER TABLE qris_templates ADD COLUMN IF NOT EXISTS access_scope text NOT NULL DEFAULT 'all_tenants' CHECK (access_scope IN ('all_tenants','selected_tenant'));
ALTER TABLE qris_templates ADD COLUMN IF NOT EXISTS max_requests_per_minute integer NOT NULL DEFAULT 60 CHECK (max_requests_per_minute BETWEEN 1 AND 10000);

CREATE TABLE IF NOT EXISTS qris_template_rate_limits (
    template_id text NOT NULL REFERENCES qris_templates(id) ON DELETE CASCADE,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    window_started_at timestamptz NOT NULL,
    request_count integer NOT NULL,
    PRIMARY KEY (template_id, tenant_id)
);

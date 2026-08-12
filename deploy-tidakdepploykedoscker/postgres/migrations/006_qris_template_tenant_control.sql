ALTER TABLE qris_templates ADD COLUMN IF NOT EXISTS tenant_id text REFERENCES tenants(id) ON DELETE RESTRICT;
ALTER TABLE qris_templates ADD COLUMN IF NOT EXISTS static_to_dynamic boolean NOT NULL DEFAULT false;
ALTER TABLE qris_templates ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;
CREATE INDEX IF NOT EXISTS qris_templates_tenant_created_idx ON qris_templates(tenant_id, created_at DESC);

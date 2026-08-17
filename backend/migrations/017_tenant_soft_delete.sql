ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS tenants_active_directory_idx
    ON tenants(created_at DESC)
    WHERE deleted_at IS NULL;

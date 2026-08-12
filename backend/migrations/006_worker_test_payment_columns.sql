ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS merchant_id TEXT REFERENCES merchant_ids(id) ON DELETE SET NULL;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS request_source TEXT NOT NULL DEFAULT 'admin';
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS match_confidence TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS matched_transaction_id TEXT REFERENCES portal_transactions(id) ON DELETE SET NULL;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS next_check_at TIMESTAMPTZ;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS check_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS test_payments_pending_next_check_idx ON test_payments (next_check_at) WHERE status = 'pending';

BEGIN;

ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS merchant_id text REFERENCES merchant_ids(id) ON DELETE SET NULL;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS tenant_id text REFERENCES tenants(id) ON DELETE SET NULL;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS request_source text NOT NULL DEFAULT 'admin_qris_test';
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS match_confidence text NOT NULL DEFAULT 'awaiting_browser_sync';
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS matched_transaction_id text REFERENCES portal_transactions(id) ON DELETE SET NULL;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS last_checked_at timestamptz;
ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS check_count integer NOT NULL DEFAULT 0 CHECK (check_count >= 0);

UPDATE test_payments payment
SET tenant_id = template.tenant_id
FROM qris_templates template
WHERE payment.qris_template_id = template.id AND payment.tenant_id IS NULL AND template.tenant_id IS NOT NULL;

UPDATE test_payments payment
SET merchant_id = tenant.merchant_id
FROM tenants tenant
WHERE payment.tenant_id = tenant.id AND payment.merchant_id IS NULL AND tenant.merchant_id IS NOT NULL;

UPDATE test_payments
SET merchant_id = (SELECT MIN(id) FROM merchant_ids WHERE active = true)
WHERE merchant_id IS NULL AND (SELECT COUNT(*) FROM merchant_ids WHERE active = true) = 1;

CREATE INDEX IF NOT EXISTS test_payments_pending_check_idx
    ON test_payments(last_checked_at, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS test_payments_merchant_created_idx
    ON test_payments(merchant_id, created_at DESC);

COMMIT;

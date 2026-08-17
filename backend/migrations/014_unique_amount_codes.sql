ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS use_unique_amount_code BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE test_payments
    ADD COLUMN IF NOT EXISTS payable_amount BIGINT;

UPDATE test_payments
SET payable_amount = amount
WHERE payable_amount IS NULL;

ALTER TABLE test_payments
    ALTER COLUMN payable_amount SET NOT NULL;

ALTER TABLE test_payments
    ADD COLUMN IF NOT EXISTS unique_amount_code SMALLINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'test_payments_unique_amount_code_range'
          AND conrelid = 'test_payments'::regclass
    ) THEN
        ALTER TABLE test_payments
            ADD CONSTRAINT test_payments_unique_amount_code_range
            CHECK (unique_amount_code BETWEEN 0 AND 99);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS qris_unique_amount_reservations (
    payment_id TEXT PRIMARY KEY REFERENCES test_payments(id) ON DELETE CASCADE,
    merchant_id TEXT NOT NULL REFERENCES merchant_ids(id) ON DELETE CASCADE,
    payable_amount BIGINT NOT NULL CHECK (payable_amount > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT qris_unique_amount_reservations_merchant_payable_key UNIQUE (merchant_id, payable_amount)
);

CREATE INDEX IF NOT EXISTS qris_unique_amount_reservations_expiry_idx
    ON qris_unique_amount_reservations(expires_at);

CREATE INDEX IF NOT EXISTS test_payments_pending_payable_lookup_idx
    ON test_payments(merchant_id, payable_amount, expires_at)
    WHERE status = 'pending';

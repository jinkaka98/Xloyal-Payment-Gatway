ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS unique_amount_cooldown_minutes SMALLINT NOT NULL DEFAULT 30;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'tenants_unique_amount_cooldown_range'
          AND conrelid = 'tenants'::regclass
    ) THEN
        ALTER TABLE tenants
            ADD CONSTRAINT tenants_unique_amount_cooldown_range
            CHECK (unique_amount_cooldown_minutes BETWEEN 30 AND 60);
    END IF;
END $$;

ALTER TABLE qris_unique_amount_reservations
    ADD COLUMN IF NOT EXISTS tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS unique_amount_code SMALLINT,
    ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS reserved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS cooldown_minutes SMALLINT NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS terminal_status TEXT,
    ADD COLUMN IF NOT EXISTS terminal_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cooldown_until TIMESTAMPTZ;

UPDATE qris_unique_amount_reservations AS reservation
SET unique_amount_code = payment.unique_amount_code,
    tenant_id = payment.tenant_id,
    reserved_at = payment.created_at,
    cooldown_minutes = COALESCE(tenant.unique_amount_cooldown_minutes, 30)
FROM test_payments AS payment
LEFT JOIN tenants AS tenant ON tenant.id = payment.tenant_id
WHERE reservation.payment_id = payment.id
  AND reservation.unique_amount_code IS NULL;

ALTER TABLE qris_unique_amount_reservations
    ALTER COLUMN unique_amount_code SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'qris_unique_amount_reservations_state_check'
          AND conrelid = 'qris_unique_amount_reservations'::regclass
    ) THEN
        ALTER TABLE qris_unique_amount_reservations
            ADD CONSTRAINT qris_unique_amount_reservations_state_check
            CHECK (state IN ('active', 'cooldown'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'qris_unique_amount_reservations_cooldown_range'
          AND conrelid = 'qris_unique_amount_reservations'::regclass
    ) THEN
        ALTER TABLE qris_unique_amount_reservations
            ADD CONSTRAINT qris_unique_amount_reservations_cooldown_range
            CHECK (cooldown_minutes BETWEEN 30 AND 60);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS qris_unique_amount_reservations_cooldown_idx
    ON qris_unique_amount_reservations(cooldown_until)
    WHERE state = 'cooldown';

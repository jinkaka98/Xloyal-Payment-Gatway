ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS requested_amount BIGINT,
    ADD COLUMN IF NOT EXISTS unique_amount_code SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS qris_template_id TEXT REFERENCES qris_templates(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS qris_merchant_id TEXT REFERENCES merchant_ids(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS qris_merchant_name TEXT,
    ADD COLUMN IF NOT EXISTS qris_merchant_city TEXT;

UPDATE invoices SET requested_amount = amount WHERE requested_amount IS NULL;
ALTER TABLE invoices ALTER COLUMN requested_amount SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'invoices_unique_amount_code_range'
          AND conrelid = 'invoices'::regclass
    ) THEN
        ALTER TABLE invoices ADD CONSTRAINT invoices_unique_amount_code_range
            CHECK (unique_amount_code BETWEEN 0 AND 99);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS hosted_invoice_unique_amount_reservations (
    invoice_id TEXT PRIMARY KEY REFERENCES invoices(id) ON DELETE CASCADE,
    tenant_id TEXT REFERENCES tenants(id) ON DELETE SET NULL,
    merchant_id TEXT NOT NULL REFERENCES merchant_ids(id) ON DELETE CASCADE,
    payable_amount BIGINT NOT NULL CHECK (payable_amount > 0),
    unique_amount_code SMALLINT NOT NULL CHECK (unique_amount_code BETWEEN 1 AND 99),
    expires_at TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active','cooldown')),
    reserved_at TIMESTAMPTZ NOT NULL,
    cooldown_minutes SMALLINT NOT NULL DEFAULT 30 CHECK (cooldown_minutes BETWEEN 30 AND 60),
    terminal_status TEXT,
    terminal_at TIMESTAMPTZ,
    cooldown_until TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS hosted_invoice_unique_amount_merchant_payable_key
    ON hosted_invoice_unique_amount_reservations(merchant_id, payable_amount);

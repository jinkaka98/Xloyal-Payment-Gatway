DO $$
DECLARE
    invoices_already_snapshotted BOOLEAN;
    payments_already_snapshotted BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'invoices'::regclass
          AND attname = 'sandbox_mode'
          AND NOT attisdropped
    ) INTO invoices_already_snapshotted;

    SELECT EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'test_payments'::regclass
          AND attname = 'sandbox_mode'
          AND NOT attisdropped
    ) INTO payments_already_snapshotted;

    ALTER TABLE invoices
        ADD COLUMN IF NOT EXISTS sandbox_mode BOOLEAN NOT NULL DEFAULT FALSE;
    ALTER TABLE test_payments
        ADD COLUMN IF NOT EXISTS sandbox_mode BOOLEAN NOT NULL DEFAULT FALSE;

    IF NOT invoices_already_snapshotted THEN
        UPDATE invoices AS invoice
        SET sandbox_mode = tenant.sandbox_mode
        FROM tenants AS tenant
        WHERE invoice.tenant_id = tenant.id;
    END IF;

    IF NOT payments_already_snapshotted THEN
        UPDATE test_payments AS payment
        SET sandbox_mode = tenant.sandbox_mode
        FROM tenants AS tenant
        WHERE payment.tenant_id = tenant.id
          AND payment.request_source = 'tenant_api';
    END IF;
END $$;

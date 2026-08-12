BEGIN;

ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS next_check_at timestamptz;

-- Existing pending rows should keep the old behavior while moving to the
-- persisted scheduler state. New rows are initialized by the API.
UPDATE test_payments
SET next_check_at = COALESCE(last_checked_at + interval '30 seconds', created_at + interval '30 seconds')
WHERE status = 'pending' AND next_check_at IS NULL;

CREATE INDEX IF NOT EXISTS test_payments_next_check_idx
    ON test_payments(next_check_at, created_at) WHERE status = 'pending';

COMMIT;

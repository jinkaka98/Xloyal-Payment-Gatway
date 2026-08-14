ALTER TABLE test_payments ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

WITH ranked_matches AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY matched_transaction_id ORDER BY updated_at, id) AS match_rank
    FROM test_payments
    WHERE matched_transaction_id IS NOT NULL
)
UPDATE test_payments AS payment
SET matched_transaction_id = NULL,
    match_confidence = 'legacy_duplicate_match'
FROM ranked_matches AS ranked
WHERE payment.id = ranked.id AND ranked.match_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS test_payments_tenant_idempotency_unique
    ON test_payments(tenant_id, idempotency_key)
    WHERE request_source = 'tenant_api' AND idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS test_payments_matched_transaction_unique
    ON test_payments(matched_transaction_id)
    WHERE matched_transaction_id IS NOT NULL;

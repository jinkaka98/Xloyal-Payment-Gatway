#!/bin/sh
set -eu
for migration in /migrations/007_restore_admin_feature_schema.sql /migrations/004_merchant_connector.sql /migrations/005_browser_credentials.sql /migrations/006_worker_test_payment_columns.sql /migrations/008_merchant_history_backfill.sql /migrations/009_test_payment_unique_code.sql /migrations/010_tenant_qris_transactions.sql /migrations/011_tenant_api_key_ciphertext.sql /migrations/012_tenant_sandbox_mode.sql /migrations/013_transaction_sandbox_mode.sql /migrations/014_unique_amount_codes.sql /migrations/015_unique_amount_cooldown.sql /migrations/016_browser_job_queue.sql; do
  /usr/local/bin/migrate "$migration"
done

ALTER TABLE merchant_connections
ADD COLUMN IF NOT EXISTS history_backfilled_at TIMESTAMPTZ;

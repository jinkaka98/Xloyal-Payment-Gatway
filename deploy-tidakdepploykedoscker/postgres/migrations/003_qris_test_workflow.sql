BEGIN;

CREATE TABLE IF NOT EXISTS qris_templates (
    id text PRIMARY KEY,
    name text NOT NULL,
    static_payload text NOT NULL,
    image_mime text NOT NULL,
    image_data bytea NOT NULL,
    merchant_name text NOT NULL DEFAULT '',
    merchant_city text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS test_payments (
    id text PRIMARY KEY,
    qris_template_id text NOT NULL REFERENCES qris_templates(id) ON DELETE RESTRICT,
    amount bigint NOT NULL CHECK (amount > 0),
    dynamic_payload text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'paid', 'expired', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS test_payments_created_idx ON test_payments(created_at DESC);

COMMIT;

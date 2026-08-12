ALTER TABLE merchant_connections ADD COLUMN IF NOT EXISTS browser_credential_ciphertext TEXT NOT NULL DEFAULT '';

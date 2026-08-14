ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS api_key_ciphertext TEXT;

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS site_url text;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS callback_url text;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS webhook_url text;

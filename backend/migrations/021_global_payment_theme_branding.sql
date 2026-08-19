-- Global payment themes are reusable across tenants. Tenant context is optional
-- for the editor; tenant branding is stored as an inheritance layer in config.
CREATE OR REPLACE FUNCTION payment_theme_config_is_valid(config JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    key_name TEXT;
BEGIN
    IF config IS NULL OR jsonb_typeof(config) <> 'object' THEN RETURN FALSE; END IF;
    FOR key_name IN SELECT jsonb_object_keys(config) LOOP
        IF key_name NOT IN ('schema_version', 'template_key', 'branding', 'tenant_branding', 'colors', 'layout', 'payment_visibility', 'timer', 'success_copy', 'redirect_delay') THEN RETURN FALSE; END IF;
    END LOOP;
    IF config ? 'schema_version' AND jsonb_typeof(config->'schema_version') <> 'number' THEN RETURN FALSE; END IF;
    IF config ? 'template_key' AND (jsonb_typeof(config->'template_key') <> 'string' OR config->>'template_key' NOT IN ('modern','minimal','dark','corporate','compact')) THEN RETURN FALSE; END IF;
    IF config ? 'branding' AND (jsonb_typeof(config->'branding') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'branding', ARRAY['display_name','logo_url','tagline'])) THEN RETURN FALSE; END IF;
    IF config ? 'tenant_branding' AND (jsonb_typeof(config->'tenant_branding') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'tenant_branding', ARRAY['display_name','logo_url','tagline','primary_color','favicon_url'])) THEN RETURN FALSE; END IF;
    IF config ? 'branding' AND EXISTS (SELECT 1 FROM jsonb_each(config->'branding') p WHERE jsonb_typeof(p.value) <> 'string') THEN RETURN FALSE; END IF;
    IF config ? 'tenant_branding' AND EXISTS (SELECT 1 FROM jsonb_each(config->'tenant_branding') p WHERE jsonb_typeof(p.value) <> 'string') THEN RETURN FALSE; END IF;
    IF config ? 'branding' AND config->'branding' ? 'logo_url' AND (config->'branding'->>'logo_url' !~ '^https://[^[:space:]<>]+$') THEN RETURN FALSE; END IF;
    IF config ? 'tenant_branding' AND config->'tenant_branding' ? 'logo_url' AND (config->'tenant_branding'->>'logo_url' !~ '^https://[^[:space:]<>]+$') THEN RETURN FALSE; END IF;
    IF config ? 'tenant_branding' AND config->'tenant_branding' ? 'favicon_url' AND (config->'tenant_branding'->>'favicon_url' !~ '^https://[^[:space:]<>]+$') THEN RETURN FALSE; END IF;
    IF EXISTS (SELECT 1 FROM jsonb_each_text(COALESCE(config->'branding','{}'::jsonb) || COALESCE(config->'tenant_branding','{}'::jsonb)) p WHERE p.value ~ '[<>]') THEN RETURN FALSE; END IF;
    IF config ? 'colors' AND (jsonb_typeof(config->'colors') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'colors', ARRAY['primary','background','surface','text','muted','success','danger'])) THEN RETURN FALSE; END IF;
    IF config ? 'colors' AND EXISTS (SELECT 1 FROM jsonb_each_text(config->'colors') p WHERE p.value !~ '^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$') THEN RETURN FALSE; END IF;
    IF config ? 'layout' AND (jsonb_typeof(config->'layout') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'layout', ARRAY['max_width','radius','density'])) THEN RETURN FALSE; END IF;
    IF config ? 'payment_visibility' AND (jsonb_typeof(config->'payment_visibility') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'payment_visibility', ARRAY['show_qr','show_amount','show_description','show_reference'])) THEN RETURN FALSE; END IF;
    IF config ? 'timer' AND (jsonb_typeof(config->'timer') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'timer', ARRAY['enabled','warning_seconds'])) THEN RETURN FALSE; END IF;
    IF config ? 'success_copy' AND (jsonb_typeof(config->'success_copy') <> 'object' OR NOT payment_theme_object_has_only_keys(config->'success_copy', ARRAY['title','message'])) THEN RETURN FALSE; END IF;
    RETURN TRUE;
END;
$$;

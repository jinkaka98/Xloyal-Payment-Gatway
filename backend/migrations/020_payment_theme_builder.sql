ALTER TABLE payment_themes ADD COLUMN IF NOT EXISTS current_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_themes ADD COLUMN IF NOT EXISTS draft_config JSONB;
CREATE UNIQUE INDEX IF NOT EXISTS payment_themes_one_tenant_default_idx ON payment_themes(tenant_id) WHERE is_default=TRUE AND tenant_id IS NOT NULL;
UPDATE payment_themes t SET current_version=COALESCE((SELECT MAX(tv.version) FROM payment_theme_versions tv WHERE tv.theme_id=t.id AND tv.status='PUBLISHED'),0);

INSERT INTO payment_themes(id,tenant_id,name,status,is_default,current_version) VALUES
('system-modern-theme',NULL,'Modern','PUBLISHED',FALSE,1),('system-minimal-theme',NULL,'Minimal','PUBLISHED',FALSE,1),('system-dark-theme',NULL,'Dark','PUBLISHED',FALSE,1),('system-corporate-theme',NULL,'Corporate','PUBLISHED',FALSE,1),('system-compact-theme',NULL,'Compact','PUBLISHED',FALSE,1) ON CONFLICT(id) DO NOTHING;
INSERT INTO payment_theme_versions(id,theme_id,version,schema_version,config,status) VALUES
('system-modern-theme-v1','system-modern-theme',1,1,'{"schema_version":1,"template_key":"modern","colors":{"primary":"#1A5C55","background":"#F4F6F2","surface":"#FFFFFF","text":"#18231F","muted":"#68766F","success":"#16805B","danger":"#B44444"}}','PUBLISHED'),
('system-minimal-theme-v1','system-minimal-theme',1,1,'{"schema_version":1,"template_key":"minimal","colors":{"primary":"#202522","background":"#FFFFFF","surface":"#FFFFFF","text":"#171A18","muted":"#6A716D","success":"#147A4D","danger":"#B43B3B"}}','PUBLISHED'),
('system-dark-theme-v1','system-dark-theme',1,1,'{"schema_version":1,"template_key":"dark","colors":{"primary":"#6FCFAD","background":"#141816","surface":"#202622","text":"#F4F7F5","muted":"#AAB4AE","success":"#69D5A2","danger":"#FF8A8A"}}','PUBLISHED'),
('system-corporate-theme-v1','system-corporate-theme',1,1,'{"schema_version":1,"template_key":"corporate","colors":{"primary":"#185FA5","background":"#F3F6F9","surface":"#FFFFFF","text":"#17212B","muted":"#637180","success":"#17804F","danger":"#B83D45"}}','PUBLISHED'),
('system-compact-theme-v1','system-compact-theme',1,1,'{"schema_version":1,"template_key":"compact","layout":{"max_width":400,"radius":6,"density":"compact"},"colors":{"primary":"#245C47","background":"#F2F4F1","surface":"#FFFFFF","text":"#18201C","muted":"#657069","success":"#147A4D","danger":"#B43B3B"}}','PUBLISHED') ON CONFLICT(id) DO NOTHING;

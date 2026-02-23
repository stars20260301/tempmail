-- ============================================================
-- TempMail v3 迁移 — 域名状态字段 + 丰富系统设置
-- ============================================================

-- 1. domains 表增加 status 列: pending | active | disabled
ALTER TABLE domains ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'active';
-- 同步现有数据
UPDATE domains SET status = CASE WHEN is_active THEN 'active' ELSE 'disabled' END;

-- 2. 新增 mx_checked_at: 上次 MX 检测时间
ALTER TABLE domains ADD COLUMN IF NOT EXISTS mx_checked_at TIMESTAMPTZ;

-- 3. 索引：快速查找 pending 域名
CREATE INDEX IF NOT EXISTS idx_domains_status ON domains (status) WHERE status = 'pending';

-- 4. 补充系统设置
INSERT INTO app_settings (key, value) VALUES ('site_title',            'TempMail')          ON CONFLICT DO NOTHING;
INSERT INTO app_settings (key, value) VALUES ('default_domain',        '')                  ON CONFLICT DO NOTHING;
INSERT INTO app_settings (key, value) VALUES ('mailbox_ttl_minutes',   '30')                ON CONFLICT DO NOTHING;
INSERT INTO app_settings (key, value) VALUES ('announcement',          '')                  ON CONFLICT DO NOTHING;
INSERT INTO app_settings (key, value) VALUES ('max_mailboxes_per_user','100')               ON CONFLICT DO NOTHING;

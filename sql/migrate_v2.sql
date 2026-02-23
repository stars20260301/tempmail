-- ============================================================
-- TempMail v2 迁移 — 为现有数据库添加 expires_at 字段
-- 仅在已有数据库上运行，全新部署使用 init.sql 即可
-- ============================================================

-- 1. 添加 expires_at 列（已有邮箱设置为 30 分钟后过期）
ALTER TABLE mailboxes
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 minutes';

-- 2. 新增系统设置项
INSERT INTO app_settings (key, value) VALUES ('smtp_server_ip', '') ON CONFLICT DO NOTHING;
INSERT INTO app_settings (key, value) VALUES ('mailbox_ttl_minutes',  '30')             ON CONFLICT DO NOTHING;

-- 3. 开放注册（内部测试用）
UPDATE app_settings SET value = 'true' WHERE key = 'registration_open';

-- 4. 为 expires_at 添加索引（加速清理查询）
CREATE INDEX IF NOT EXISTS idx_mailboxes_expires_at ON mailboxes (expires_at);

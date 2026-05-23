-- +migrate Up
-- Drop removed columns
ALTER TABLE notifications DROP COLUMN IF EXISTS type;
ALTER TABLE notifications DROP COLUMN IF EXISTS channel;
ALTER TABLE notifications DROP COLUMN IF EXISTS status;
ALTER TABLE notifications DROP COLUMN IF EXISTS delivered_at;
ALTER TABLE notifications DROP COLUMN IF EXISTS opened_at;
ALTER TABLE notifications DROP COLUMN IF EXISTS error_message;
ALTER TABLE notifications DROP COLUMN IF EXISTS metadata;

-- Drop removed constraints
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS chk_notif_status;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS chk_notif_type;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS chk_notif_channel;

-- Drop FK (never applied in multi-DB, but drop it anyway)
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_user;

-- Drop dead indexes
DROP INDEX IF EXISTS idx_notifications_status;
DROP INDEX IF EXISTS idx_notifications_failed_retry;
DROP INDEX IF EXISTS idx_notifications_provider_msg;

-- Drop unused columns
ALTER TABLE notifications DROP COLUMN IF EXISTS subject;
ALTER TABLE notifications DROP COLUMN IF EXISTS content;
ALTER TABLE notifications DROP COLUMN IF EXISTS data;
ALTER TABLE notifications DROP COLUMN IF EXISTS provider_message_id;

-- Keep this index:
-- idx_notifications_user_id — keep

-- +migrate Down
-- Restore removed columns (with defaults for existing rows)
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS type VARCHAR(20) NOT NULL DEFAULT 'transactional';
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS channel VARCHAR(20) NOT NULL DEFAULT 'email';
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'sent';
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS opened_at TIMESTAMPTZ;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

-- Restore constraints
ALTER TABLE notifications ADD CONSTRAINT chk_notif_type CHECK (type IN ('transactional', 'marketing', 'system'));
ALTER TABLE notifications ADD CONSTRAINT chk_notif_channel CHECK (channel IN ('email', 'sms', 'websocket'));
ALTER TABLE notifications ADD CONSTRAINT chk_notif_status CHECK (status IN ('pending', 'sent', 'delivered', 'opened', 'failed', 'bounced'));

-- Restore removed columns
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS subject VARCHAR(255);
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS content TEXT NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}';
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS provider_message_id VARCHAR(255);

-- Restore indexes
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status) WHERE status IN ('pending');
CREATE INDEX IF NOT EXISTS idx_notifications_failed_retry ON notifications(status, created_at) WHERE status = 'failed';
CREATE INDEX IF NOT EXISTS idx_notifications_provider_msg ON notifications(provider_message_id) WHERE provider_message_id IS NOT NULL;

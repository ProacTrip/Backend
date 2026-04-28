-- +migrate Up

-- =============================================================================
-- ISSUE 20: Foreign key constraints on user_id → auth.users
-- =============================================================================
ALTER TABLE notifications
    ADD CONSTRAINT IF NOT EXISTS fk_notifications_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE notification_reads
    ADD CONSTRAINT IF NOT EXISTS fk_notification_reads_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- =============================================================================
-- ISSUE 31: Partial index covering status = 'failed' for retry queries
-- GetPending queries for WHERE status IN ('pending', 'failed')
-- idx_notifications_status already covers 'pending'/'processing' but NOT 'failed'
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_notifications_failed_retry
    ON notifications(status, created_at)
    WHERE status = 'failed';

-- +migrate Up

-- =============================================================================
-- ISSUE 20: Foreign key constraints on user_id → auth.users
-- =============================================================================
ALTER TABLE notifications
    ADD CONSTRAINT IF NOT EXISTS fk_notifications_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- =============================================================================
-- ISSUE 31: Partial index covering status = 'failed' for retry queries
-- idx_notifications_status already covers 'pending' but NOT 'failed'
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_notifications_failed_retry
    ON notifications(status, created_at)
    WHERE status = 'failed';

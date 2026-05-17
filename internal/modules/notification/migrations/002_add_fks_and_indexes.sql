-- +migrate Up

-- =============================================================================
-- ISSUE 20: Foreign key constraint on user_id → auth.users
-- NOTE: Cross-database FK not enforceable in PostgreSQL multi-tenant setup.
-- The users table lives in proactrip_auth; notifications in proactrip_notification.
-- This constraint is applied as soft — wrapped in DO block to skip if
-- the referenced table is on a different database.
-- =============================================================================
DO $$
BEGIN
    -- Attempt FK creation — will fail gracefully on cross-database setups
    ALTER TABLE notifications
        ADD CONSTRAINT fk_notifications_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
EXCEPTION
    WHEN undefined_table THEN
        RAISE NOTICE 'Skipping fk_notifications_user: users table not in this database (cross-DB setup)';
    WHEN duplicate_object THEN
        RAISE NOTICE 'Skipping fk_notifications_user: constraint already exists';
    WHEN others THEN
        RAISE NOTICE 'Skipping fk_notifications_user: %', SQLERRM;
END $$;

-- =============================================================================
-- ISSUE 31: Partial index covering status = 'failed' for retry queries
-- idx_notifications_status already covers 'pending' but NOT 'failed'
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_notifications_failed_retry
    ON notifications(status, created_at)
    WHERE status = 'failed';

-- +migrate Down

DROP INDEX IF EXISTS idx_notifications_failed_retry;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_user;

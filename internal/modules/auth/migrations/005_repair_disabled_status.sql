-- +migrate Up
-- Repair migration: 002 had a broken UP section that never actually ran.
-- Apply the real changes now.

-- 1. Add 'disabled' to the CHECK constraint (drop + recreate)
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_user_status;
ALTER TABLE users ADD CONSTRAINT chk_user_status 
    CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked', 'disabled'));

-- 2. Add token_version column if missing
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INT NOT NULL DEFAULT 1;

-- 3. Add positive constraint if missing
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_token_version_positive;
ALTER TABLE users ADD CONSTRAINT chk_token_version_positive CHECK (token_version >= 1);

-- +migrate Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_token_version_positive;
ALTER TABLE users ALTER COLUMN token_version DROP DEFAULT;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_user_status;
ALTER TABLE users ADD CONSTRAINT chk_user_status 
    CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked'));

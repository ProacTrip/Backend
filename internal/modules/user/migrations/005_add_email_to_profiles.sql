-- +migrate Up
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS email VARCHAR(255) NOT NULL DEFAULT '';

-- +migrate Down
ALTER TABLE user_profiles DROP COLUMN IF EXISTS email;

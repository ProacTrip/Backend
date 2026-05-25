-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 016: content_hash para limpieza de dedup en delete
-- =============================================================================
ALTER TABLE user_documents ADD COLUMN IF NOT EXISTS content_hash TEXT;

-- +migrate Down
ALTER TABLE user_documents DROP COLUMN IF EXISTS content_hash;

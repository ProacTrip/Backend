-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 009: Índice único case-insensitive sobre users.email
-- =============================================================================
-- La constraint UNIQUE(email) existente es case-sensitive:
-- "User@Example.com" y "user@example.com" son tratados como distintos.
-- Este índice funcional cierra esa brecha tratando LOWER(email) como único,
-- previniendo registros con el mismo email en diferente capitalización.
-- =============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower
    ON users (lower(email));

-- +migrate Down
DROP INDEX IF EXISTS idx_users_email_lower;

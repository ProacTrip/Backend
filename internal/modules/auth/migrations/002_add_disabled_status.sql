-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 002: Estado 'disabled' + token_version para invalidación
-- =============================================================================
-- Agrega el estado 'disabled' a la tabla users (CHECK constraint ampliado)
-- y la columna token_version. Cada vez que se deshabilita una cuenta,
-- token_version se incrementa atómicamente, invalidando todos los tokens
-- existentes del usuario (el middleware de auth compara el token_version
-- del PASETO con el valor actual en DB/caché).
-- =============================================================================
--
-- 1. Agregar 'disabled' al CHECK constraint de status
-- Down below:
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_user_status;
ALTER TABLE users ADD CONSTRAINT chk_user_status
    CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked', 'disabled'));

-- 2. Agregar token_version para invalidar sesiones al deshabilitar cuenta
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INT NOT NULL DEFAULT 1;

-- 3. Verificar que no hay valores negativos (invariante de dominio)
ALTER TABLE users ADD CONSTRAINT chk_token_version_positive
    CHECK (token_version >= 1);

-- +migrate Down
-- Revierte: quita token_version, revierte CHECK a 5 estados originales.
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_token_version_positive;
ALTER TABLE users DROP COLUMN IF EXISTS token_version;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_user_status;
ALTER TABLE users ADD CONSTRAINT chk_user_status
    CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked'));

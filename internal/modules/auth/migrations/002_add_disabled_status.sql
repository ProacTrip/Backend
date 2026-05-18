-- +migrate Up
-- Agrega el estado 'disabled' al CHECK constraint de users
-- y la columna token_version para invalidación de sesiones.
-- Migración de schema del módulo auth (002).

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
-- Revierte los cambios del migration 002.
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_token_version_positive;
ALTER TABLE users DROP COLUMN IF EXISTS token_version;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_user_status;
ALTER TABLE users ADD CONSTRAINT chk_user_status
    CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked'));

-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 003: Agrega columna role y valores por defecto a user_profiles
-- Soporta distinción client/admin para verificación de documentos.
-- =============================================================================

-- 1. Agregar columna role con CHECK constraint
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS role VARCHAR(20) NOT NULL DEFAULT 'client';

-- 2. Agregar CHECK constraint para valores válidos
ALTER TABLE user_profiles
    ADD CONSTRAINT chk_user_role CHECK (
        role IN ('client', 'admin')
    );

-- 3. El admin real se crea durante el deployment (no se incluye semilla aquí
--    por seguridad — las credenciales de admin nunca deben estar en migraciones)

-- +migrate Down
-- =============================================================================

ALTER TABLE user_profiles
    DROP CONSTRAINT IF EXISTS chk_user_role;

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS role;

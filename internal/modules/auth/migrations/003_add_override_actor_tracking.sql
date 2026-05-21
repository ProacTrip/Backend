-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 003: Actor tracking en permission overrides
-- =============================================================================
-- Agrega columnas created_by y updated_by a user_permission_overrides
-- para auditar qué admin creó o modificó cada override de permisos.
-- Ambas referencian users(id) para trazabilidad completa.
-- =============================================================================

ALTER TABLE user_permission_overrides
    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id);

ALTER TABLE user_permission_overrides
    ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES users(id);

-- +migrate Down
-- Revierte: elimina las columnas de actor tracking.
ALTER TABLE user_permission_overrides DROP COLUMN IF EXISTS updated_by;
ALTER TABLE user_permission_overrides DROP COLUMN IF EXISTS created_by;

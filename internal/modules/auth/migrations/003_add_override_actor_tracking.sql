-- +migrate Up
-- Agrega columnas de actor tracking (created_by, updated_by) a la tabla
-- user_permission_overrides. Esto permite auditar quién creó o modificó
-- cada override de permisos.
-- Migración de schema del módulo auth (003).

ALTER TABLE user_permission_overrides
    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id);

ALTER TABLE user_permission_overrides
    ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES users(id);

-- +migrate Down
-- +migrate Down
-- Remueve las columnas de actor tracking agregadas en 003.
ALTER TABLE user_permission_overrides DROP COLUMN IF EXISTS updated_by;
ALTER TABLE user_permission_overrides DROP COLUMN IF EXISTS created_by;

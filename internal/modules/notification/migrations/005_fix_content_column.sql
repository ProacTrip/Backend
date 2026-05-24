-- +migrate Up
-- Fix content column: 004 fue modificado después de aplicarse;
-- sus DROP CONTENT nunca se ejecutó en producción.
-- El INSERT del repositorio excluye content → "null violates not-null constraint".
ALTER TABLE notifications ALTER COLUMN content DROP NOT NULL;
ALTER TABLE notifications ALTER COLUMN content SET DEFAULT '';

-- Eliminar columnas que 004 debería haber eliminado.
-- Cero referencias en Go (sin campos de struct, sin queries).
ALTER TABLE notifications DROP COLUMN IF EXISTS subject;
ALTER TABLE notifications DROP COLUMN IF EXISTS data;
ALTER TABLE notifications DROP COLUMN IF EXISTS provider_message_id;

-- +migrate Down
-- Restaurar NOT NULL con default para seguridad del rollback.
ALTER TABLE notifications ALTER COLUMN content SET NOT NULL;
ALTER TABLE notifications ALTER COLUMN content SET DEFAULT '';

-- Re-agregar columnas obsoletas con definiciones mínimas.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS subject VARCHAR(255);
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}';
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS provider_message_id VARCHAR(255);

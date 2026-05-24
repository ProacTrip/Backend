-- +migrate Up
-- Fix content column: 004 fue modificado después de aplicarse en producción;
-- originalmente 004 NO droppeaba content. En fresh installs, 004 YA droppeó
-- content → esta migración solo actúa si la columna todavía existe.
-- El INSERT del repositorio excluye content → "null violates not-null constraint"
-- solo aplica en DBs donde 004 corrió antes de ser modificado.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'notifications' AND column_name = 'content'
    ) THEN
        ALTER TABLE notifications ALTER COLUMN content DROP NOT NULL;
        ALTER TABLE notifications ALTER COLUMN content SET DEFAULT '';
    END IF;
END $$;

-- Eliminar columnas que 004 debería haber eliminado (idempotente).
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

-- +migrate Up

-- =============================================================================
-- ISSUE A1: Agregar 'opened' al CHECK constraint de notification_status.
-- Remover 'processing' que nunca fue usado por ningún código.
-- Los estados válidos ahora son: pending, sent, delivered, opened, failed, bounced.
-- =============================================================================
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS chk_notif_status;

ALTER TABLE notifications ADD CONSTRAINT chk_notif_status CHECK (
    status IN ('pending', 'sent', 'delivered', 'opened', 'failed', 'bounced')
);

-- +migrate Down

-- =============================================================================
-- Reversión: restaurar el constraint original con 'processing' y sin 'opened'.
-- =============================================================================
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS chk_notif_status;

ALTER TABLE notifications ADD CONSTRAINT chk_notif_status CHECK (
    status IN ('pending', 'processing', 'sent', 'delivered', 'failed', 'bounced')
);

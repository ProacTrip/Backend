-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 011: Tabla de historial de verificación de documentos
-- =============================================================================
-- Crea la tabla document_verification_history para audit trail inmutable
-- de cambios de estado de verificación de documentos del dashboard.
-- Ubicada en proactrip_user porque depende de user_documents(id).
-- =============================================================================

CREATE TABLE IF NOT EXISTS document_verification_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL,  -- FK implícita a user_documents(id), no declarada (misma DB)
    previous_status VARCHAR(20) NOT NULL,
    new_status      VARCHAR(20) NOT NULL,
    verified_by     UUID NOT NULL,  -- referencia cross-DB a auth.users(id)
    reason          TEXT,
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dvh_document_id
    ON document_verification_history(document_id);

-- +migrate Down
DROP INDEX IF EXISTS idx_dvh_document_id;
DROP TABLE IF EXISTS document_verification_history;

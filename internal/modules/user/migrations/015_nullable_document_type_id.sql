-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 015: document_type_id nullable
-- =============================================================================
-- Al momento de subir un documento, el tipo aún no se conoce — lo determina
-- el pipeline de OCR asincrónicamente. Permitir NULL evita la FK violation
-- con uuid.Nil y refleja correctamente el estado "pendiente de clasificación".
-- =============================================================================

ALTER TABLE user_documents ALTER COLUMN document_type_id DROP NOT NULL;

-- +migrate Down
ALTER TABLE user_documents ALTER COLUMN document_type_id SET NOT NULL;

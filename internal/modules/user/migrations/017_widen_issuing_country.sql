-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 017: Ensanchar issuing_country para aceptar OCR raw output
-- =============================================================================
-- Antes: VARCHAR(2) — solo aceptaba códigos ISO (PE, AR).
-- Ahora: VARCHAR(100) — acepta valores raw del OCR ("PERUANA", "PERU", etc.).
-- La normalización a ISO es responsabilidad de los consumidores (search, etc.).
ALTER TABLE user_documents ALTER COLUMN issuing_country TYPE VARCHAR(100);

-- +migrate Down
ALTER TABLE user_documents ALTER COLUMN issuing_country TYPE VARCHAR(2);

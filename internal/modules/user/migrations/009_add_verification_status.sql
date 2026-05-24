-- +migrate Up
-- Reemplazar is_verified/verified_at/verified_by con verification_status (alineado con domain/document.go).
-- El domain model consolidó esto en un solo enum para simplificar el pipeline OCR.

-- 1. Agregar nueva columna con valor por defecto backfilleado desde is_verified
ALTER TABLE user_documents
    ADD COLUMN IF NOT EXISTS verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified';

-- 2. Backfill: mapear is_verified=true → 'verified', is_verified=false → 'unverified'
--    Solo aplica en DBs donde la columna is_verified existe (migraciones legacy).
--    En fresh installs, verification_status ya se crea con default 'unverified' desde 001.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_documents' AND column_name = 'is_verified'
    ) THEN
        UPDATE user_documents
           SET verification_status = CASE
               WHEN is_verified = true THEN 'verified'
               ELSE 'unverified'
           END
         WHERE verification_status = 'unverified';
    END IF;
END $$;

-- 3. Agregar CHECK constraint
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_verification_status'
          AND conrelid = 'user_documents'::regclass
    ) THEN
        ALTER TABLE user_documents
            ADD CONSTRAINT chk_verification_status
            CHECK (verification_status IN ('unverified', 'verified', 'rejected', 'manual_review', 'suspicious'));
    END IF;
END $$;

-- +migrate Down
ALTER TABLE user_documents DROP CONSTRAINT IF EXISTS chk_verification_status;
ALTER TABLE user_documents DROP COLUMN IF EXISTS verification_status;

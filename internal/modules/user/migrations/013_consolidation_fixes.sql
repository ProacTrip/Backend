-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 013: Agrega constraints que la 001 consolidada ya incluye
-- Para DBs ya desplegadas que corrieron migraciones 001-012 por separado.
-- Idempotente: usa DO blocks con IF NOT EXISTS para poder re-correr.
-- =============================================================================

-- 1. UNIQUE(email) en user_profiles — previene duplicados
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_profiles_email'
          AND conrelid = 'user_profiles'::regclass
    ) THEN
        ALTER TABLE user_profiles
            ADD CONSTRAINT uq_profiles_email UNIQUE (email);
    END IF;
END $$;

-- 2. E.164 CHECK en phone — solo permite números internacionales válidos
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_phone_e164'
          AND conrelid = 'user_profiles'::regclass
    ) THEN
        ALTER TABLE user_profiles
            ADD CONSTRAINT chk_phone_e164
            CHECK (phone IS NULL OR phone ~ '^\+[1-9]\d{6,14}$');
    END IF;
END $$;

-- 3. FK en document_verification_history.document_id → user_documents(id)
--    La migración 011 no declaró el FK (comentario decía "implícito").
--    NOTA: si hay filas huérfanas (document_id que no existe en user_documents),
--    esta FK fallará. En producción, los documentos se borran con ON DELETE CASCADE
--    de user_profiles → user_documents, así que no deberían existir huérfanos.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_dvh_document_id'
          AND conrelid = 'document_verification_history'::regclass
    ) THEN
        ALTER TABLE document_verification_history
            ADD CONSTRAINT fk_dvh_document_id
            FOREIGN KEY (document_id) REFERENCES user_documents(id);
    END IF;
END $$;

-- 4. Cambiar DEFAULT de gen_random_uuid() a uuidv7() en document_verification_history
--    La 011 usó gen_random_uuid(); alineamos con el resto del schema.
DO $$
BEGIN
    ALTER TABLE document_verification_history
        ALTER COLUMN id SET DEFAULT uuidv7();
EXCEPTION
    WHEN undefined_function THEN
        -- uuidv7() no está disponible (PostgreSQL < 17 sin extensión)
        -- No es crítico: gen_random_uuid() funciona igual para unicidad.
        RAISE NOTICE 'uuidv7() not available, keeping current default on document_verification_history.id';
END $$;

-- +migrate Down
-- No hay down: los constraints agregados son correcciones estructurales.
-- Si se necesita revertir, aplicar migraciones inversas manualmente.

-- +migrate Up
-- Agregar columna status a user_profiles para control de estado.
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS status varchar(20) NOT NULL DEFAULT 'active';

-- Restricción CHECK para valores válidos
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_profile_status'
          AND conrelid = 'user_profiles'::regclass
    ) THEN
        ALTER TABLE user_profiles
            ADD CONSTRAINT chk_profile_status
            CHECK (status IN ('active', 'inactive', 'suspended', 'deleted'));
    END IF;
END $$;

-- Índice para filtrado por status
CREATE INDEX IF NOT EXISTS idx_user_profiles_status
    ON user_profiles(status)
    WHERE status != 'active';

-- +migrate Down
ALTER TABLE user_profiles DROP CONSTRAINT IF EXISTS chk_profile_status;
DROP INDEX IF EXISTS idx_user_profiles_status;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS status;

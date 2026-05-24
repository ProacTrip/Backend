-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 012: Eliminar columnas muertas de user_profiles
-- =============================================================================
-- Las columnas is_public, timezone_name, phone_verified y current_location
-- no existen en el domain model ni en USER_API.md.
--   - is_public:             no está en el modelo de dominio ni en la API
--   - timezone_name:         no se gestiona por usuario, viene de /v1/environment
--   - phone_verified:        no está en la API
--   - current_location:      no está en la API
-- =============================================================================

ALTER TABLE user_profiles DROP COLUMN IF EXISTS is_public;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS timezone_name;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS phone_verified;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS current_location;

-- +migrate Down
-- No hay rollback: estas columnas nunca deberían haber existido.
-- Si se necesita restaurar, usar un backup de la DB.

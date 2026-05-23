-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 008: Agregar columna first_name a la tabla users
-- =============================================================================
-- El first_name es obligatorio para los emails de cambio de estado de cuenta.
-- Antes de esta migración, el first_name vivía únicamente en el perfil del
-- usuario (user/profile module). Ahora se almacena en auth.users para que
-- el módulo auth pueda publicarlo en los eventos account_disabled/enabled
-- sin depender de otros módulos.
-- =============================================================================
ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name varchar(255);

-- +migrate Down
ALTER TABLE users DROP COLUMN IF EXISTS first_name;

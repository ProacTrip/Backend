-- +migrate Up
-- Índices de soporte para el dashboard de usuarios:
--   - idx_users_list: paginación por cursor (created_at DESC, id DESC)
--   - idx_users_status_search: filtro combinado status + búsqueda por email
-- Migración de schema del módulo auth (004).

CREATE INDEX IF NOT EXISTS idx_users_list
    ON users (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_users_status_search
    ON users (status, email);

-- +migrate Down
-- +migrate Down
-- Remueve los índices de dashboard creados en 004.
DROP INDEX IF EXISTS idx_users_status_search;
DROP INDEX IF EXISTS idx_users_list;

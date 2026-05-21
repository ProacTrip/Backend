-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 004: Índices para dashboard de usuarios
-- =============================================================================
-- Optimiza las queries del dashboard de administración:
--   idx_users_list          — paginación por cursor (list_users feature)
--   idx_users_status_search — filtro combinado status + búsqueda por email
-- =============================================================================

CREATE INDEX IF NOT EXISTS idx_users_list
    ON users (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_users_status_search
    ON users (status, email);

-- +migrate Down
-- Revierte: elimina los índices de dashboard.
DROP INDEX IF EXISTS idx_users_status_search;
DROP INDEX IF EXISTS idx_users_list;

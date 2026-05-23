-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 007: Limpieza de schema del dashboard
-- =============================================================================
-- Elimina features obsoletas del dashboard:
--   1. Tabla user_permission_overrides (reemplazada por resolución solo-rol)
--   2. Columna users.mfa_enabled (MFA eliminado del dominio)
--   3. Permisos no usados: roles:read, roles:write, permissions:read,
--      permissions:write, feature_limits:read, sessions:read, sessions:write
--   4. Actualiza role_permissions del admin a solo 3 permisos
-- =============================================================================

-- 1. Eliminar tabla de permission overrides
DROP TABLE IF EXISTS user_permission_overrides;

-- 2. Eliminar columna mfa_enabled de users
ALTER TABLE users DROP COLUMN IF EXISTS mfa_enabled;

-- 3. Eliminar permisos no usados del catálogo
DELETE FROM permissions WHERE (resource, action) IN (
    ('roles', 'read'),
    ('roles', 'write'),
    ('permissions', 'read'),
    ('permissions', 'write'),
    ('feature_limits', 'read'),
    ('sessions', 'read'),
    ('sessions', 'write')
);

-- 4. Actualizar role_permissions del admin: mantener solo los 3 permisos activos
--    Los permisos activos son: users:read, users:write, feature_limits:write.
--    Eliminamos las asociaciones a permisos que ya no existen.
DELETE FROM role_permissions rp
USING roles r
WHERE rp.role_id = r.id
  AND r.name = 'admin'
  AND rp.permission_id NOT IN (
    SELECT p.id FROM permissions p
    WHERE (p.resource, p.action) IN (
      ('users', 'read'),
      ('users', 'write'),
      ('feature_limits', 'write')
    )
  );

-- +migrate Down
-- =============================================================================
-- DOWN: Recrea la tabla de overrides, la columna mfa_enabled y los permisos.
-- =============================================================================

-- 1. Recrear tabla user_permission_overrides
CREATE TABLE IF NOT EXISTS user_permission_overrides(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    granted boolean NOT NULL DEFAULT true,
    reason varchar(500) NOT NULL DEFAULT '',
    expires_at timestamptz,
    created_by uuid,
    updated_by uuid,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_user_permission_override UNIQUE (user_id, permission_id)
);

CREATE INDEX idx_user_permission_overrides_user ON user_permission_overrides(user_id);
CREATE INDEX idx_user_permission_overrides_perm ON user_permission_overrides(permission_id);

-- 2. Restaurar columna mfa_enabled
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_enabled boolean DEFAULT FALSE;

-- 3. Reinsertar permisos eliminados
INSERT INTO permissions(resource, action, description) VALUES
    ('roles', 'read', 'Ver roles'),
    ('roles', 'write', 'Gestionar roles'),
    ('permissions', 'read', 'Ver permisos RBAC'),
    ('permissions', 'write', 'Gestionar permisos RBAC'),
    ('feature_limits', 'read', 'Ver límites de features'),
    ('sessions', 'read', 'Ver sesiones activas'),
    ('sessions', 'write', 'Invalidar sesiones')
ON CONFLICT (resource, action) DO NOTHING;

-- 4. Reasignar al admin
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'admin'
  AND (p.resource, p.action) IN (
    ('roles', 'read'), ('roles', 'write'),
    ('permissions', 'read'), ('permissions', 'write'),
    ('feature_limits', 'read'),
    ('sessions', 'read'), ('sessions', 'write')
  )
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

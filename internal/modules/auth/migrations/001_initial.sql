-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 001: Schema base del módulo Auth
-- =============================================================================
-- Crea las tablas core del sistema de autenticación:
--   roles              — roles del sistema (client, admin)
--   permissions        — permisos RBAC (recurso + acción)
--   role_permissions   — relación roles ↔ permisos (M:N)
--   users              — usuarios con email, contraseña, estado, rol
--   user_permission_overrides — sobrescritura de permisos por usuario
--   user_feature_limits       — límites de features por usuario
--   default_feature_limits    — límites de features por rol
--   user_auth_identities      — identidades OAuth vinculadas a usuarios
--
-- También incluye seeds: roles (2), permisos admin (9), admin user.
-- =============================================================================
--
-- =============================================================================
-- FUNCIONES Y UTILIDADES (sin lógica de negocio)
-- =============================================================================

-- Solo actualiza el timestamp. Responsabilidad puramente técnica.
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

-- =============================================================================
-- ROLES
-- =============================================================================
-- Define los roles base del sistema. Cada usuario tiene un único rol.
-- is_system = TRUE: roles que no pueden eliminarse (client, admin).
CREATE TABLE IF NOT EXISTS roles(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name varchar(50) UNIQUE NOT NULL,
    description text,
    is_system boolean DEFAULT FALSE,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER update_roles_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

INSERT INTO roles(name, description, is_system)
    VALUES
        ('client', 'Usuario estándar de la plataforma', TRUE),
        ('admin', 'Administrador con control total', TRUE);

-- =============================================================================
-- PERMISOS RBAC
-- =============================================================================
-- Catálogo de permisos del sistema. Cada permiso es un par (recurso, acción).
-- Ejemplo: resource='users', action='read' → puede ver usuarios.
CREATE TABLE IF NOT EXISTS permissions(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    resource varchar(50) NOT NULL,
    action varchar(50) NOT NULL,
    description text,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_permission_resource_action UNIQUE (resource, action)
);

CREATE TRIGGER update_permissions_updated_at
    BEFORE UPDATE ON permissions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- =============================================================================
-- RELACIÓN ROLES ↔ PERMISOS (M:N)
-- =============================================================================
-- Asocia cada rol con los permisos que tiene. Un rol puede tener
-- múltiples permisos y un permiso puede estar en múltiples roles.
CREATE TABLE IF NOT EXISTS role_permissions(
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TRIGGER update_role_permissions_updated_at
    BEFORE UPDATE ON role_permissions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_role_permissions_permission_id ON role_permissions(permission_id);

-- Seed permissions for admin role
INSERT INTO permissions(resource, action, description)
    VALUES
        ('users', 'read', 'Ver usuarios'),
        ('users', 'write', 'Editar usuarios'),
        ('users', 'delete', 'Eliminar usuarios'),
        ('roles', 'read', 'Ver roles'),
        ('roles', 'write', 'Editar roles'),
        ('roles', 'delete', 'Eliminar roles'),
        ('settings', 'read', 'Ver configuración'),
        ('settings', 'write', 'Editar configuración'),
        ('system', 'manage', 'Gestión total del sistema');

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p WHERE r.name = 'admin';

-- =============================================================================
-- USUARIOS
-- =============================================================================
-- Tabla principal de autenticación. Almacena credenciales (password_hash),
-- estado de verificación (email_verified), bloqueo por intentos fallidos
-- (locked_until, failed_login_attempts), y relación con roles.
-- mfa_enabled: reservado para futuro MFA, actualmente no implementado.
CREATE TABLE IF NOT EXISTS users(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    email varchar(255) NOT NULL UNIQUE,
    email_verified boolean DEFAULT FALSE,
    email_verified_at timestamptz,
    password_hash varchar(255),
    status varchar(50) NOT NULL DEFAULT 'pending_verification',
    role_id uuid NOT NULL REFERENCES roles(id),
    last_login_at timestamptz,
    login_count integer DEFAULT 0,
    failed_login_attempts integer DEFAULT 0,
    locked_until timestamptz,
    mfa_enabled boolean DEFAULT FALSE,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_user_status CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked')),
    CONSTRAINT chk_email_format CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_users_role_id      ON users(role_id);
CREATE INDEX idx_users_status       ON users(status);
CREATE INDEX idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;

-- =============================================================================
-- SOBRESCRITURA DE PERMISOS POR USUARIO
-- =============================================================================
-- Permite otorgar/denegar permisos a usuarios individuales más allá de su rol.
-- Ejemplo: denegar 'users:delete' a un admin específico, o conceder
-- 'users:read' a un client. Usado por el dashboard de permisos.
CREATE TABLE IF NOT EXISTS user_permission_overrides(
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    granted boolean NOT NULL,
    reason text,
    created_by uuid REFERENCES users(id),
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    expires_at timestamptz,
    PRIMARY KEY (user_id, permission_id)
);

CREATE TRIGGER update_user_permission_overrides_updated_at
    BEFORE UPDATE ON user_permission_overrides
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_upo_permission_id ON user_permission_overrides(permission_id);
CREATE INDEX idx_upo_expires_at    ON user_permission_overrides(expires_at)
    WHERE expires_at IS NOT NULL;

-- =============================================================================
-- LÍMITES DE FEATURES POR USUARIO
-- =============================================================================
-- Restringe el uso de features a nivel de usuario (rate limiting funcional).
-- Ejemplo: limitar búsquedas de vuelo a 10/día para un usuario específico.
-- Si no hay límite por usuario, se usa el límite por rol (default_feature_limits).
CREATE TABLE IF NOT EXISTS user_feature_limits(
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feature_key varchar(100) NOT NULL,
    "window" VARCHAR(20) NOT NULL CHECK ("window" IN ('minute', 'hour', 'day', 'month')),
    limit_value INTEGER CHECK (limit_value IS NULL OR limit_value >= 0),
    reason TEXT,
    created_by UUID REFERENCES users(id),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, feature_key, "window")
);

CREATE TRIGGER update_user_feature_limits_updated_at
    BEFORE UPDATE ON user_feature_limits
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- =============================================================================
-- LÍMITES DE FEATURES POR ROL (default)
-- =============================================================================
-- Límites base por rol. Se aplican cuando no hay un límite específico
-- por usuario en user_feature_limits. Ejemplo: client = 50 búsquedas/día.
CREATE TABLE IF NOT EXISTS default_feature_limits(
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    feature_key varchar(100) NOT NULL,
    "window" VARCHAR(20) NOT NULL CHECK ("window" IN ('minute', 'hour', 'day', 'month')),
    limit_value integer NOT NULL CHECK (limit_value >= 0),
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, feature_key, "window")
);

CREATE TRIGGER update_default_feature_limits_updated_at
    BEFORE UPDATE ON default_feature_limits
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- =============================================================================
-- IDENTIDADES DE AUTENTICACIÓN (OAuth2)
-- =============================================================================
-- Vincula usuarios con proveedores externos (Google). Almacena tokens OAuth
-- encriptados (access_token_enc, refresh_token_enc) y metadatos del perfil
-- externo (display_name, avatar_url). Un usuario puede tener múltiples
-- identidades (una por proveedor).
CREATE TABLE IF NOT EXISTS user_auth_identities(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_code varchar(30) NOT NULL,
    provider_user_id varchar(255) NOT NULL,
    email varchar(255),
    display_name varchar(255),
    avatar_url text,
    access_token_enc text,
    refresh_token_enc text,
    token_expires_at timestamptz,
    raw_data jsonb DEFAULT '{}',
    last_used_at timestamptz,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_user_provider UNIQUE (user_id, provider_code),
    CONSTRAINT uq_provider_user UNIQUE (provider_code, provider_user_id)
);

CREATE TRIGGER update_user_auth_identities_updated_at
    BEFORE UPDATE ON user_auth_identities
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_uai_user_id ON user_auth_identities(user_id);

-- =============================================================================
-- SEED: Usuario admin inicial
-- =============================================================================
-- Email: admin@proactrip.com / Password: Admin123!
-- Rol admin con todos los permisos (ver seed de role_permissions arriba).
-- Email ya verificado, cuenta activa, sin bloqueo.

-- 1. Create admin user
INSERT INTO users (
    id, email, email_verified, email_verified_at, password_hash,
    status, role_id, first_name, mfa_enabled, failed_login_attempts, created_at, updated_at
)
SELECT
    uuidv7(),
    'admin@proactrip.com',
    true,
    NOW(),
    -- password: Admin123! (Argon2id hash)
    '$argon2id$v=19$m=65536,t=3,p=4$7uSScickGokih2Yd09NYcQ$w43HqGfvbeIKIXPNFsovygtfY1QjHnVYKkEIamo58+A',
    'active',
    (SELECT id FROM roles WHERE name = 'admin' LIMIT 1),
    'Admin',
    true,
    0,
    NOW(),
    NOW()
ON CONFLICT (email) DO NOTHING;
-- +migrate Down
-- No hay rollback para migración inicial (schema base).

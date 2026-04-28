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
-- TIPOS DE TOKEN
-- =============================================================================
CREATE TABLE IF NOT EXISTS token_types(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    code varchar(50) NOT NULL UNIQUE,
    description text,
    ttl_seconds integer NOT NULL CHECK (ttl_seconds > 0),
    is_active boolean DEFAULT TRUE,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER update_token_types_updated_at
    BEFORE UPDATE ON token_types
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

INSERT INTO token_types(code, description, ttl_seconds)
    VALUES
        ('access', 'Token de acceso para API', 3600),
        ('refresh', 'Token para renovar access tokens', 2592000),
        ('email_verification', 'Token para verificación de email', 86400),
        ('password_reset', 'Token para reset de contraseña', 3600);

-- =============================================================================
-- PROVEEDORES DE AUTENTICACIÓN (OAuth2)
-- =============================================================================
CREATE TABLE IF NOT EXISTS auth_providers(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    code varchar(30) NOT NULL UNIQUE,
    name varchar(50) NOT NULL,
    is_active boolean DEFAULT TRUE,
    config jsonb DEFAULT '{}',
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER update_auth_providers_updated_at
    BEFORE UPDATE ON auth_providers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

INSERT INTO auth_providers(code, name, config)
    VALUES ('google', 'Google', '{"icon": "google", "scopes": ["openid", "email", "profile"]}'::jsonb);

-- =============================================================================
-- ROLES
-- =============================================================================
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
        ('staff', 'Personal interno con permisos elevados', FALSE),
        ('admin', 'Administrador con control total', TRUE);

-- =============================================================================
-- PERMISOS RBAC
-- =============================================================================
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
-- RELACIÓN ROLES ↔ PERMISOS
-- =============================================================================
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
-- LÍMITES DE FEATURES POR ROL
-- =============================================================================
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
CREATE TABLE IF NOT EXISTS user_auth_identities(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_code varchar(30) NOT NULL REFERENCES auth_providers(code),
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
-- TOKENS ACTIVOS
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_tokens(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_type_id varchar(50) NOT NULL,
    token_jti uuid NOT NULL UNIQUE,
    issued_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoked_reason varchar(100),
    ip_address inet,
    user_agent text,
    device_info jsonb DEFAULT '{}',
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_user_token_type CHECK (token_type_id IN ('access', 'refresh', 'email_verification', 'password_reset'))
);

CREATE TRIGGER update_user_tokens_updated_at
    BEFORE UPDATE ON user_tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_user_tokens_user_id    ON user_tokens(user_id);
CREATE INDEX idx_user_tokens_expires_at ON user_tokens(expires_at);
CREATE INDEX idx_user_tokens_active     ON user_tokens(user_id, expires_at)
    WHERE revoked_at IS NULL;

-- =============================================================================
-- TOKENS REVOCADOS (blacklist persistente)
-- =============================================================================
CREATE TABLE IF NOT EXISTS revoked_tokens(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    token_jti uuid NOT NULL UNIQUE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_type_id varchar(50) NOT NULL,
    revoked_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    revoked_by uuid REFERENCES users(id),
    reason varchar(100),
    ip_address inet,
    user_agent text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_revoked_token_type CHECK (token_type_id IN ('access', 'refresh', 'email_verification', 'password_reset'))
);

CREATE TRIGGER update_revoked_tokens_updated_at
    BEFORE UPDATE ON revoked_tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_revoked_tokens_user_id    ON revoked_tokens(user_id);
CREATE INDEX idx_revoked_tokens_expires_at ON revoked_tokens(expires_at);

-- =============================================================================
-- MÉTODOS MFA
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_mfa_methods(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    method_type varchar(20) NOT NULL CHECK (method_type IN ('totp', 'sms', 'email', 'backup_codes')),
    is_enabled boolean DEFAULT TRUE,
    is_verified boolean DEFAULT FALSE,
    phone_number varchar(20),
    email_address varchar(255),
    totp_secret_encrypted text,
    backup_codes_hash text[],
    verified_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER update_user_mfa_methods_updated_at
    BEFORE UPDATE ON user_mfa_methods
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_mfa_user_id ON user_mfa_methods(user_id);

-- =============================================================================
-- SEED: Admin user with verified email and MFA
-- Password: Admin123!
-- =============================================================================

-- 1. Create admin user
INSERT INTO users (
    id, email, email_verified, email_verified_at, password_hash,
    status, role_id, mfa_enabled, failed_login_attempts, created_at, updated_at
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
    true,
    0,
    NOW(),
    NOW()
ON CONFLICT (email) DO NOTHING;

-- 2. Create MFA method (TOTP) for admin
-- Secret: JBSWY3DPEHPK3PXP (test secret, compatible with Google Authenticator)
INSERT INTO user_mfa_methods (
    id, user_id, method_type, is_enabled, is_verified,
    totp_secret_encrypted, verified_at, created_at, updated_at
)
SELECT
    uuidv7(),
    u.id,
    'totp',
    true,
    true,
    'JBSWY3DPEHPK3PXP',
    NOW(),
    NOW(),
    NOW()
FROM users u
WHERE u.email = 'admin@proactrip.com'
  AND NOT EXISTS (
      SELECT 1 FROM user_mfa_methods m WHERE m.user_id = u.id
  );
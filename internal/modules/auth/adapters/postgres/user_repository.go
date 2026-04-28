package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Implementación PostgreSQL del UserRepository.
// Alineado con el schema de la migración SQL.

type UserRepository struct {
	pool PgxPool
}

type PgxPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func NewUserRepository(pool PgxPool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create crea un nuevo usuario (alineado con schema)
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (
			id, email, email_verified, email_verified_at, password_hash, status,
			role_id, login_count, failed_login_attempts, locked_until, mfa_enabled,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := r.pool.Exec(ctx, query,
		user.ID,
		user.Email,
		user.EmailVerified,
		user.EmailVerifiedAt,
		user.PasswordHash,
		user.Status,
		user.RoleID,
		user.LoginCount,
		user.FailedLoginAttempts,
		user.LockedUntil,
		user.MFAEnabled,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

// GetByID obtiene un usuario por su ID
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT u.id, u.email, u.email_verified, u.email_verified_at, u.password_hash,
		       u.status, u.role_id, r.name as role_name, u.login_count, u.failed_login_attempts,
		       u.locked_until, u.mfa_enabled, u.last_login_at, u.created_at, u.updated_at
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1
	`

	var user domain.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.EmailVerified,
		&user.EmailVerifiedAt,
		&user.PasswordHash,
		&user.Status,
		&user.RoleID,
		&user.RoleName,
		&user.LoginCount,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&user.MFAEnabled,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &user, nil
}

// GetByEmail obtiene un usuario por su email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT u.id, u.email, u.email_verified, u.email_verified_at, u.password_hash,
		       u.status, u.role_id, r.name as role_name, u.login_count, u.failed_login_attempts,
		       u.locked_until, u.mfa_enabled, u.last_login_at, u.created_at, u.updated_at
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1
	`

	var user domain.User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.EmailVerified,
		&user.EmailVerifiedAt,
		&user.PasswordHash,
		&user.Status,
		&user.RoleID,
		&user.RoleName,
		&user.LoginCount,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&user.MFAEnabled,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	return &user, nil
}

// Update actualiza un usuario existente
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users SET
			email = $1, email_verified = $2, email_verified_at = $3,
			password_hash = $4, status = $5, role_id = $6,
			login_count = $7, failed_login_attempts = $8, locked_until = $9,
			mfa_enabled = $10, last_login_at = $11, updated_at = $12
		WHERE id = $13
	`

	ct, err := r.pool.Exec(ctx, query,
		user.Email,
		user.EmailVerified,
		user.EmailVerifiedAt,
		user.PasswordHash,
		user.Status,
		user.RoleID,
		user.LoginCount,
		user.FailedLoginAttempts,
		user.LockedUntil,
		user.MFAEnabled,
		user.LastLoginAt,
		user.UpdatedAt,
		user.ID,
	)

	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("update user: %w", domain.ErrUserNotFound)
	}

	return nil
}

// GetRoleByName obtiene un rol por su nombre (con permisos via JOIN)
func (r *UserRepository) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	// La tabla permissions tiene columnas 'resource' y 'action', no 'name'
	// Los permisos se construyen como "resource:action" ej. "users:read"
	query := `
		SELECT r.id, r.name, r.description, r.is_system,
		       COALESCE(array_agg(p.resource || ':' || p.action) FILTER (WHERE p.resource IS NOT NULL), '{}')
		FROM roles r
		LEFT JOIN role_permissions rp ON r.id = rp.role_id
		LEFT JOIN permissions p ON rp.permission_id = p.id
		WHERE r.name = $1
		GROUP BY r.id, r.name, r.description, r.is_system
	`

	var role domain.Role
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&role.IsSystem,
		&role.Permissions,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrRoleNotFound
		}
		return nil, fmt.Errorf("get role by name: %w", err)
	}

	return &role, nil
}

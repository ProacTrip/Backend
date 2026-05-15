// Adaptador PostgreSQL para CRUD de permission overrides del dashboard.
// Implementa permission_overrides.OverrideRepo (para el usecase CRUD) y
// services.PermissionOverrideRepository (para el PermissionResolver).
// Son dos interfaces distintas porque GetOverridesByUserID retorna tipos diferentes.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain/services"
	overrides "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/permission_overrides"
)

// =============================================================================
// PermissionOverridesRepository — implementa permission_overrides.OverrideRepo
// =============================================================================

// PermissionOverridesRepository maneja el CRUD de overrides de permiso para
// el dashboard. Implementa permission_overrides.OverrideRepo.
type PermissionOverridesRepository struct {
	pool PgxPool
}

// NewPermissionOverridesRepository crea un nuevo repositorio de overrides.
func NewPermissionOverridesRepository(pool PgxPool) *PermissionOverridesRepository {
	return &PermissionOverridesRepository{pool: pool}
}

// GetOverridesByUserID lista todos los overrides de un usuario con JOIN a
// permissions para obtener el código del permiso (resource:action).
// PO-SPEC-002: incluye overrides expirados (el cliente o el resolver los filtra).
func (r *PermissionOverridesRepository) GetOverridesByUserID(ctx context.Context, userID uuid.UUID) ([]overrides.OverrideRow, error) {
	query := `
		SELECT upo.user_id, upo.permission_id,
		       p.resource || ':' || p.action AS permission,
		       upo.granted, upo.reason, upo.expires_at,
		       upo.created_at, upo.updated_at
		FROM user_permission_overrides upo
		JOIN permissions p ON upo.permission_id = p.id
		WHERE upo.user_id = $1
		ORDER BY p.resource, p.action
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get overrides by user: %w", err)
	}
	defer rows.Close()

	var result []overrides.OverrideRow
	for rows.Next() {
		var row overrides.OverrideRow
		if scanErr := rows.Scan(
			&userID,             // upo.user_id (no usado en el struct)
			&row.ID,             // upo.permission_id
			&row.Permission,     // p.resource || ':' || p.action
			&row.Granted,        // upo.granted
			&row.Reason,         // upo.reason
			&row.ExpiresAt,      // upo.expires_at
			&row.CreatedAt,      // upo.created_at
			&row.UpdatedAt,      // upo.updated_at
		); scanErr != nil {
			return nil, fmt.Errorf("scan override row: %w", scanErr)
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

// CreateOverride crea un override de permiso para un usuario.
// PO-SPEC-001: actor tracking via createdBy. Retorna el permission_id como ID.
// Duplicado (mismo user+permission) → ErrPermissionOverrideAlreadyExists.
func (r *PermissionOverridesRepository) CreateOverride(
	ctx context.Context,
	userID, permissionID uuid.UUID,
	granted bool,
	expiresAt *time.Time,
	reason string,
	createdBy uuid.UUID,
) (uuid.UUID, error) {
	query := `INSERT INTO user_permission_overrides
	          (user_id, permission_id, granted, reason, expires_at, created_by, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
	          ON CONFLICT (user_id, permission_id) DO NOTHING
	          RETURNING permission_id`

	var id uuid.UUID
	err := r.pool.QueryRow(ctx, query, userID, permissionID, granted, reason, expiresAt, createdBy).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrPermissionOverrideAlreadyExists
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("create override: %w", err)
	}

	return id, nil
}

// UpdateOverride actualiza un override existente.
// PO-SPEC-003: permission_id es inmutable.
func (r *PermissionOverridesRepository) UpdateOverride(
	ctx context.Context,
	overrideID uuid.UUID,
	granted bool,
	expiresAt *time.Time,
	reason string,
	updatedBy uuid.UUID,
) error {
	query := `UPDATE user_permission_overrides
	          SET granted = $1, reason = $2, expires_at = $3,
	              updated_by = $4, updated_at = NOW()
	          WHERE permission_id = $5`

	ct, err := r.pool.Exec(ctx, query, granted, reason, expiresAt, updatedBy, overrideID)
	if err != nil {
		return fmt.Errorf("update override: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrPermissionOverrideNotFound
	}

	return nil
}

// DeleteOverride elimina un override por su permission_id.
func (r *PermissionOverridesRepository) DeleteOverride(ctx context.Context, overrideID uuid.UUID) error {
	query := `DELETE FROM user_permission_overrides WHERE permission_id = $1`

	ct, err := r.pool.Exec(ctx, query, overrideID)
	if err != nil {
		return fmt.Errorf("delete override: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrPermissionOverrideNotFound
	}

	return nil
}

// Compile-time check: implementa permission_overrides.OverrideRepo.
var _ overrides.OverrideRepo = (*PermissionOverridesRepository)(nil)

// =============================================================================
// PermissionOverrideResolverRepository — implementa services.PermissionOverrideRepository
// =============================================================================

// PermissionOverrideResolverRepository expone los overrides en el formato que
// el PermissionResolver necesita. Es un tipo separado porque GetOverridesByUserID
// retorna services.PermissionOverride (no overrides.OverrideRow).
type PermissionOverrideResolverRepository struct {
	pool PgxPool
}

// NewPermissionOverrideResolverRepository crea un nuevo repositorio de resolución.
func NewPermissionOverrideResolverRepository(pool PgxPool) *PermissionOverrideResolverRepository {
	return &PermissionOverrideResolverRepository{pool: pool}
}

// GetOverridesByUserID retorna los overrides en el formato del resolver de permisos.
// PM-SPEC-003: incluye expirados (el resolver los filtra por expires_at < now).
func (r *PermissionOverrideResolverRepository) GetOverridesByUserID(ctx context.Context, userID uuid.UUID) ([]services.PermissionOverride, error) {
	query := `
		SELECT upo.user_id, upo.permission_id,
		       p.resource || ':' || p.action AS permission,
		       upo.granted, upo.expires_at
		FROM user_permission_overrides upo
		JOIN permissions p ON upo.permission_id = p.id
		WHERE upo.user_id = $1
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get resolver overrides: %w", err)
	}
	defer rows.Close()

	var result []services.PermissionOverride
	for rows.Next() {
		var ov services.PermissionOverride
		if scanErr := rows.Scan(
			&ov.UserID,
			&ov.PermissionID,
			&ov.Permission,
			&ov.Granted,
			&ov.ExpiresAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan resolver override: %w", scanErr)
		}
		result = append(result, ov)
	}

	return result, rows.Err()
}

// Compile-time check: implementa services.PermissionOverrideRepository.
var _ services.PermissionOverrideRepository = (*PermissionOverrideResolverRepository)(nil)

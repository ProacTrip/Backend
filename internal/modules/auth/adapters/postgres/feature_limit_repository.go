// Adaptador PostgreSQL para CRUD de feature limits del dashboard.
// Implementa feature_limits.FeatureLimitRepo usando las tablas
// user_feature_limits y default_feature_limits.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	featurelimits "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/feature_limits"
)

// =============================================================================
// FeatureLimitRepository — implementa feature_limits.FeatureLimitRepo
// =============================================================================

// FeatureLimitRepository maneja el CRUD de límites de feature para usuarios
// y defaults de rol. Implementa feature_limits.FeatureLimitRepo (y su sub-interfaz
// FeatureLimitResolutionRepo que usa FeatureLimitService).
type FeatureLimitRepository struct {
	pool PgxPool
}

// NewFeatureLimitRepository crea un nuevo repositorio de feature limits.
func NewFeatureLimitRepository(pool PgxPool) *FeatureLimitRepository {
	return &FeatureLimitRepository{pool: pool}
}

// =============================================================================
// User Limits CRUD
// =============================================================================

// GetUserLimits lista los límites de feature de un usuario.
func (r *FeatureLimitRepository) GetUserLimits(ctx context.Context, userID uuid.UUID) ([]featurelimits.FeatureLimitRow, error) {
	query := `SELECT feature_key, limit_value, "window", created_at, updated_at
	          FROM user_feature_limits WHERE user_id = $1 ORDER BY feature_key`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get user limits: %w", err)
	}
	defer rows.Close()

	var limits []featurelimits.FeatureLimitRow
	for rows.Next() {
		var l featurelimits.FeatureLimitRow
		if scanErr := rows.Scan(&l.FeatureKey, &l.LimitValue, &l.Window, &l.CreatedAt, &l.UpdatedAt); scanErr != nil {
			return nil, fmt.Errorf("scan user limit: %w", scanErr)
		}
		limits = append(limits, l)
	}

	return limits, rows.Err()
}

// SetUserLimit crea o actualiza un límite de feature para un usuario.
// Usa INSERT ... ON CONFLICT para UPSERT idempotente.
func (r *FeatureLimitRepository) SetUserLimit(ctx context.Context, userID uuid.UUID, featureKey string, limitValue *int, window string) error {
	if window == "" {
		window = "month" // default window
	}

	query := `INSERT INTO user_feature_limits (user_id, feature_key, "window", limit_value, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, NOW(), NOW())
	          ON CONFLICT (user_id, feature_key, "window") DO UPDATE
	          SET limit_value = EXCLUDED.limit_value, updated_at = NOW()`

	if _, err := r.pool.Exec(ctx, query, userID, featureKey, window, limitValue); err != nil {
		return fmt.Errorf("set user limit: %w", err)
	}

	return nil
}

// DeleteUserLimit elimina un límite de feature de un usuario.
func (r *FeatureLimitRepository) DeleteUserLimit(ctx context.Context, userID uuid.UUID, featureKey string) error {
	query := `DELETE FROM user_feature_limits WHERE user_id = $1 AND feature_key = $2`

	ct, err := r.pool.Exec(ctx, query, userID, featureKey)
	if err != nil {
		return fmt.Errorf("delete user limit: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrFeatureLimitNotFound
	}

	return nil
}

// =============================================================================
// Role Defaults CRUD
// =============================================================================

// GetRoleDefaults lista los defaults de feature para un rol.
func (r *FeatureLimitRepository) GetRoleDefaults(ctx context.Context, roleID uuid.UUID) ([]featurelimits.FeatureLimitRow, error) {
	query := `SELECT feature_key, limit_value, "window", created_at, updated_at
	          FROM default_feature_limits WHERE role_id = $1 ORDER BY feature_key`

	rows, err := r.pool.Query(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("get role defaults: %w", err)
	}
	defer rows.Close()

	var limits []featurelimits.FeatureLimitRow
	for rows.Next() {
		var l featurelimits.FeatureLimitRow
		if scanErr := rows.Scan(&l.FeatureKey, &l.LimitValue, &l.Window, &l.CreatedAt, &l.UpdatedAt); scanErr != nil {
			return nil, fmt.Errorf("scan role default: %w", scanErr)
		}
		limits = append(limits, l)
	}

	return limits, rows.Err()
}

// SetRoleDefault crea o actualiza un default de feature para un rol.
func (r *FeatureLimitRepository) SetRoleDefault(ctx context.Context, roleID uuid.UUID, featureKey string, limitValue *int, window string) error {
	if window == "" {
		window = "month"
	}

	if limitValue == nil {
		return fmt.Errorf("%w: role default requires a non-nil limit_value", domain.ErrInvalidInput)
	}

	query := `INSERT INTO default_feature_limits (role_id, feature_key, "window", limit_value, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, NOW(), NOW())
	          ON CONFLICT (role_id, feature_key, "window") DO UPDATE
	          SET limit_value = EXCLUDED.limit_value, updated_at = NOW()`

	if _, err := r.pool.Exec(ctx, query, roleID, featureKey, window, *limitValue); err != nil {
		return fmt.Errorf("set role default: %w", err)
	}

	return nil
}

// DeleteRoleDefault elimina un default de feature de un rol.
func (r *FeatureLimitRepository) DeleteRoleDefault(ctx context.Context, roleID uuid.UUID, featureKey string) error {
	query := `DELETE FROM default_feature_limits WHERE role_id = $1 AND feature_key = $2`

	ct, err := r.pool.Exec(ctx, query, roleID, featureKey)
	if err != nil {
		return fmt.Errorf("delete role default: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrFeatureLimitNotFound
	}

	return nil
}

// =============================================================================
// Effective Limit Resolution (FeatureLimitResolutionRepo)
// =============================================================================

// GetUserLimitValue obtiene el valor del límite de feature para un usuario.
// Retorna nil si no existe límite para el usuario.
func (r *FeatureLimitRepository) GetUserLimitValue(ctx context.Context, userID uuid.UUID, featureKey string) (*int, error) {
	query := `SELECT limit_value FROM user_feature_limits
	          WHERE user_id = $1 AND feature_key = $2
	          ORDER BY "window" LIMIT 1`

	var limitValue *int
	err := r.pool.QueryRow(ctx, query, userID, featureKey).Scan(&limitValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user limit value: %w", err)
	}

	return limitValue, nil
}

// GetRoleDefaultValue obtiene el valor del default de feature para un rol.
// Retorna nil si no existe default para el rol.
func (r *FeatureLimitRepository) GetRoleDefaultValue(ctx context.Context, roleID uuid.UUID, featureKey string) (*int, error) {
	query := `SELECT limit_value FROM default_feature_limits
	          WHERE role_id = $1 AND feature_key = $2
	          ORDER BY "window" LIMIT 1`

	var limitValue int
	err := r.pool.QueryRow(ctx, query, roleID, featureKey).Scan(&limitValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get role default value: %w", err)
	}

	return &limitValue, nil
}

// Compile-time check: FeatureLimitRepository implementa ambas interfaces.
var _ featurelimits.FeatureLimitRepo = (*FeatureLimitRepository)(nil)
var _ featurelimits.FeatureLimitResolutionRepo = (*FeatureLimitRepository)(nil)

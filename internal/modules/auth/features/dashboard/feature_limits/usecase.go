// Lógica de negocio para CRUD de feature limits desde el dashboard.
// Orquesta validación y operaciones de DB para límites de usuario.
package feature_limits

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// FeatureLimitRepo es el puerto local para operaciones CRUD de feature limits.
// Implementado por el adapter postgres.
type FeatureLimitRepo interface {
	// User limits
	GetUserLimits(ctx context.Context, userID uuid.UUID) ([]FeatureLimitRow, error)
	SetUserLimit(ctx context.Context, userID uuid.UUID, featureKey string, limitValue *int, window string) (isCreated bool, err error)
	DeleteUserLimit(ctx context.Context, userID uuid.UUID, featureKey string) error

	// Effective limit resolution (para FeatureLimitService)
	GetUserLimitValue(ctx context.Context, userID uuid.UUID, featureKey string) (*int, error)
	GetRoleDefaultValue(ctx context.Context, roleID uuid.UUID, featureKey string) (*int, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta el CRUD de feature limits.
type UseCase struct {
	repo FeatureLimitRepo
}

// NewUseCase crea un nuevo use case de feature limits.
func NewUseCase(repo FeatureLimitRepo) *UseCase {
	return &UseCase{repo: repo}
}

// =============================================================================
// User Limits
// =============================================================================

// GetUserLimits lista los límites de feature para un usuario.
func (uc *UseCase) GetUserLimits(ctx context.Context, cmd GetUserLimitsCommand) (*FeatureLimitsListResponse, error) {
	rows, err := uc.repo.GetUserLimits(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user feature limits: %w", err)
	}

	limits := make([]FeatureLimitResponse, len(rows))
	for i, row := range rows {
		limits[i] = FeatureLimitResponse{
			FeatureKey: row.FeatureKey,
			LimitValue: row.LimitValue,
			Window:     row.Window,
		}
	}

	return &FeatureLimitsListResponse{Limits: limits}, nil
}

// SetUserLimit crea o actualiza un límite de feature para un usuario.
// Retorna isCreated=true si fue INSERT (no existía), false si fue UPDATE.
func (uc *UseCase) SetUserLimit(ctx context.Context, cmd SetUserLimitCommand) (*FeatureLimitResponse, bool, error) {
	if err := cmd.Validate(); err != nil {
		return nil, false, err
	}

	isCreated, err := uc.repo.SetUserLimit(ctx, cmd.UserID, cmd.FeatureKey, cmd.LimitValue, cmd.Window)
	if err != nil {
		return nil, false, fmt.Errorf("set user feature limit: %w", err)
	}

	return &FeatureLimitResponse{
		FeatureKey: cmd.FeatureKey,
		LimitValue: cmd.LimitValue,
		Window:     cmd.Window,
	}, isCreated, nil
}

// DeleteUserLimit elimina un límite de feature de un usuario.
func (uc *UseCase) DeleteUserLimit(ctx context.Context, cmd DeleteUserLimitCommand) error {
	if err := uc.repo.DeleteUserLimit(ctx, cmd.UserID, cmd.FeatureKey); err != nil {
		return fmt.Errorf("delete user feature limit: %w", err)
	}
	return nil
}

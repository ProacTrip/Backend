// Lógica de negocio para CRUD de feature limits desde el dashboard.
// Orquesta validación y operaciones de DB para límites de usuario y defaults de rol.
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
	SetUserLimit(ctx context.Context, userID uuid.UUID, featureKey string, limitValue *int, window string) error
	DeleteUserLimit(ctx context.Context, userID uuid.UUID, featureKey string) error

	// Role defaults
	GetRoleDefaults(ctx context.Context, roleID uuid.UUID) ([]FeatureLimitRow, error)
	SetRoleDefault(ctx context.Context, roleID uuid.UUID, featureKey string, limitValue *int, window string) error
	DeleteRoleDefault(ctx context.Context, roleID uuid.UUID, featureKey string) error

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
// FL-SPEC-001: duplicado (mismo user+feature+window) → 409 conflict.
func (uc *UseCase) SetUserLimit(ctx context.Context, cmd SetUserLimitCommand) (*FeatureLimitResponse, error) {
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	if err := uc.repo.SetUserLimit(ctx, cmd.UserID, cmd.FeatureKey, cmd.LimitValue, cmd.Window); err != nil {
		return nil, fmt.Errorf("set user feature limit: %w", err)
	}

	return &FeatureLimitResponse{
		FeatureKey: cmd.FeatureKey,
		LimitValue: cmd.LimitValue,
		Window:     cmd.Window,
	}, nil
}

// DeleteUserLimit elimina un límite de feature de un usuario.
func (uc *UseCase) DeleteUserLimit(ctx context.Context, cmd DeleteUserLimitCommand) error {
	if err := uc.repo.DeleteUserLimit(ctx, cmd.UserID, cmd.FeatureKey); err != nil {
		return fmt.Errorf("delete user feature limit: %w", err)
	}
	return nil
}

// =============================================================================
// Role Defaults
// =============================================================================

// GetRoleDefaults lista los defaults de feature para un rol.
func (uc *UseCase) GetRoleDefaults(ctx context.Context, cmd GetRoleDefaultsCommand) (*FeatureLimitsListResponse, error) {
	rows, err := uc.repo.GetRoleDefaults(ctx, cmd.RoleID)
	if err != nil {
		return nil, fmt.Errorf("get role feature defaults: %w", err)
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

// SetRoleDefault crea o actualiza un default de feature para un rol.
func (uc *UseCase) SetRoleDefault(ctx context.Context, cmd SetRoleDefaultCommand) (*FeatureLimitResponse, error) {
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	if err := uc.repo.SetRoleDefault(ctx, cmd.RoleID, cmd.FeatureKey, cmd.LimitValue, cmd.Window); err != nil {
		return nil, fmt.Errorf("set role feature default: %w", err)
	}

	return &FeatureLimitResponse{
		FeatureKey: cmd.FeatureKey,
		LimitValue: cmd.LimitValue,
		Window:     cmd.Window,
	}, nil
}

// DeleteRoleDefault elimina un default de feature de un rol.
func (uc *UseCase) DeleteRoleDefault(ctx context.Context, cmd DeleteRoleDefaultCommand) error {
	if err := uc.repo.DeleteRoleDefault(ctx, cmd.RoleID, cmd.FeatureKey); err != nil {
		return fmt.Errorf("delete role feature default: %w", err)
	}
	return nil
}

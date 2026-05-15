// DTOs de entrada para CRUD de feature limits desde el dashboard.
// Endpoints: GET/POST/DELETE para usuarios, GET/POST para roles.
package feature_limits

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// SetUserLimitCommand — POST /v1/dashboard/users/:id/feature-limits
// =============================================================================

// SetUserLimitCommand es el DTO para crear/actualizar un límite de feature por usuario.
type SetUserLimitCommand struct {
	UserID     uuid.UUID
	FeatureKey string
	LimitValue *int   // nil = ilimitado, 0 = bloqueado, >0 = cuota
	Window     string // opcional: "minute", "hour", "day", "month"
}

// Validate rechaza campos requeridos ausentes.
func (cmd *SetUserLimitCommand) Validate() error {
	if cmd.UserID == uuid.Nil {
		return fmt.Errorf("%w: user ID is required", domain.ErrInvalidInput)
	}
	if cmd.FeatureKey == "" {
		return fmt.Errorf("%w: feature_key is required", domain.ErrInvalidInput)
	}
	return nil
}

// =============================================================================
// DeleteUserLimitCommand — DELETE /v1/dashboard/users/:id/feature-limits/:key
// =============================================================================

// DeleteUserLimitCommand es el DTO para eliminar un límite de feature de un usuario.
type DeleteUserLimitCommand struct {
	UserID     uuid.UUID
	FeatureKey string
}

// GetUserLimitsCommand es el DTO para listar límites de feature de un usuario.
type GetUserLimitsCommand struct {
	UserID uuid.UUID
}

// =============================================================================
// SetRoleDefaultCommand — POST /v1/dashboard/roles/:id/feature-limits
// =============================================================================

// SetRoleDefaultCommand es el DTO para crear/actualizar un default de feature por rol.
type SetRoleDefaultCommand struct {
	RoleID     uuid.UUID
	FeatureKey string
	LimitValue *int
	Window     string
}

// Validate rechaza campos requeridos ausentes.
func (cmd *SetRoleDefaultCommand) Validate() error {
	if cmd.RoleID == uuid.Nil {
		return fmt.Errorf("%w: role ID is required", domain.ErrInvalidInput)
	}
	if cmd.FeatureKey == "" {
		return fmt.Errorf("%w: feature_key is required", domain.ErrInvalidInput)
	}
	return nil
}

// GetRoleDefaultsCommand es el DTO para listar defaults de feature de un rol.
type GetRoleDefaultsCommand struct {
	RoleID uuid.UUID
}

// DeleteRoleDefaultCommand es el DTO para eliminar un default de feature de un rol.
type DeleteRoleDefaultCommand struct {
	RoleID     uuid.UUID
	FeatureKey string
}

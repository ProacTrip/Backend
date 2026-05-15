// DTOs de entrada para CRUD de permission overrides desde el dashboard.
// Endpoints: GET/POST/DELETE /v1/dashboard/users/:id/permission-overrides.
package permission_overrides

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Constantes de validación
// =============================================================================

const (
	// MaxReasonLength es la longitud máxima de la razón (PO-SPEC-006).
	MaxReasonLength = 500

	// MaxBlockDuration es la duración máxima de un deny (PO-SPEC-007).
	// Deny con expiración > 365 días desde ahora → ErrInvalidBlockDuration.
	MaxBlockDuration = 365 * 24 * time.Hour
)

// =============================================================================
// CreateOverrideCommand — POST /v1/dashboard/users/:id/permission-overrides
// =============================================================================

// CreateOverrideCommand es el DTO para crear un override de permiso.
type CreateOverrideCommand struct {
	UserID       uuid.UUID
	PermissionID uuid.UUID
	Granted      bool
	ExpiresAt    *time.Time
	Reason       string
	ActorID      uuid.UUID
}

// Validate rechaza campos requeridos ausentes y aplica reglas de PO-SPEC-006 y PO-SPEC-007.
func (cmd *CreateOverrideCommand) Validate() error {
	if cmd.UserID == uuid.Nil {
		return fmt.Errorf("%w: user ID is required", domain.ErrInvalidInput)
	}
	if cmd.PermissionID == uuid.Nil {
		return fmt.Errorf("%w: permission ID is required", domain.ErrInvalidInput)
	}

	// PO-SPEC-006: razón requerida, no vacía, no solo whitespace, 1-500 chars
	if err := validateReason(cmd.Reason); err != nil {
		return err
	}

	// Validar expiración si está seteada
	if cmd.ExpiresAt != nil {
		now := time.Now()

		// Expiración en el pasado → error
		if cmd.ExpiresAt.Before(now) {
			return fmt.Errorf("%w: expires_at must be in the future", domain.ErrInvalidInput)
		}

		// PO-SPEC-007: deny con duración > 365 días → bloqueado
		if !cmd.Granted {
			maxExpiry := now.Add(MaxBlockDuration)
			if cmd.ExpiresAt.After(maxExpiry) {
				return domain.ErrInvalidBlockDuration
			}
		}
	}

	return nil
}

// =============================================================================
// ListOverridesCommand — GET /v1/dashboard/users/:id/permission-overrides
// =============================================================================

// ListOverridesCommand es el DTO para listar overrides de un usuario.
type ListOverridesCommand struct {
	UserID uuid.UUID
}

// =============================================================================
// DeleteOverrideCommand — DELETE .../permission-overrides/:overrideId
// =============================================================================

// DeleteOverrideCommand es el DTO para eliminar un override.
type DeleteOverrideCommand struct {
	OverrideID uuid.UUID
	UserID     uuid.UUID
	ActorID    uuid.UUID
}

// =============================================================================
// Helpers de validación
// =============================================================================

// validateReason valida que la razón cumpla con PO-SPEC-006:
// no vacía, no solo whitespace, 1-500 caracteres.
func validateReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return domain.ErrInvalidReason
	}
	if len(reason) > MaxReasonLength {
		return domain.ErrInvalidReason
	}
	return nil
}

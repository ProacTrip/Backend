// DTO de entrada para habilitar/deshabilitar cuentas desde el dashboard.
// PUT /v1/dashboard/users/:id/status.
package account_status

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Command
// =============================================================================

// EnableDisableCommand es el DTO de entrada para cambiar el estado de una cuenta.
// UserID es extraído del path param por el handler.
// Status debe ser "active" o "disabled" (AS-SPEC-003).
// ActorID es el usuario autenticado que ejecuta la acción.
type EnableDisableCommand struct {
	UserID  uuid.UUID
	Status  string
	ActorID uuid.UUID
}

// =============================================================================
// Validación
// =============================================================================

// Validate rechaza campos requeridos ausentes y estados no permitidos.
// AS-SPEC-003: solo "active" y "disabled" son transiciones válidas por este endpoint.
func (cmd *EnableDisableCommand) Validate() error {
	if cmd.UserID == uuid.Nil {
		return fmt.Errorf("%w: user ID is required", domain.ErrInvalidInput)
	}

	if cmd.Status == "" {
		return fmt.Errorf("%w: status is required", domain.ErrInvalidInput)
	}

	// Solo "active" y "disabled" son válidos (no suspended, no pending_verification)
	if cmd.Status != "active" && cmd.Status != "disabled" {
		return fmt.Errorf("%w: invalid status %q — solo active/disabled permitidos", domain.ErrInvalidInput, cmd.Status)
	}

	return nil
}

// =============================================================================
// Helpers de validación
// =============================================================================

// validTargetStatuses es el conjunto de estados válidos para transición por dashboard.
var validTargetStatuses = map[string]bool{
	"active":   true,
	"disabled": true,
}

// IsValidTargetStatus verifica si un status es válido para transición via dashboard.
func IsValidTargetStatus(s string) bool {
	return validTargetStatuses[s]
}

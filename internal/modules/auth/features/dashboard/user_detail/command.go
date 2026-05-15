// DTO de entrada para detalle de usuario del dashboard.
// Valida el userID (UUID de path param).
package user_detail

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Command
// =============================================================================

// Command is the input DTO for getting user detail via GET /v1/dashboard/users/:id.
// UserID is extracted from the path parameter by the handler.
type Command struct {
	UserID uuid.UUID
}

// =============================================================================
// Validación
// =============================================================================

// Validate rejects empty/zero UUID.
func (cmd *Command) Validate() error {
	if cmd.UserID == uuid.Nil {
		return fmt.Errorf("%w: user ID is required", domain.ErrInvalidInput)
	}
	return nil
}

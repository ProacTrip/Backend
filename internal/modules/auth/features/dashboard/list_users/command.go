// DTO de entrada para listado de usuarios del dashboard.
// Valida parámetros de filtro y paginación.
package list_users

import (
	"errors"
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	DefaultLimit    = 10
	MaxResultsLimit = 100
)

// =============================================================================
// Command
// =============================================================================

// Command is the input DTO for listing dashboard users via GET /v1/dashboard/users.
// All filter fields are optional; empty string / nil means "no filter".
type Command struct {
	Cursor        string `query:"cursor"`
	Limit         int    `query:"limit"`
	Role          string `query:"role"`
	Status        string `query:"status"`
	Search        string `query:"search"`
	CreatedBefore string `query:"created_before"`
	CreatedAfter  string `query:"created_after"`
}

// =============================================================================
// Validación
// =============================================================================

// Validate normalizes default values and rejects invalid parameters.
// Follows the pattern from search_flights/command.go: defaults set when zero,
// explicit range checks, invalid enums rejected.
func (cmd *Command) Validate() error {
	// Default limit
	if cmd.Limit == 0 {
		cmd.Limit = DefaultLimit
	}
	if cmd.Limit < 1 || cmd.Limit > MaxResultsLimit {
		return fmt.Errorf("%w: limit must be between 1 and %d", domain.ErrInvalidInput, MaxResultsLimit)
	}

	// Validate status if provided (DU-SPEC-002: invalid status returns 400)
	if cmd.Status != "" {
		if !isValidUserStatus(cmd.Status) {
			return fmt.Errorf("%w: invalid status: %s", domain.ErrInvalidInput, cmd.Status)
		}
	}

	return nil
}

// =============================================================================
// Mapeo de status públicos a valores DB
// =============================================================================

// StatusToDB mapea el valor público del query param al valor de DB.
// "unverified" → "pending_verification", los demás pasan directo.
func StatusToDB(status string) string {
	switch status {
	case "unverified":
		return string(domain.StatusPendingVerification)
	default:
		return status
	}
}

// =============================================================================
// Helpers de validación
// =============================================================================

// validUserStatuses es el conjunto de valores de status aceptados en el query param.
// Solo 3 valores públicos: "unverified", "active", "disabled".
var validUserStatuses = map[string]bool{
	"unverified": true,
	"active":     true,
	"disabled":   true,
}

func isValidUserStatus(s string) bool {
	return validUserStatuses[s]
}

// =============================================================================
// Errores locales
// =============================================================================

var ErrInvalidStatus = errors.New("INVALID_STATUS: el estado proporcionado no es válido")

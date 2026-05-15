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
// Helpers de validación
// =============================================================================

// validUserStatuses is the set of valid user status values for filtering.
// Matches the CHECK constraint in the users table.
var validUserStatuses = map[string]bool{
	string(domain.StatusPendingVerification): true,
	string(domain.StatusActive):              true,
	string(domain.StatusInactive):            true,
	string(domain.StatusSuspended):           true,
	string(domain.StatusLocked):              true,
	string(domain.StatusDisabled):            true,
}

func isValidUserStatus(s string) bool {
	return validUserStatuses[s]
}

// =============================================================================
// Errores locales
// =============================================================================

var ErrInvalidStatus = errors.New("INVALID_STATUS: el estado proporcionado no es válido")

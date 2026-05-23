// Test: logout Command.Validate() — validación del token.
package logout_test

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/logout"
)

// =============================================================================
// Test: Validate — token vacío retorna error
// =============================================================================

func TestCommand_Validate_TokenVacio(t *testing.T) {
	cmd := logout.Command{Token: ""}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("esperaba error para token vacío")
	}
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("esperaba ErrInvalidInput, obtuve %v", err)
	}
}

// =============================================================================
// Test: Validate — token no vacío es válido

func TestCommand_Validate_TokenValido(t *testing.T) {
	cmd := logout.Command{Token: "v5.local.some-refresh-token"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("esperaba nil, obtuve %v", err)
	}
}

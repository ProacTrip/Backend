// Test: verify_email Command.Validate() — validación del token de verificación.
package verify_email

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Test: Validate — token vacío retorna error
// =============================================================================

func TestCommand_Validate_TokenVacio(t *testing.T) {
	cmd := Command{Token: ""}
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
// =============================================================================

func TestCommand_Validate_TokenValido(t *testing.T) {
	cmd := Command{Token: "v5.local.some-verification-token"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("esperaba nil, obtuve %v", err)
	}
}

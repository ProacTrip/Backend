// Test: oauth/callback Command.Validate() — validación de code y state.
package callback

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Test: Validate — code vacío retorna error
// =============================================================================

func TestCommand_Validate_CodeVacio(t *testing.T) {
	cmd := Command{ProviderCode: "", State: "valid-state"}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("esperaba error para code vacío")
	}
	if !errors.Is(err, domain.ErrOAuthCodeMissing) {
		t.Errorf("esperaba ErrOAuthCodeMissing, obtuve %v", err)
	}
}

// =============================================================================
// Test: Validate — state vacío retorna error
// =============================================================================

func TestCommand_Validate_StateVacio(t *testing.T) {
	cmd := Command{ProviderCode: "valid-code", State: ""}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("esperaba error para state vacío")
	}
	if !errors.Is(err, domain.ErrOAuthStateMissing) {
		t.Errorf("esperaba ErrOAuthStateMissing, obtuve %v", err)
	}
}

// =============================================================================
// Test: Validate — ambos vacíos retorna error (code primero)
// =============================================================================

func TestCommand_Validate_AmbosVacios(t *testing.T) {
	cmd := Command{ProviderCode: "", State: ""}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("esperaba error para code y state vacíos")
	}
	if !errors.Is(err, domain.ErrOAuthCodeMissing) {
		t.Errorf("esperaba ErrOAuthCodeMissing (se valida code primero), obtuve %v", err)
	}
}

// =============================================================================
// Test: Validate — ambos no vacíos es válido
// =============================================================================

func TestCommand_Validate_AmbosValidos(t *testing.T) {
	cmd := Command{ProviderCode: "4/0AbcdEfghIjkl", State: "v5.local.state-token"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("esperaba nil, obtuve %v", err)
	}
}

// Test: oauth/authorize Command.Validate() — validación del provider.
package authorize

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Test: Validate — provider vacío retorna error
// =============================================================================

func TestCommand_Validate_ProviderVacio(t *testing.T) {
	cmd := Command{Provider: ""}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("esperaba error para provider vacío")
	}
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("esperaba ErrInvalidInput, obtuve %v", err)
	}
}

// =============================================================================
// Test: Validate — provider no vacío es válido
// =============================================================================

func TestCommand_Validate_ProviderValido(t *testing.T) {
	tests := []struct {
		nombre   string
		provider string
	}{
		{"google", "google"},
		{"github", "github"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			cmd := Command{Provider: tt.provider}
			err := cmd.Validate()
			if err != nil {
				t.Errorf("esperaba nil para provider %q, obtuve %v", tt.provider, err)
			}
		})
	}
}

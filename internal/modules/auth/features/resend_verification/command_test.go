// Test: resend_verification Command.Validate() — validación del email.
package resend_verification

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Test: Validate — email vacío retorna error
// =============================================================================

func TestCommand_Validate_EmailVacio(t *testing.T) {
	cmd := Command{Email: ""}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("esperaba error para email vacío")
	}
	if !errors.Is(err, domain.ErrInvalidEmail) {
		t.Errorf("esperaba ErrInvalidEmail, obtuve %v", err)
	}
}

// =============================================================================
// Test: Validate — email inválido (sin @)
// =============================================================================

func TestCommand_Validate_EmailInvalido(t *testing.T) {
	tests := []struct {
		nombre string
		email  string
	}{
		{"sin arroba", "noesunemail"},
		{"solo arroba", "@"},
		{"sin dominio", "usuario@"},
		{"sin usuario", "@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			cmd := Command{Email: tt.email}
			err := cmd.Validate()
			if err == nil {
				t.Errorf("esperaba error para email %q", tt.email)
			}
			if !errors.Is(err, domain.ErrInvalidEmail) {
				t.Errorf("esperaba ErrInvalidEmail, obtuve %v", err)
			}
		})
	}
}

// =============================================================================
// Test: Validate — email válido
// =============================================================================

func TestCommand_Validate_EmailValido(t *testing.T) {
	tests := []struct {
		nombre string
		email  string
	}{
		{"email simple", "usuario@example.com"},
		{"subdominio", "usuario@sub.example.com"},
		{"con tag", "usuario+tag@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			cmd := Command{Email: tt.email}
			err := cmd.Validate()
			if err != nil {
				t.Errorf("esperaba nil para %q, obtuve %v", tt.email, err)
			}
		})
	}
}

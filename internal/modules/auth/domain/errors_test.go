// Test: domain errors — unicidad de códigos de error.
// Verifica que todos los errores centinela tengan códigos únicos (sin colisiones).
package domain_test

import (
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// allErrors lista todos los errores centinela exportados del dominio auth.
// Debe mantenerse sincronizada con domain/errors.go.
var allErrors = []struct {
	nombre string
	err    error
}{
	// Usuario
	{"ErrUserNotFound", domain.ErrUserNotFound},
	{"ErrEmailAlreadyExists", domain.ErrEmailAlreadyExists},
	{"ErrUserAlreadyVerified", domain.ErrUserAlreadyVerified},
	{"ErrAccountLocked", domain.ErrAccountLocked},
	{"ErrAccountSuspended", domain.ErrAccountSuspended},
	{"ErrAccountInactive", domain.ErrAccountInactive},
	{"ErrAccountDisabled", domain.ErrAccountDisabled},
	{"ErrEmailNotVerified", domain.ErrEmailNotVerified},
	{"ErrAccountPending", domain.ErrAccountPending},

	// Credenciales y autenticación
	{"ErrInvalidCredentials", domain.ErrInvalidCredentials},
	{"ErrWeakPassword", domain.ErrWeakPassword},
	{"ErrPasswordMismatch", domain.ErrPasswordMismatch},
	{"ErrInvalidPassword", domain.ErrInvalidPassword},
	{"ErrPasswordTooShort", domain.ErrPasswordTooShort},

	// Autenticación
	{"ErrNotAuthenticated", domain.ErrNotAuthenticated},

	// Tokens
	{"ErrTokenExpired", domain.ErrTokenExpired},
	{"ErrTokenInvalid", domain.ErrTokenInvalid},
	{"ErrTokenRevoked", domain.ErrTokenRevoked},
	{"ErrTokenNotFound", domain.ErrTokenNotFound},
	{"ErrInvalidVerificationToken", domain.ErrInvalidVerificationToken},
	{"ErrSessionExpired", domain.ErrSessionExpired},
	{"ErrSessionNotFound", domain.ErrSessionNotFound},

	// OAuth
	{"ErrOAuthProviderNotFound", domain.ErrOAuthProviderNotFound},
	{"ErrOAuthCodeMissing", domain.ErrOAuthCodeMissing},
	{"ErrOAuthStateMissing", domain.ErrOAuthStateMissing},
	{"ErrOAuthStateInvalid", domain.ErrOAuthStateInvalid},
	{"ErrOAuthAccessDenied", domain.ErrOAuthAccessDenied},
	{"ErrOAuthExchangeFailed", domain.ErrOAuthExchangeFailed},

	// Identidad
	{"ErrIdentityNotFound", domain.ErrIdentityNotFound},
	{"ErrIdentityAlreadyExists", domain.ErrIdentityAlreadyExists},

	// MFA
	{"ErrMFARequired", domain.ErrMFARequired},
	{"ErrMFAInvalidCode", domain.ErrMFAInvalidCode},
	{"ErrMFANotEnabled", domain.ErrMFANotEnabled},
	{"ErrMFAAlreadyEnabled", domain.ErrMFAAlreadyEnabled},
	{"ErrMFAInvalidMethod", domain.ErrMFAInvalidMethod},
	{"ErrMFARequiredCode", domain.ErrMFARequiredCode},
	{"ErrMFACodeExpired", domain.ErrMFACodeExpired},
	{"ErrInvalidBackupCode", domain.ErrInvalidBackupCode},
	{"ErrMFAInvalidRecoveryCode", domain.ErrMFAInvalidRecoveryCode},
	{"ErrMFARecoveryCodesExhausted", domain.ErrMFARecoveryCodesExhausted},

	// Validación
	{"ErrInvalidEmail", domain.ErrInvalidEmail},
	{"ErrInvalidInput", domain.ErrInvalidInput},
	{"ErrValidationError", domain.ErrValidationError},

	// Roles y permisos
	{"ErrRoleNotFound", domain.ErrRoleNotFound},
	{"ErrPermissionNotFound", domain.ErrPermissionNotFound},
	{"ErrPermissionDenied", domain.ErrPermissionDenied},
	{"ErrFeatureLimitNotFound", domain.ErrFeatureLimitNotFound},
	{"ErrInvalidBlockDuration", domain.ErrInvalidBlockDuration},
	{"ErrInvalidReason", domain.ErrInvalidReason},
	{"ErrPermissionOverrideNotFound", domain.ErrPermissionOverrideNotFound},
}

// =============================================================================
// Test: todos los códigos de error son únicos (sin colisiones)
// =============================================================================

func TestErrorCodes_Unicos(t *testing.T) {
	codes := make(map[string]string) // code -> nombre del primer error

	for _, e := range allErrors {
		msg := e.err.Error()
		// Extraer el código de error: todo antes de ":"
		idx := strings.Index(msg, ":")
		if idx == -1 {
			t.Errorf("%s: Error() no contiene ':': %q", e.nombre, msg)
			continue
		}
		code := msg[:idx]

		if existing, ok := codes[code]; ok {
			t.Errorf("COLISIÓN: código %q usado por %s y %s", code, existing, e.nombre)
		} else {
			codes[code] = e.nombre
		}
	}

	t.Logf("Verificados %d códigos de error — sin colisiones", len(codes))
}

// =============================================================================
// Test: cada error tiene formato CODE: mensaje
// =============================================================================

func TestErrorCodes_FormatoCorrecto(t *testing.T) {
	for _, e := range allErrors {
		t.Run(e.nombre, func(t *testing.T) {
			msg := e.err.Error()
			if msg == "" {
				t.Error("Error() retornó string vacío")
			}
			if !strings.Contains(msg, ":") {
				t.Errorf("formato sin ':': %q", msg)
			}
			// El código debe ser UPPER_SNAKE_CASE
			idx := strings.Index(msg, ":")
			code := msg[:idx]
			for _, c := range code {
				if c != '_' && (c < 'A' || c > 'Z') {
					t.Errorf("código contiene carácter no permitido %q en %q", c, code)
					break
				}
			}
		})
	}
}

// =============================================================================
// Test: errores agrupados por comentario (verificación de estructura)
// =============================================================================

func TestErrorCodes_CantidadEsperada(t *testing.T) {
	// Debe haber al menos 46 errores (los definidos en errors.go)
	if len(allErrors) < 46 {
		t.Errorf("cantidad de errores: esperaba >= 46, obtuve %d", len(allErrors))
	}
	t.Logf("Total de errores centinela: %d", len(allErrors))
}

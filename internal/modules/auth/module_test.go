package auth_test

import (
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/auth"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// T4.4.1: registerAuthErrorMappings — no hace panic
// =============================================================================

func TestRegisterAuthErrorMappings_NoPanic(t *testing.T) {
	// Defer + recover para verificar que no hay panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("registerAuthErrorMappings hizo panic: %v", r)
		}
	}()

	auth.RegisterAuthErrorMappings()
}

// =============================================================================
// T4.4.2: Error mappers — verificar que errores de dominio mapean a HTTP correcto
// =============================================================================

func TestRegisterAuthErrorMappings_MapeaErroresDominioAHTTP(t *testing.T) {
	auth.RegisterAuthErrorMappings()

	tests := []struct {
		name       string
		err        error
		wantStatus int  // 0 significa que MapDomainError debe retornar nil
		wantNil    bool // true si el mapper debe retornar nil
	}{
		// Autenticación
		{name: "ErrNotAuthenticated", err: domain.ErrNotAuthenticated, wantStatus: 401},
		{name: "ErrTokenInvalid", err: domain.ErrTokenInvalid, wantStatus: 401},

		// Credenciales
		{name: "ErrInvalidCredentials", err: domain.ErrInvalidCredentials, wantStatus: 401},
		{name: "ErrInvalidPassword", err: domain.ErrInvalidPassword, wantStatus: 400},
		{name: "ErrPasswordTooShort", err: domain.ErrPasswordTooShort, wantStatus: 400},
		{name: "ErrWeakPassword", err: domain.ErrWeakPassword, wantStatus: 400},
		{name: "ErrPasswordMismatch", err: domain.ErrPasswordMismatch, wantStatus: 400},

		// Verificación de email
		{name: "ErrEmailNotVerified", err: domain.ErrEmailNotVerified, wantStatus: 401},
		{name: "ErrAccountPending", err: domain.ErrAccountPending, wantStatus: 401},
		{name: "ErrUserAlreadyVerified", err: domain.ErrUserAlreadyVerified, wantStatus: 409},
		{name: "ErrInvalidVerificationToken", err: domain.ErrInvalidVerificationToken, wantStatus: 401},
		{name: "ErrTokenExpired", err: domain.ErrTokenExpired, wantStatus: 401},
		{name: "ErrTokenRevoked", err: domain.ErrTokenRevoked, wantStatus: 401},
		{name: "ErrTokenNotFound", err: domain.ErrTokenNotFound, wantStatus: 401},

		// Cuenta
		{name: "ErrAccountLocked", err: domain.ErrAccountLocked, wantStatus: 429},
		{name: "ErrAccountSuspended", err: domain.ErrAccountSuspended, wantStatus: 403},
		{name: "ErrAccountInactive", err: domain.ErrAccountInactive, wantStatus: 403},
		{name: "ErrAccountDisabled", err: domain.ErrAccountDisabled, wantStatus: 403},
		{name: "ErrEmailAlreadyExists", err: domain.ErrEmailAlreadyExists, wantStatus: 409},
		{name: "ErrUserNotFound", err: domain.ErrUserNotFound, wantStatus: 404},

		// Sesión
		{name: "ErrSessionExpired", err: domain.ErrSessionExpired, wantStatus: 401},
		{name: "ErrSessionNotFound", err: domain.ErrSessionNotFound, wantStatus: 401},

		// Validación
		{name: "ErrInvalidEmail", err: domain.ErrInvalidEmail, wantStatus: 400},
		{name: "ErrInvalidInput", err: domain.ErrInvalidInput, wantStatus: 400},
		{name: "ErrValidationError", err: domain.ErrValidationError, wantStatus: 400},

		// OAuth
		{name: "ErrOAuthProviderNotFound", err: domain.ErrOAuthProviderNotFound, wantStatus: 400},
		{name: "ErrOAuthCodeMissing", err: domain.ErrOAuthCodeMissing, wantStatus: 400},
		{name: "ErrOAuthStateMissing", err: domain.ErrOAuthStateMissing, wantStatus: 400},
		{name: "ErrOAuthStateInvalid", err: domain.ErrOAuthStateInvalid, wantStatus: 400},
		{name: "ErrOAuthAccessDenied", err: domain.ErrOAuthAccessDenied, wantStatus: 400},
		{name: "ErrOAuthExchangeFailed", err: domain.ErrOAuthExchangeFailed, wantStatus: 401},

		// Identidad
		{name: "ErrIdentityAlreadyExists", err: domain.ErrIdentityAlreadyExists, wantStatus: 409},
		{name: "ErrIdentityNotFound", err: domain.ErrIdentityNotFound, wantStatus: 404},

		// Roles / Permisos
		{name: "ErrPermissionDenied", err: domain.ErrPermissionDenied, wantStatus: 403},
		{name: "ErrRoleNotFound", err: domain.ErrRoleNotFound, wantStatus: 404},
		{name: "ErrPermissionNotFound", err: domain.ErrPermissionNotFound, wantStatus: 404},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := serrors.MapDomainError(tt.err)

			if tt.wantNil {
				if problem != nil {
					t.Errorf("MapDomainError(%s) debería retornar nil, pero retornó status %d", tt.name, problem.Status)
				}
				return
			}

			if problem == nil {
				t.Errorf("MapDomainError(%s) retornó nil, esperaba status %d", tt.name, tt.wantStatus)
				return
			}

			if problem.Status != tt.wantStatus {
				t.Errorf("MapDomainError(%s).Status = %d, want %d", tt.name, problem.Status, tt.wantStatus)
			}

			// Campos obligatorios RFC 9457
			if problem.Type == "" {
				t.Errorf("MapDomainError(%s).Type está vacío", tt.name)
			}
			if problem.Title == "" {
				t.Errorf("MapDomainError(%s).Title está vacío", tt.name)
			}
			if problem.Detail == "" {
				t.Errorf("MapDomainError(%s).Detail está vacío", tt.name)
			}
			if problem.TraceID == "" {
				t.Errorf("MapDomainError(%s).TraceID está vacío", tt.name)
			}
		})
	}
}

// =============================================================================
// T4.4.3: DefaultTTLs — valores esperados
// =============================================================================

func TestDefaultTTLs_ValoresEsperados(t *testing.T) {
	access, refresh, emailVerif, passwordReset := auth.DefaultTTLs()

	if access != 15*time.Minute {
		t.Errorf("access TTL = %v, want 15m", access)
	}
	if refresh != 7*24*time.Hour {
		t.Errorf("refresh TTL = %v, want 7d", refresh)
	}
	if emailVerif != 24*time.Hour {
		t.Errorf("email verification TTL = %v, want 24h", emailVerif)
	}
	if passwordReset != 1*time.Hour {
		t.Errorf("password reset TTL = %v, want 1h", passwordReset)
	}
}

// =============================================================================
// T4.4.4: GeneratePasetoKey — 32 bytes aleatorios
// =============================================================================

func TestGeneratePasetoKey_Retorna32Bytes(t *testing.T) {
	key, err := auth.GeneratePasetoKey()
	if err != nil {
		t.Fatalf("GeneratePasetoKey devolvió error: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("GeneratePasetoKey length = %d, want 32 bytes", len(key))
	}
}

// =============================================================================
// T4.4.5: GeneratePasetoKey — dos llamadas producen claves diferentes
// =============================================================================

func TestGeneratePasetoKey_ClavesAleatorias(t *testing.T) {
	key1, err := auth.GeneratePasetoKey()
	if err != nil {
		t.Fatalf("primera llamada a GeneratePasetoKey falló: %v", err)
	}
	key2, err := auth.GeneratePasetoKey()
	if err != nil {
		t.Fatalf("segunda llamada a GeneratePasetoKey falló: %v", err)
	}

	// Probabilístico pero con 256 bits la colisión es imposible
	iguales := true
	for i := range key1 {
		if key1[i] != key2[i] {
			iguales = false
			break
		}
	}
	if iguales {
		t.Error("GeneratePasetoKey produjo la misma clave dos veces — posible problema de entropía")
	}
}

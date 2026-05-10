package auth

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Test: validation-related domain errors must produce ProblemTypeValidationError
// (not ProblemTypeBadRequest — compliance with RFC 9457 error taxonomy)
// =============================================================================

func TestErrorMapping_ValidationErrors_UseValidationErrorURI(t *testing.T) {
	// Ensure the mapper is registered (it registers in NewModule, but we call directly)
	registerAuthErrorMappings()

	tests := []struct {
		name        string
		err         error
		wantType    serrors.ProblemType
	}{
		{"ErrInvalidEmail", domain.ErrInvalidEmail, serrors.ProblemTypeValidationError},
		{"ErrInvalidInput", domain.ErrInvalidInput, serrors.ProblemTypeValidationError},
		{"ErrValidationError", domain.ErrValidationError, serrors.ProblemTypeValidationError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// MapDomainError iterates all registered mappers
			prob := serrors.MapDomainError(tt.err)
			if prob == nil {
				t.Fatalf("MapDomainError(%v) returned nil — mapper not registered?", tt.err)
			}
			if prob.Type != tt.wantType {
				t.Errorf("MapDomainError(%v).Type = %q, want %q", tt.err, prob.Type, tt.wantType)
			}
		})
	}
}

func TestErrorMapping_PasswordErrors_KeepBadRequest(t *testing.T) {
	registerAuthErrorMappings()

	tests := []struct {
		name     string
		err      error
		wantType serrors.ProblemType
	}{
		{"ErrInvalidPassword", domain.ErrInvalidPassword, serrors.ProblemTypeBadRequest},
		{"ErrPasswordTooShort", domain.ErrPasswordTooShort, serrors.ProblemTypeBadRequest},
		{"ErrWeakPassword", domain.ErrWeakPassword, serrors.ProblemTypeBadRequest},
		{"ErrOAuthProviderNotFound", domain.ErrOAuthProviderNotFound, serrors.ProblemTypeBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prob := serrors.MapDomainError(tt.err)
			if prob == nil {
				t.Fatalf("MapDomainError(%v) returned nil", tt.err)
			}
			if prob.Type != tt.wantType {
				t.Errorf("MapDomainError(%v).Type = %q, want %q", tt.err, prob.Type, tt.wantType)
			}
		})
	}
}

func TestErrorMapping_OtherErrors_Unchanged(t *testing.T) {
	registerAuthErrorMappings()

	tests := []struct {
		name     string
		err      error
		wantType serrors.ProblemType
	}{
		{"ErrNotAuthenticated", domain.ErrNotAuthenticated, serrors.ProblemTypeUnauthorized},
		{"ErrInvalidCredentials", domain.ErrInvalidCredentials, serrors.ProblemTypeUnauthorized},
		{"ErrEmailAlreadyExists", domain.ErrEmailAlreadyExists, serrors.ProblemTypeConflict},
		{"ErrUserNotFound", domain.ErrUserNotFound, serrors.ProblemTypeNotFound},
		{"ErrAccountLocked", domain.ErrAccountLocked, serrors.ProblemTypeTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prob := serrors.MapDomainError(tt.err)
			if prob == nil {
				t.Fatalf("MapDomainError(%v) returned nil", tt.err)
			}
			if prob.Type != tt.wantType {
				t.Errorf("MapDomainError(%v).Type = %q, want %q", tt.err, prob.Type, tt.wantType)
			}
		})
	}
}

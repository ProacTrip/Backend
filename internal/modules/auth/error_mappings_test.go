package auth

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Test: validation-related domain errors must produce specific RFC 9457 Problem types
// so the frontend can distinguish error codes properly.
// =============================================================================

func TestErrorMapping_ValidationErrors_UseValidationErrorURI(t *testing.T) {
	// Ensure the mapper is registered (it registers in NewModule, but we call directly)
	registerAuthErrorMappings()

	tests := []struct {
		name     string
		err      error
		wantType serrors.ProblemType
	}{
		{"ErrInvalidEmail", domain.ErrInvalidEmail, serrors.ProblemTypeInvalidEmail},
		{"ErrInvalidInput", domain.ErrInvalidInput, serrors.ProblemTypeInvalidInput},
		{"ErrValidationError", domain.ErrValidationError, serrors.ProblemTypeInvalidInput},
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
		{"ErrInvalidPassword", domain.ErrInvalidPassword, serrors.ProblemTypeWeakPassword},
		{"ErrPasswordTooShort", domain.ErrPasswordTooShort, serrors.ProblemTypeWeakPassword},
		{"ErrWeakPassword", domain.ErrWeakPassword, serrors.ProblemTypeWeakPassword},
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

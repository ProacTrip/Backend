package login_test

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/login"
)

// =============================================================================
// Test: login Command.Validate() — email and password validation
// =============================================================================

func TestCommand_Validate_EmptyEmail(t *testing.T) {
	cmd := login.Command{Email: "", Password: "password123"}
	err := cmd.Validate()
	if !errors.Is(err, domain.ErrInvalidEmail) {
		t.Errorf("expected ErrInvalidEmail, got %v", err)
	}
}

func TestCommand_Validate_InvalidEmail_NoAt(t *testing.T) {
	cmd := login.Command{Email: "notanemail", Password: "password123"}
	err := cmd.Validate()
	if !errors.Is(err, domain.ErrInvalidEmail) {
		t.Errorf("expected ErrInvalidEmail, got %v", err)
	}
}

func TestCommand_Validate_InvalidEmail_AtSignOnly(t *testing.T) {
	cmd := login.Command{Email: "@", Password: "password123"}
	err := cmd.Validate()
	if !errors.Is(err, domain.ErrInvalidEmail) {
		t.Errorf("expected ErrInvalidEmail for '@', got %v", err)
	}
}

func TestCommand_Validate_InvalidEmail_MissingDomain(t *testing.T) {
	cmd := login.Command{Email: "user@", Password: "password123"}
	err := cmd.Validate()
	if !errors.Is(err, domain.ErrInvalidEmail) {
		t.Errorf("expected ErrInvalidEmail for 'user@', got %v", err)
	}
}

func TestCommand_Validate_ValidEmail(t *testing.T) {
	cmd := login.Command{Email: "user@example.com", Password: "password123"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCommand_Validate_ValidEmail_Subdomain(t *testing.T) {
	cmd := login.Command{Email: "user+tag@sub.example.co.uk", Password: "password123"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for subdomain email, got %v", err)
	}
}

func TestCommand_Validate_EmptyPassword(t *testing.T) {
	cmd := login.Command{Email: "user@example.com", Password: ""}
	err := cmd.Validate()
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCommand_Validate_PasswordTooShort_NoLongerRejected(t *testing.T) {
	// Regression: short passwords are no longer rejected at validation level.
	// They pass Validate() and fail at credential verification (usecase).
	cmd := login.Command{Email: "user@example.com", Password: "1234567"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for short password (caught at credential level), got %v", err)
	}
}

func TestCommand_Validate_PasswordMinLength(t *testing.T) {
	cmd := login.Command{Email: "user@example.com", Password: "12345678"}
	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for 8-char password, got %v", err)
	}
}

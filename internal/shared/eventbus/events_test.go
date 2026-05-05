package eventbus

import (
	"testing"
)

// =============================================================================
// Tests for NewUserRegisteredEvent with env fields (Task 2.3)
// =============================================================================

func TestNewUserRegisteredEvent_AllFieldsIncludingEnv(t *testing.T) {
	event := NewUserRegisteredEvent(
		"user-123",
		"test@example.com",
		"verify-token-abc",
		"es",
		"ARS",
		"AR",
		"America/Argentina/Buenos_Aires",
	)

	if event.EventType != UserRegistered {
		t.Errorf("EventType = %q, want %q", event.EventType, UserRegistered)
	}
	if event.AggregateID != "user-123" {
		t.Errorf("AggregateID = %q, want %q", event.AggregateID, "user-123")
	}
	if event.Timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}

	payload := event.Payload
	if payload["user_id"] != "user-123" {
		t.Errorf("user_id = %q, want %q", payload["user_id"], "user-123")
	}
	if payload["email"] != "test@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "test@example.com")
	}
	if payload["verification_token"] != "verify-token-abc" {
		t.Errorf("verification_token = %q, want %q", payload["verification_token"], "verify-token-abc")
	}
	if payload["language_code"] != "es" {
		t.Errorf("language_code = %q, want %q", payload["language_code"], "es")
	}
	if payload["currency_code"] != "ARS" {
		t.Errorf("currency_code = %q, want %q", payload["currency_code"], "ARS")
	}
	if payload["country_code"] != "AR" {
		t.Errorf("country_code = %q, want %q", payload["country_code"], "AR")
	}
	if payload["timezone_name"] != "America/Argentina/Buenos_Aires" {
		t.Errorf("timezone_name = %q, want %q", payload["timezone_name"], "America/Argentina/Buenos_Aires")
	}
}

func TestNewUserRegisteredEvent_WithoutEnvFields(t *testing.T) {
	event := NewUserRegisteredEvent(
		"user-456",
		"noenv@example.com",
		"verify-token-xyz",
		"", "", "", "",
	)

	payload := event.Payload

	// Core fields must exist
	if payload["user_id"] != "user-456" {
		t.Errorf("user_id = %q, want %q", payload["user_id"], "user-456")
	}
	if payload["email"] != "noenv@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "noenv@example.com")
	}
	if payload["verification_token"] != "verify-token-xyz" {
		t.Errorf("verification_token = %q, want %q", payload["verification_token"], "verify-token-xyz")
	}

	// Env fields must NOT be present when empty
	if _, ok := payload["language_code"]; ok {
		t.Error("language_code should NOT be present when empty")
	}
	if _, ok := payload["currency_code"]; ok {
		t.Error("currency_code should NOT be present when empty")
	}
	if _, ok := payload["country_code"]; ok {
		t.Error("country_code should NOT be present when empty")
	}
	if _, ok := payload["timezone_name"]; ok {
		t.Error("timezone_name should NOT be present when empty")
	}
}

func TestNewUserRegisteredEvent_PartialEnvFields(t *testing.T) {
	// Resolver might return some fields but not all
	event := NewUserRegisteredEvent(
		"user-789",
		"partial@example.com",
		"token-123",
		"en", // language only
		"",   // no currency
		"US", // country only
		"",   // no timezone
	)

	payload := event.Payload

	if payload["language_code"] != "en" {
		t.Errorf("language_code = %q, want %q", payload["language_code"], "en")
	}
	if payload["country_code"] != "US" {
		t.Errorf("country_code = %q, want %q", payload["country_code"], "US")
	}
	if _, ok := payload["currency_code"]; ok {
		t.Error("currency_code should NOT be present when empty")
	}
	if _, ok := payload["timezone_name"]; ok {
		t.Error("timezone_name should NOT be present when empty")
	}
}

func TestNewUserRegisteredEvent_EmptyVerificationToken_Omitted(t *testing.T) {
	// No verification token — base behavior preserved
	event := NewUserRegisteredEvent("user-000", "no-token@example.com", "", "", "", "", "")

	payload := event.Payload

	if _, ok := payload["verification_token"]; ok {
		t.Error("verification_token should be omitted when empty")
	}
}

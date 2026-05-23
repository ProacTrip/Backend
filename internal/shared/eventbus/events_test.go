package eventbus_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Tests for NewUserRegisteredEvent with env fields (Task 2.3)
// =============================================================================

func TestNewUserRegisteredEvent_AllFieldsIncludingEnv(t *testing.T) {
	event := eventbus.NewUserRegisteredEvent(
		"user-123",
		"test@example.com",
		"verify-token-abc",
		"es",
		"ARS",
		"AR",
		"America/Argentina/Buenos_Aires",
	)

	if event.EventType != eventbus.UserRegistered {
		t.Errorf("EventType = %q, want %q", event.EventType, eventbus.UserRegistered)
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
	event := eventbus.NewUserRegisteredEvent(
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
	event := eventbus.NewUserRegisteredEvent(
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
	event := eventbus.NewUserRegisteredEvent("user-000", "no-token@example.com", "", "", "", "", "")

	payload := event.Payload

	if _, ok := payload["verification_token"]; ok {
		t.Error("verification_token should be omitted when empty")
	}
}

// =============================================================================
// Tests para NewAccountDisabledEvent y NewAccountEnabledEvent
// Sin first_name — el template de Resend usa user_email
// =============================================================================

func TestNewAccountDisabledEvent_PayloadCompleto(t *testing.T) {
	event := eventbus.NewAccountDisabledEvent(
		"user-dis-001",
		"disabled@example.com",
		"admin-001",
	)

	if event.EventType != eventbus.AccountDisabled {
		t.Errorf("EventType = %q, want %q", event.EventType, eventbus.AccountDisabled)
	}
	if event.AggregateID != "user-dis-001" {
		t.Errorf("AggregateID = %q, want %q", event.AggregateID, "user-dis-001")
	}
	if event.Timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}

	payload := event.Payload
	if payload["user_id"] != "user-dis-001" {
		t.Errorf("user_id = %q, want %q", payload["user_id"], "user-dis-001")
	}
	if payload["email"] != "disabled@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "disabled@example.com")
	}
	if payload["disabled_by"] != "admin-001" {
		t.Errorf("disabled_by = %q, want %q", payload["disabled_by"], "admin-001")
	}

	// first_name NO debe estar en el payload
	if _, ok := payload["first_name"]; ok {
		t.Error("first_name should NOT be in payload")
	}
}

func TestNewAccountDisabledEvent_EmailAlwaysPresent(t *testing.T) {
	event := eventbus.NewAccountDisabledEvent(
		"user-dis-002",
		"nodata@example.com",
		"admin-002",
	)

	payload := event.Payload

	if payload["user_id"] != "user-dis-002" {
		t.Errorf("user_id = %q, want %q", payload["user_id"], "user-dis-002")
	}
	if payload["email"] != "nodata@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "nodata@example.com")
	}
	if payload["disabled_by"] != "admin-002" {
		t.Errorf("disabled_by = %q, want %q", payload["disabled_by"], "admin-002")
	}
}

func TestNewAccountEnabledEvent_PayloadCompleto(t *testing.T) {
	event := eventbus.NewAccountEnabledEvent(
		"user-en-001",
		"enabled@example.com",
		"admin-003",
	)

	if event.EventType != eventbus.AccountEnabled {
		t.Errorf("EventType = %q, want %q", event.EventType, eventbus.AccountEnabled)
	}
	if event.AggregateID != "user-en-001" {
		t.Errorf("AggregateID = %q, want %q", event.AggregateID, "user-en-001")
	}
	if event.Timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}

	payload := event.Payload
	if payload["user_id"] != "user-en-001" {
		t.Errorf("user_id = %q, want %q", payload["user_id"], "user-en-001")
	}
	if payload["email"] != "enabled@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "enabled@example.com")
	}
	if payload["enabled_by"] != "admin-003" {
		t.Errorf("enabled_by = %q, want %q", payload["enabled_by"], "admin-003")
	}

	// first_name NO debe estar en el payload
	if _, ok := payload["first_name"]; ok {
		t.Error("first_name should NOT be in payload")
	}
}

func TestNewAccountEnabledEvent_EmailAlwaysPresent(t *testing.T) {
	event := eventbus.NewAccountEnabledEvent(
		"user-en-002",
		"nodata2@example.com",
		"admin-004",
	)

	payload := event.Payload

	if payload["user_id"] != "user-en-002" {
		t.Errorf("user_id = %q, want %q", payload["user_id"], "user-en-002")
	}
	if payload["email"] != "nodata2@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "nodata2@example.com")
	}
	if payload["enabled_by"] != "admin-004" {
		t.Errorf("enabled_by = %q, want %q", payload["enabled_by"], "admin-004")
	}
}

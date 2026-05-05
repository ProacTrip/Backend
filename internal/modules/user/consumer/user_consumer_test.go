package consumer_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/user/consumer"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Tests for extractEnvPrefs — the consumer helper that reads env fields
// from the event payload.
// =============================================================================

// Note: extractEnvPrefs is unexported in the consumer package.
// We test it via the domain.EnvPrefs type and the extractEnvPrefs function
// which is accessible from within the package. Since we're in consumer_test,
// we use a different approach: the test verifies the EnvPrefs.HasAny() method
// and the consumer's extraction logic indirectly.
//
// For direct testing, we test the extractEnvPrefs function by creating
// a test helper in the package's _test.go file that re-exports it,
// OR we test the EnvPrefs domain type directly.

func TestEnvPrefs_HasAny(t *testing.T) {
	tests := []struct {
		name string
		p    domain.EnvPrefs
		want bool
	}{
		{"all empty", domain.EnvPrefs{}, false},
		{"only language", domain.EnvPrefs{LanguageCode: "es"}, true},
		{"only currency", domain.EnvPrefs{CurrencyCode: "EUR"}, true},
		{"only country", domain.EnvPrefs{CountryCode: "AR"}, true},
		{"only timezone", domain.EnvPrefs{TimezoneName: "UTC"}, true},
		{"multiple", domain.EnvPrefs{LanguageCode: "es", CurrencyCode: "EUR", CountryCode: "AR"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.HasAny(); got != tt.want {
				t.Errorf("EnvPrefs.HasAny() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractEnvPrefs validates that the consumer correctly extracts
// environment fields from the event payload.
// We test this by creating a payload map that matches what comes from
// Dragonfly streams and verifying the EnvPrefs values.
//
// The extractEnvPrefs function is tested via an internal test helper
// pattern: we define a test-only export in the consumer package.
func TestExtractEnvPrefs_AllFieldsPresent(t *testing.T) {
	payload := map[string]interface{}{
		"user_id":       "550e8400-e29b-41d4-a716-446655440000",
		"email":         "test@example.com",
		"language_code": "pt",
		"currency_code": "BRL",
		"country_code":  "BR",
		"timezone_name": "America/Sao_Paulo",
	}

	prefs := consumer.ExtractEnvPrefsForTest(payload)

	if prefs.LanguageCode != "pt" {
		t.Errorf("LanguageCode = %q, want %q", prefs.LanguageCode, "pt")
	}
	if prefs.CurrencyCode != "BRL" {
		t.Errorf("CurrencyCode = %q, want %q", prefs.CurrencyCode, "BRL")
	}
	if prefs.CountryCode != "BR" {
		t.Errorf("CountryCode = %q, want %q", prefs.CountryCode, "BR")
	}
	if prefs.TimezoneName != "America/Sao_Paulo" {
		t.Errorf("TimezoneName = %q, want %q", prefs.TimezoneName, "America/Sao_Paulo")
	}
}

func TestExtractEnvPrefs_NoFieldsPresent_LegacyEvent(t *testing.T) {
	// Legacy event: only user_id and email, no env fields
	payload := map[string]interface{}{
		"user_id": "550e8400-e29b-41d4-a716-446655440000",
		"email":   "test@example.com",
	}

	prefs := consumer.ExtractEnvPrefsForTest(payload)

	if prefs.HasAny() {
		t.Errorf("expected HasAny=false for legacy event, got prefs=%+v", prefs)
	}
	if prefs.LanguageCode != "" {
		t.Errorf("LanguageCode = %q, want empty", prefs.LanguageCode)
	}
	if prefs.CurrencyCode != "" {
		t.Errorf("CurrencyCode = %q, want empty", prefs.CurrencyCode)
	}
	if prefs.CountryCode != "" {
		t.Errorf("CountryCode = %q, want empty", prefs.CountryCode)
	}
	if prefs.TimezoneName != "" {
		t.Errorf("TimezoneName = %q, want empty", prefs.TimezoneName)
	}
}

func TestExtractEnvPrefs_PartialFieldsPresent(t *testing.T) {
	// Only currency_code present (other env fields absent)
	payload := map[string]interface{}{
		"user_id":       "550e8400-e29b-41d4-a716-446655440000",
		"email":         "test@example.com",
		"currency_code": "JPY",
	}

	prefs := consumer.ExtractEnvPrefsForTest(payload)

	if !prefs.HasAny() {
		t.Fatal("expected HasAny=true when currency_code is present")
	}
	if prefs.CurrencyCode != "JPY" {
		t.Errorf("CurrencyCode = %q, want %q", prefs.CurrencyCode, "JPY")
	}
	if prefs.LanguageCode != "" {
		t.Errorf("LanguageCode = %q, want empty", prefs.LanguageCode)
	}
	if prefs.CountryCode != "" {
		t.Errorf("CountryCode = %q, want empty", prefs.CountryCode)
	}
	if prefs.TimezoneName != "" {
		t.Errorf("TimezoneName = %q, want empty", prefs.TimezoneName)
	}
}

// TestExtractEnvPrefs_EmptyStringsSkipped verifies that empty string values
// in the payload are treated as absent (not extracted).
func TestExtractEnvPrefs_EmptyStringsSkipped(t *testing.T) {
	payload := map[string]interface{}{
		"user_id":       "550e8400-e29b-41d4-a716-446655440000",
		"email":         "test@example.com",
		"language_code": "",
		"currency_code": "",
		"country_code":  "",
		"timezone_name": "",
	}

	prefs := consumer.ExtractEnvPrefsForTest(payload)

	if prefs.HasAny() {
		t.Errorf("expected HasAny=false for empty strings, got prefs=%+v", prefs)
	}
}

// TestExtractEnvPrefs_InvalidTypesIgnored verifies that non-string values
// (e.g. numbers) in env fields are silently ignored.
func TestExtractEnvPrefs_InvalidTypesIgnored(t *testing.T) {
	payload := map[string]interface{}{
		"user_id":       "550e8400-e29b-41d4-a716-446655440000",
		"email":         "test@example.com",
		"language_code": 123,   // integer, not string
		"currency_code": 978,   // integer, not string
		"country_code":  true,  // bool, not string
		"timezone_name": "UTC", // valid string
	}

	prefs := consumer.ExtractEnvPrefsForTest(payload)

	// Only timezone_name should be extracted (it's a valid string)
	if !prefs.HasAny() {
		t.Fatal("expected HasAny=true — timezone_name is a valid string")
	}
	if prefs.TimezoneName != "UTC" {
		t.Errorf("TimezoneName = %q, want %q", prefs.TimezoneName, "UTC")
	}
	if prefs.LanguageCode != "" {
		t.Errorf("LanguageCode = %q, want empty (non-string type ignored)", prefs.LanguageCode)
	}
}

// Ensure the event does NOT panic when env fields are missing entirely.
func TestNewUserRegisteredEvent_LegacyCompatibility(t *testing.T) {
	// Legacy event: no env fields at all
	event := eventbus.NewUserRegisteredEvent(
		"user-123",
		"test@example.com",
		"verification-token",
		"", "", "", "", // all env fields empty
	)

	// Verify the payload does not contain env keys with empty values
	if _, ok := event.Payload["language_code"]; ok {
		t.Error("event payload should NOT contain language_code when empty")
	}
	if _, ok := event.Payload["currency_code"]; ok {
		t.Error("event payload should NOT contain currency_code when empty")
	}
	if _, ok := event.Payload["country_code"]; ok {
		t.Error("event payload should NOT contain country_code when empty")
	}
	if _, ok := event.Payload["timezone_name"]; ok {
		t.Error("event payload should NOT contain timezone_name when empty")
	}
}

func TestNewUserRegisteredEvent_WithEnvFields(t *testing.T) {
	event := eventbus.NewUserRegisteredEvent(
		"user-123",
		"test@example.com",
		"verification-token",
		"es", "ARS", "AR", "America/Argentina/Buenos_Aires",
	)

	if v, ok := event.Payload["language_code"]; !ok || v != "es" {
		t.Errorf("payload language_code = %v, want 'es'", v)
	}
	if v, ok := event.Payload["currency_code"]; !ok || v != "ARS" {
		t.Errorf("payload currency_code = %v, want 'ARS'", v)
	}
	if v, ok := event.Payload["country_code"]; !ok || v != "AR" {
		t.Errorf("payload country_code = %v, want 'AR'", v)
	}
	if v, ok := event.Payload["timezone_name"]; !ok || v != "America/Argentina/Buenos_Aires" {
		t.Errorf("payload timezone_name = %v, want 'America/Argentina/Buenos_Aires'", v)
	}
}

package shared

import (
	"context"
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// =============================================================================
// Mock LocationProvider
// =============================================================================

type mockLocationProvider struct {
	location *domain.LocationData
	err      error
}

func (m *mockLocationProvider) ResolveIP(_ context.Context, _ string) (*domain.LocationData, error) {
	return m.location, m.err
}

// =============================================================================
// Tests for EnvironmentResolverAdapter
// =============================================================================

func TestResolveDefaults_Success_Argentina(t *testing.T) {
	t.Parallel()

	adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
		location: &domain.LocationData{
			CountryCode: "AR",
			Timezone:    "America/Argentina/Buenos_Aires",
		},
	})

	currency, language, countryCode, timezone, err := adapter.ResolveDefaults(t.Context(), "190.191.192.193")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currency != "ARS" {
		t.Errorf("currency = %q, want %q", currency, "ARS")
	}
	if language != "es" {
		t.Errorf("language = %q, want %q", language, "es")
	}
	if countryCode != "AR" {
		t.Errorf("countryCode = %q, want %q", countryCode, "AR")
	}
	if timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("timezone = %q, want %q", timezone, "America/Argentina/Buenos_Aires")
	}
}

func TestResolveDefaults_Success_Japan(t *testing.T) {
	t.Parallel()

	adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
		location: &domain.LocationData{
			CountryCode: "JP",
			Timezone:    "Asia/Tokyo",
		},
	})

	currency, language, countryCode, timezone, err := adapter.ResolveDefaults(t.Context(), "8.8.8.8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currency != "JPY" {
		t.Errorf("currency = %q, want %q", currency, "JPY")
	}
	if language != "ja" {
		t.Errorf("language = %q, want %q", language, "ja")
	}
	if countryCode != "JP" {
		t.Errorf("countryCode = %q, want %q", countryCode, "JP")
	}
	if timezone != "Asia/Tokyo" {
		t.Errorf("timezone = %q, want %q", timezone, "Asia/Tokyo")
	}
}

func TestResolveDefaults_UnknownCountry_EmptyCurrencyLanguage(t *testing.T) {
	t.Parallel()

	// Country code not in CountryMetadata — currency and language should be empty
	adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
		location: &domain.LocationData{
			CountryCode: "XX", // unknown country
			Timezone:    "UTC",
		},
	})

	currency, language, countryCode, timezone, err := adapter.ResolveDefaults(t.Context(), "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currency != "" {
		t.Errorf("currency = %q, want empty (unknown country)", currency)
	}
	if language != "" {
		t.Errorf("language = %q, want empty (unknown country)", language)
	}
	if countryCode != "XX" {
		t.Errorf("countryCode = %q, want %q", countryCode, "XX")
	}
	if timezone != "UTC" {
		t.Errorf("timezone = %q, want %q", timezone, "UTC")
	}
}

func TestResolveDefaults_ProviderError_ReturnsError(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("ipquery timeout")
	adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
		err: providerErr,
	})

	_, _, _, _, err := adapter.ResolveDefaults(t.Context(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, providerErr) {
		t.Errorf("error = %v, want %v", err, providerErr)
	}
}

func TestResolveDefaults_Spain(t *testing.T) {
	t.Parallel()

	adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
		location: &domain.LocationData{
			CountryCode: "ES",
			Timezone:    "Europe/Madrid",
		},
	})

	currency, language, countryCode, timezone, err := adapter.ResolveDefaults(t.Context(), "80.80.80.80")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if currency != "EUR" {
		t.Errorf("currency = %q, want %q", currency, "EUR")
	}
	if language != "es" {
		t.Errorf("language = %q, want %q", language, "es")
	}
	if countryCode != "ES" {
		t.Errorf("countryCode = %q, want %q", countryCode, "ES")
	}
	if timezone != "Europe/Madrid" {
		t.Errorf("timezone = %q, want %q", timezone, "Europe/Madrid")
	}
}

// =============================================================================
// Compile-time interface check — verifies EnvironmentResolverAdapter
// structurally satisfies register.EnvironmentResolver
// =============================================================================

// This is verified at the wiring point in bootstrap/app.go:
//   var _ register.EnvironmentResolver = (*shared.EnvironmentResolverAdapter)(nil)

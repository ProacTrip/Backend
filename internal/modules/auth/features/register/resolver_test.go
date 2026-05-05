package register

import (
	"context"
	"errors"
	"testing"
)

// =============================================================================
// Tests for EnvironmentResolver port interface
// =============================================================================

// mockResolver implements EnvironmentResolver for testing.
// Verifies the interface contract is satisfied at compile time.
var _ EnvironmentResolver = (*mockResolver)(nil)

type mockResolver struct {
	currency    string
	language    string
	countryCode string
	timezone    string
	err         error
}

func (m *mockResolver) ResolveDefaults(ctx context.Context, ip string) (currency, language, countryCode, timezone string, err error) {
	if m.err != nil {
		return "", "", "", "", m.err
	}
	return m.currency, m.language, m.countryCode, m.timezone, nil
}

func TestEnvironmentResolver_ResolvesDefaultsFromIP(t *testing.T) {
	ctx := context.Background()
	resolver := &mockResolver{
		currency:    "ARS",
		language:    "es",
		countryCode: "AR",
		timezone:    "America/Argentina/Buenos_Aires",
	}

	currency, language, countryCode, timezone, err := resolver.ResolveDefaults(ctx, "190.191.192.193")
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

func TestEnvironmentResolver_ReturnsError(t *testing.T) {
	ctx := context.Background()
	resolver := &mockResolver{
		err: errors.New("geoip unavailable"),
	}

	_, _, _, _, err := resolver.ResolveDefaults(ctx, "127.0.0.1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

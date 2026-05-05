package config

import (
	"os"
	"testing"
)

// =============================================================================
// Tests for DEFAULT_CURRENCY and DEFAULT_LANGUAGE env var loading
// =============================================================================

func TestDefaultCurrencyFromEnv(t *testing.T) {
	// Unset any pre-existing env var to test default
	os.Unsetenv("DEFAULT_CURRENCY")

	cfg := Load()

	if cfg.DefaultCurrency != "EUR" {
		t.Errorf("DefaultCurrency = %q, want %q (default when env not set)", cfg.DefaultCurrency, "EUR")
	}
}

func TestDefaultLanguageFromEnv(t *testing.T) {
	// Unset any pre-existing env var to test default
	os.Unsetenv("DEFAULT_LANGUAGE")

	cfg := Load()

	if cfg.DefaultLanguage != "es" {
		t.Errorf("DefaultLanguage = %q, want %q (default when env not set)", cfg.DefaultLanguage, "es")
	}
}

func TestDefaultCurrencyAndLanguageFromEnvOverride(t *testing.T) {
	// Set custom env values
	os.Setenv("DEFAULT_CURRENCY", "JPY")
	os.Setenv("DEFAULT_LANGUAGE", "ja")
	t.Cleanup(func() {
		os.Unsetenv("DEFAULT_CURRENCY")
		os.Unsetenv("DEFAULT_LANGUAGE")
	})

	cfg := Load()

	if cfg.DefaultCurrency != "JPY" {
		t.Errorf("DefaultCurrency = %q, want %q", cfg.DefaultCurrency, "JPY")
	}
	if cfg.DefaultLanguage != "ja" {
		t.Errorf("DefaultLanguage = %q, want %q", cfg.DefaultLanguage, "ja")
	}
}

func TestDefaultsAreIndependent(t *testing.T) {
	// Set only currency, not language — verify they don't interfere
	os.Setenv("DEFAULT_CURRENCY", "GBP")
	os.Unsetenv("DEFAULT_LANGUAGE")
	t.Cleanup(func() {
		os.Unsetenv("DEFAULT_CURRENCY")
	})

	cfg := Load()

	if cfg.DefaultCurrency != "GBP" {
		t.Errorf("DefaultCurrency = %q, want %q", cfg.DefaultCurrency, "GBP")
	}
	if cfg.DefaultLanguage != "es" {
		t.Errorf("DefaultLanguage = %q, want %q (should stay at default)", cfg.DefaultLanguage, "es")
	}
}

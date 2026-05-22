package ai_search_test

import (
	"os"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
)

// =============================================================================
// Feature flags — AR-008
// =============================================================================

func TestFeatureFlags_DefaultsAreFalse(t *testing.T) {
	// UseCaseDeps sin flags explícitos → todos false (zero value de bool)
	deps := ai_search.UseCaseDeps{}

	if deps.DiscoveryEnabled != false {
		t.Errorf("DiscoveryEnabled default = %v, want false", deps.DiscoveryEnabled)
	}
}

func TestFeatureFlags_CanBeSetTrue(t *testing.T) {
	deps := ai_search.UseCaseDeps{
		DiscoveryEnabled: true,
	}

	if !deps.DiscoveryEnabled {
		t.Error("DiscoveryEnabled should be true")
	}
}

func TestFeatureFlags_Mixed(t *testing.T) {
	deps := ai_search.UseCaseDeps{
		DiscoveryEnabled: false,
	}

	if deps.DiscoveryEnabled {
		t.Error("DiscoveryEnabled should be false")
	}
}

func TestFeatureFlags_EnvVarParsing(t *testing.T) {
	// Simula lectura de variables de entorno como se haría en module.go
	tests := []struct {
		name     string
		envVal   string
		expected bool
	}{
		{"vacía → false", "", false},
		{"true → true", "true", true},
		{"1 → true", "1", true},
		{"false → false", "false", false},
		{"0 → false", "0", false},
		{"cualquier_otro → false", "otro", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set env var
			envKey := "TEST_FEATURE_FLAG_PARSE"
			os.Setenv(envKey, tc.envVal)
			t.Cleanup(func() { os.Unsetenv(envKey) })

			result := parseEnvBool(envKey)
			if result != tc.expected {
				t.Errorf("parseEnvBool(%q) = %v, want %v", tc.envVal, result, tc.expected)
			}
		})
	}
}

// parseEnvBool replica la lógica que usará module.go para leer feature flags.
func parseEnvBool(key string) bool {
	val := os.Getenv(key)
	switch val {
	case "true", "1":
		return true
	default:
		return false
	}
}

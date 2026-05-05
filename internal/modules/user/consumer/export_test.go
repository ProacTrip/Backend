package consumer

import "github.com/ProacTrip/Backend/internal/modules/user/domain"

// ExtractEnvPrefsForTest exposes the unexported extractEnvPrefs for testing.
func ExtractEnvPrefsForTest(payload map[string]interface{}) domain.EnvPrefs {
	return extractEnvPrefs(payload)
}

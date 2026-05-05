package register

import "context"

// =============================================================================
// EnvironmentResolver port — abstraction to resolve defaults from IP.
// The auth module NEVER imports the environment module directly.
// This interface provides the inversion-of-dependency boundary.
// =============================================================================

// EnvironmentResolver resolves user preferences (currency, language, country code,
// timezone) from an IP address. The implementation lives in the environment module
// and is injected via the UseCaseDeps struct.
type EnvironmentResolver interface {
	ResolveDefaults(ctx context.Context, ip string) (currency, language, countryCode, timezone string, err error)
}

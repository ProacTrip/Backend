// Package shared provides cross-cutting adapters for the environment module.
//
// EnvironmentResolverAdapter bridges the environment module's geo-IP
// capabilities to the auth module's register.EnvironmentResolver interface.
// It implements register.EnvironmentResolver structurally — no import of the
// auth module is needed (Go's structural typing handles it).
package shared

import (
	"context"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// =============================================================================
// EnvironmentResolverAdapter — adapts LocationProvider → register.EnvironmentResolver
// =============================================================================

// EnvironmentResolverAdapter resolves currency, language, country code, and
// timezone from an IP address using the geo-IP LocationProvider and the
// static CountryMetadata map.
type EnvironmentResolverAdapter struct {
	locationProvider domain.LocationProvider
}

// NewEnvironmentResolverAdapter creates an adapter backed by the given LocationProvider.
func NewEnvironmentResolverAdapter(lp domain.LocationProvider) *EnvironmentResolverAdapter {
	return &EnvironmentResolverAdapter{locationProvider: lp}
}

// ResolveDefaults performs an IP geo-lookup and enriches the result with
// currency and language from the CountryMetadata map.
//
// Returns (currency, language, countryCode, timezone, error).
// When the country code is not found in CountryMetadata, currency and language
// are returned as empty strings — the caller (registration use case) treats
// them as "not resolved" and continues without env defaults.
func (a *EnvironmentResolverAdapter) ResolveDefaults(ctx context.Context, ip string) (currency, language, countryCode, timezone string, err error) {
	loc, err := a.locationProvider.ResolveIP(ctx, ip)
	if err != nil {
		return "", "", "", "", err
	}

	countryCode = loc.CountryCode
	timezone = loc.Timezone

	if info, ok := domain.CountryMetadata[countryCode]; ok {
		currency = info.Currency
		language = info.Language
	}
	// If country code is not in CountryMetadata, currency and language remain ""
	// which signals "not resolved" to the auth use case.

	return currency, language, countryCode, timezone, nil
}

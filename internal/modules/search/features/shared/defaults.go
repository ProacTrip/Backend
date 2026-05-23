// shared — utility helpers shared across search feature handlers.
// Provides ResolveSearchDefaults for 3-tier per-param resolution:
//   1. Explicit client params (always win per-param)
//   2. Authenticated profile prefs (profile:{userID}:prefs Dragonfly hash)
//   3. Config defaults (DEFAULT_CURRENCY, DEFAULT_LANGUAGE, DEFAULT_COUNTRY_CODE)
package shared

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	sharedUser "github.com/ProacTrip/Backend/internal/shared/user"
	searchshared "github.com/ProacTrip/Backend/internal/modules/search/shared"
)

// =============================================================================
// Auth context extraction
// =============================================================================

// userIDClaimer is satisfied by any claims struct that exposes a uuid-based user ID.
// Both token.AccessClaims and token.RefreshClaims satisfy this interface.
type userIDClaimer interface {
	GetUserID() uuid.UUID
}

// UserIDFromContext extracts the authenticated user ID from the Echo context.
// Reads "user_claims" set by the auth middleware and returns the user ID string.
// Returns empty string for anonymous requests (no claims or claims without user ID).
func UserIDFromContext(c *echo.Context) string {
	raw := c.Get("user_claims")
	if raw == nil {
		return ""
	}
	if claims, ok := raw.(userIDClaimer); ok {
		return claims.GetUserID().String()
	}
	return ""
}

// =============================================================================
// ResolveSearchDefaults — 3-tier per-param resolution
// =============================================================================

// SearchDefaultConfig holds the hardcoded fallback defaults for search params.
// CountryCode is used as the final fallback when env:{ip} cache is empty and
// no other location data is available (e.g., first request in local dev).
type SearchDefaultConfig struct {
	Currency    string // e.g. "EUR"
	Language    string // e.g. "es"
	CountryCode string // e.g. "AR" — default country for location fallback
}

// ResolveSearchDefaults resolves HL (language) and Currency for search requests.
// GL (country code) is NOT resolved here — it comes from the env:{ip} cache or
// DEFAULT_COUNTRY_CODE env var (see resolveLocationHint in ai_search/usecase.go).
// Each parameter is resolved independently:
//
//	Tier 1: Client-supplied explicit param (non-nil pointer wins for that param)
//	Tier 2: Authenticated user profile prefs (Dragonfly hash profile:{userID}:prefs)
//	        — used for HL and Currency only
//	Tier 3: Config fallback defaults
//
// Returns the resolved (gl, hl, currency) values. gl is always empty (no longer
// resolved by this function — see Phase 2 of ai-discovery-rewrite).
func ResolveSearchDefaults(
	ctx context.Context,
	rdb *redis.Client,
	userID string,
	clientIP string,
	explicitGL *string,
	explicitHL *string,
	explicitCurrency *string,
	cfg SearchDefaultConfig,
) (gl, hl, currency string) {
	// Tier 1: explicit params win per-param
	gl = searchshared.PtrOrEmpty(explicitGL)
	hl = searchshared.PtrOrEmpty(explicitHL)
	currency = searchshared.PtrOrEmpty(explicitCurrency)

	// If all three are already explicit, skip remaining tiers
	if explicitGL != nil && explicitHL != nil && explicitCurrency != nil {
		return
	}

	// Guard: if Redis is nil (tests, AI not configured), skip to Tier 3
	if rdb == nil {
		// GL is no longer resolved from config defaults — caller must
		// resolve via env cache or DEFAULT_COUNTRY_CODE env var.
		if hl == "" {
			hl = cfg.Language
		}
		if currency == "" {
			currency = cfg.Currency
		}
		return
	}

	// Tier 2: Authenticated user → profile prefs (HL and Currency only)
	if userID != "" {
		prefs, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
		if err != nil {
			slog.WarnContext(ctx, "resolve defaults: profile prefs lookup failed, falling through",
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
		}
		if prefs != nil {
			if hl == "" && prefs.Language != "" {
				hl = prefs.Language
			}
			if currency == "" && prefs.Currency != "" {
				currency = prefs.Currency
			}
		}
	}

	// Tier 3: Config fallback defaults
	// GL is no longer resolved from config defaults — see Phase 2 of
	// ai-discovery-rewrite. Country code resolution comes from env:{ip}
	// cache or DEFAULT_COUNTRY_CODE env var in the usecase layer.
	if hl == "" {
		hl = cfg.Language
	}
	if currency == "" {
		currency = cfg.Currency
	}

	return
}
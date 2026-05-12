// shared — utility helpers shared across search feature handlers.
// Provides ResolveSearchDefaults for the 4-tier priority:
//   1. Explicit client params (always win)
//   2. Authenticated profile prefs (profile:{userID}:prefs Dragonfly hash)
//   3. Anonymous environment cache (env:{ip} Dragonfly key)
//   4. Config defaults (DEFAULT_CURRENCY, DEFAULT_LANGUAGE, DEFAULT_COUNTRY_CODE)
package shared

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
	userprefs "github.com/ProacTrip/Backend/internal/modules/user/features/shared"
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
// ResolveSearchDefaults — 4-tier priority resolution
// =============================================================================

// SearchDefaultConfig holds the hardcoded fallback defaults for search params.
type SearchDefaultConfig struct {
	Currency    string // e.g. "EUR"
	Language    string // e.g. "es"
	CountryCode string // e.g. "AR"
}

// ResolveSearchDefaults resolves GL (country), HL (language), and Currency
// for search requests according to the 4-tier priority chain:
//
//	Tier 1: Client-supplied explicit params (any non-nil pointer wins)
//	Tier 2: Authenticated user profile prefs (Dragonfly hash profile:{userID}:prefs)
//	Tier 3: Anonymous environment cache (Dragonfly key env:{ip})
//	Tier 4: Config fallback defaults
//
// Returns the resolved (gl, hl, currency) values.
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
	// Tier 1: If ANY explicit param is set, the client wins completely.
	// This prevents mixed-mode where some defaults bleed through.
	if explicitGL != nil || explicitHL != nil || explicitCurrency != nil {
		return ptrOrEmpty(explicitGL), ptrOrEmpty(explicitHL), ptrOrEmpty(explicitCurrency)
	}

	// Guard: if Redis is nil (tests, AI not configured), skip to Tier 4 config fallback.
	if rdb == nil {
		return cfg.CountryCode, cfg.Language, cfg.Currency
	}

	// Tier 2: Authenticated user → profile prefs cache
	if userID != "" {
		cur, lang, country, _, found, err := userprefs.GetProfilePrefs(ctx, rdb, userID)
		if err != nil {
			slog.WarnContext(ctx, "resolve defaults: profile prefs lookup failed, falling through",
				slog.String("user_id", userID),
				slog.String("error", err.Error()),
			)
		}
		if found {
			// GL = country code, HL = language, Currency = currency
			return country, lang, cur
		}
	}

	// Tier 3: Anonymous → environment cache env:{ip}
	if clientIP != "" {
		if gl, hl, cur, ok := resolveFromEnvCache(ctx, rdb, clientIP); ok {
			return gl, hl, cur
		}
	}

	// Tier 4: Hardcoded config fallback
	return cfg.CountryCode, cfg.Language, cfg.Currency
}

// =============================================================================
// Env cache parsing — uses shared/environment DTO contract
// =============================================================================

func resolveFromEnvCache(ctx context.Context, rdb *redis.Client, ip string) (gl, hl, currency string, ok bool) {
	key := sharedEnv.CacheKey(ip)

	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			slog.WarnContext(ctx, "resolve defaults: env cache lookup failed",
				slog.String("ip", ip),
				slog.String("error", err.Error()),
			)
		}
		return "", "", "", false
	}
	if raw == "" {
		return "", "", "", false
	}

	var entry sharedEnv.CacheEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		slog.WarnContext(ctx, "resolve defaults: env cache unmarshal failed",
			slog.String("error", err.Error()),
		)
		return "", "", "", false
	}

	loc := entry.Location
	if loc.CountryCode == "" && loc.Currency == "" && loc.Language == "" {
		return "", "", "", false
	}

	return loc.CountryCode, loc.Language, loc.Currency, true
}

// ptrOrEmpty returns the dereferenced value or "" if nil.
func ptrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

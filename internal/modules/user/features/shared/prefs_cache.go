package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// ProfilePrefsCacheTTL is the TTL for cached profile preferences
	ProfilePrefsCacheTTL = 30 * time.Minute // 30 minutes

	// profilePrefsKeyPrefix is the cache key prefix
	profilePrefsKeyPrefix = "profile:"
	profilePrefsKeySuffix = ":prefs"
)

// GetProfilePrefs retrieves cached user preferences from Dragonfly.
// Returns (currency, language, countryCode, timezone, found, error).
// On cache miss, found=false and all string values are empty — the caller
// should fall back to the next tier (geoip or environment defaults).
func GetProfilePrefs(ctx context.Context, rdb *redis.Client, userID string) (currency, language, countryCode, timezone string, found bool, err error) {
	key := profilePrefsKeyPrefix + userID + profilePrefsKeySuffix

	fields, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return "", "", "", "", false, fmt.Errorf("get profile prefs: %w", err)
	}

	// No fields at all → cache miss
	if len(fields) == 0 {
		return "", "", "", "", false, nil
	}

	// Found — extract individual fields (may be partial)
	currency = fields["currency"]
	language = fields["language"]
	countryCode = fields["country_code"]
	timezone = fields["timezone"]

	return currency, language, countryCode, timezone, true, nil
}

// SetProfilePrefs stores user preferences in Dragonfly as a Hash.
// Cache key: profile:{userID}:prefs
// Fields: currency, language, country_code, timezone
func SetProfilePrefs(ctx context.Context, rdb *redis.Client, userID, currency, language, countryCode, timezone string) error {
	key := profilePrefsKeyPrefix + userID + profilePrefsKeySuffix

	fields := make(map[string]interface{}, 4)
	if currency != "" {
		fields["currency"] = currency
	}
	if language != "" {
		fields["language"] = language
	}
	if countryCode != "" {
		fields["country_code"] = countryCode
	}
	if timezone != "" {
		fields["timezone"] = timezone
	}

	if len(fields) == 0 {
		return nil
	}

	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("set profile prefs: %w", err)
	}

	// Set TTL on the entire hash (future HEXPIRE per-field could be used)
	if err := rdb.Expire(ctx, key, ProfilePrefsCacheTTL).Err(); err != nil {
		return fmt.Errorf("set profile prefs TTL: %w", err)
	}

	return nil
}

// DeleteProfilePrefs invalida el cache de preferencias para un usuario.
// Se llama después de actualizar timezone, idioma o moneda.
func DeleteProfilePrefs(ctx context.Context, rdb *redis.Client, userID string) error {
	key := profilePrefsKeyPrefix + userID + profilePrefsKeySuffix
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete profile prefs: %w", err)
	}
	return nil
}

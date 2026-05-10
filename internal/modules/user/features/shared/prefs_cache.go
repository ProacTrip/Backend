package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// ProfilePrefsCacheTTL es el TTL para las preferencias cacheadas del perfil
	ProfilePrefsCacheTTL = 30 * time.Minute // 30 minutos

	// profilePrefsKeyPrefix es el prefijo de la clave de cache
	profilePrefsKeyPrefix = "profile:"
	profilePrefsKeySuffix = ":prefs"
)

// GetProfilePrefs obtiene las preferencias cacheadas del usuario desde Dragonfly.
// Retorna (currency, language, countryCode, timezone, found, error).
// En cache miss, found=false y todos los strings vacíos — el caller
// debe hacer fallback al siguiente nivel (geoip o defaults de entorno).
func GetProfilePrefs(ctx context.Context, rdb *redis.Client, userID string) (currency, language, countryCode, timezone string, found bool, err error) {
	key := profilePrefsKeyPrefix + userID + profilePrefsKeySuffix

	fields, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return "", "", "", "", false, fmt.Errorf("get profile prefs: %w", err)
	}

	// Sin campos → cache miss
	if len(fields) == 0 {
		return "", "", "", "", false, nil
	}

	// Encontrado — extraer campos individuales (pueden ser parciales)
	currency = fields["currency"]
	language = fields["language"]
	countryCode = fields["country_code"]
	timezone = fields["timezone"]

	return currency, language, countryCode, timezone, true, nil
}

// SetProfilePrefs guarda las preferencias del usuario en Dragonfly como un Hash.
// Clave de cache: profile:{userID}:prefs
// Campos: currency, language, country_code, timezone
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

	// Setear TTL en el hash completo (a futuro podría usarse HEXPIRE por campo)
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

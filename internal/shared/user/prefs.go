// Paquete user expone funciones compartidas para lectura/escritura de preferencias
// de perfil de usuario en DragonflyDB. El contrato de clave es user:prefs:{userID} —
// documentado en S-SPEC-001. Ambos módulos (user y search) usan este paquete para
// garantizar compatibilidad de formato en tiempo de compilación.
//
// Formato de clave: user:prefs:{userID} — hash con campos currency, language.
package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Prefs — preferencias de perfil cacheadas en Dragonfly
// =============================================================================

// Prefs contiene las preferencias de perfil del usuario cacheadas en DragonflyDB.
// Los campos se almacenan como campos individuales de un hash user:prefs:{userID}.
// CountryCode y Timezone fueron removidos en Phase 2 de ai-discovery-rewrite —
// esos datos se resuelven exclusivamente desde la caché env:{ip} o DEFAULT_COUNTRY_CODE.
type Prefs struct {
	Currency          string   `json:"currency"`
	Language          string   `json:"language"`
	PreferredAirlines []string `json:"preferred_airlines"`
	PreferredHotels   []string `json:"preferred_hotels"`
}

// =============================================================================
// GetProfilePrefs — lectura del hash user:prefs:{userID}
// =============================================================================

// GetProfilePrefs obtiene las preferencias cacheadas del usuario desde Dragonfly.
// Retorna nil, nil en cache miss (hash inexistente o vacío).
// Retorna error solo si Dragonfly falla.
// Clave: user:prefs:{userID} (hash con campos currency, language).
func GetProfilePrefs(ctx context.Context, rdb *redis.Client, userID string) (*Prefs, error) {
	key := "user:prefs:" + userID

	fields, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get profile prefs: %w", err)
	}

	// Sin campos → cache miss
	if len(fields) == 0 {
		return nil, nil
	}

	return &Prefs{
		Currency:          fields["currency"],
		Language:          fields["language"],
		PreferredAirlines: deserializeStringSlice(fields["preferred_airlines"]),
		PreferredHotels:   deserializeStringSlice(fields["preferred_hotels"]),
	}, nil
}

// =============================================================================
// SetProfilePrefs — escritura del hash user:prefs:{userID}
// =============================================================================

// SetProfilePrefs guarda las preferencias del usuario en Dragonfly como un hash.
// Si prefs es nil o todos los campos están vacíos, no hace nada (no-op).
// Clave: user:prefs:{userID} — campos: currency, language.
func SetProfilePrefs(ctx context.Context, rdb *redis.Client, userID string, prefs *Prefs) error {
	if prefs == nil {
		return nil
	}

	fields := make(map[string]interface{}, 4)
	if prefs.Currency != "" {
		fields["currency"] = prefs.Currency
	}
	if prefs.Language != "" {
		fields["language"] = prefs.Language
	}
	if len(prefs.PreferredAirlines) > 0 {
		fields["preferred_airlines"] = serializeStringSlice(prefs.PreferredAirlines)
	}
	if len(prefs.PreferredHotels) > 0 {
		fields["preferred_hotels"] = serializeStringSlice(prefs.PreferredHotels)
	}

	if len(fields) == 0 {
		return nil
	}

	key := "user:prefs:" + userID
	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("set profile prefs: %w", err)
	}

	// TTL de 24h — alineado con S-SPEC-001. La clave se refresca en cada escritura.
	if err := rdb.Expire(ctx, key, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("set profile prefs TTL: %w", err)
	}

	return nil
}

// =============================================================================
// DeleteProfilePrefs — invalidación del hash user:prefs:{userID}
// =============================================================================

// DeleteProfilePrefs elimina las preferencias cacheadas del usuario desde DragonflyDB.
// Se llama después de actualizar idioma o moneda para forzar re-cache en
// la próxima lectura.
// Clave: user:prefs:{userID}.
func DeleteProfilePrefs(ctx context.Context, rdb *redis.Client, userID string) error {
	key := "user:prefs:" + userID
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete profile prefs: %w", err)
	}
	return nil
}

// =============================================================================
// Helpers for string slice serialization in Dragonfly hash fields
// =============================================================================

// serializeStringSlice marshals a []string to JSON for hash field storage.
func serializeStringSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(s)
	return string(data)
}

// deserializeStringSlice unmarshals a JSON string to []string from a hash field.
// Returns nil for empty or invalid JSON.
func deserializeStringSlice(raw string) []string {
	if raw == "" {
		return nil
	}
	var s []string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil
	}
	return s
}

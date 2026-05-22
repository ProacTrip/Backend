// Paquete user expone funciones compartidas para lectura/escritura de preferencias
// de perfil de usuario en DragonflyDB. El contrato de clave es user:prefs:{userID} —
// documentado en S-SPEC-001. Ambos módulos (user y search) usan este paquete para
// garantizar compatibilidad de formato en tiempo de compilación.
//
// Formato de clave: user:prefs:{userID} — hash con campos currency, language.
package user

import (
	"context"
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
	Currency string `json:"currency"`
	Language string `json:"language"`
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
		Currency: fields["currency"],
		Language: fields["language"],
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

	fields := make(map[string]interface{}, 2)
	if prefs.Currency != "" {
		fields["currency"] = prefs.Currency
	}
	if prefs.Language != "" {
		fields["language"] = prefs.Language
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

// Paquete session expone funciones compartidas para lectura/escritura del cache
// de sesión en DragonflyDB. El contrato de clave es {auth}:session:{userID} —
// documentado en PM-SPEC-004. El middleware de autenticación usa este paquete para
// evitar consultas a la DB en cada request.
//
// Formato de clave: {auth}:session:{userID} — hash con campos permissions,
// status, token_version, schema_version.
package session

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// SessionData — datos de sesión cacheados en Dragonfly
// =============================================================================

// SessionData contiene los campos cacheados para la sesión de un usuario.
// Se almacena como un hash {auth}:session:{userID} en DragonflyDB.
// Single-session: una sola entrada por usuario.
type SessionData struct {
	// UserID es el ID del usuario propietario de esta sesión.
	UserID string

	// Permissions es una lista de códigos de permiso separados por coma (sin espacios).
	// Ej: "users:read,users:write,roles:read"
	Permissions string

	// Status es el estado de la cuenta del usuario.
	Status string

	// TokenVersion es la versión del token del usuario (para invalidación).
	TokenVersion string

	// SchemaVersion es la versión del schema del cache (para migraciones de formato).
	SchemaVersion string
}

// =============================================================================
// Constantes del paquete
// =============================================================================

const (
	// SessionTTL es el TTL por defecto para las entradas de cache de sesión.
	// Reducido de 5min a 1min para que cambios de estado (disable/enable)
	// se propaguen más rápido. El sliding reset en cada request mantiene
	// las sesiones activas sin afectar el rendimiento.
	SessionTTL = 1 * time.Minute

	// SchemaVersionActual es la versión actual del schema de cache.
	// Incrementar cuando cambie la estructura del hash para forzar repoblación.
	SchemaVersionActual = "1"
)

// keyForSession genera la clave Dragonfly para el hash de sesión.
// Formato: {auth}:session:{userID} — el hashtag {auth} asegura que
// todas las claves de sesión caigan en el mismo shard.
// Single-session: una sola clave por usuario.
func keyForSession(userID string) string {
	return fmt.Sprintf("{auth}:session:%s", userID)
}

// =============================================================================
// GetSession — lectura del hash {auth}:session:{userID}
// =============================================================================

// GetSession obtiene los datos de sesión cacheados desde Dragonfly.
// Retorna nil, nil en cache miss (hash inexistente o vacío).
// Retorna error solo si Dragonfly falla.
func GetSession(ctx context.Context, rdb *redis.Client, userID string) (*SessionData, error) {
	key := keyForSession(userID)

	fields, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get session cache: %w", err)
	}

	// Sin campos → cache miss
	if len(fields) == 0 {
		return nil, nil
	}

	return &SessionData{
		UserID:        fields["user_id"],
		Permissions:   fields["permissions"],
		Status:        fields["status"],
		TokenVersion:  fields["token_version"],
		SchemaVersion: fields["schema_version"],
	}, nil
}

// =============================================================================
// SetSession — escritura del hash {auth}:session:{userID}
// =============================================================================

// SetSession guarda los datos de sesión en Dragonfly como un hash con TTL.
// El TTL se resetea en cada escritura (sliding expiration).
// Si data es nil, no hace nada (no-op).
func SetSession(ctx context.Context, rdb *redis.Client, userID string, data *SessionData, ttl time.Duration) error {
	if data == nil {
		return nil
	}

	key := keyForSession(userID)
	schemaVer := cmp.Or(data.SchemaVersion, SchemaVersionActual)

	fields := map[string]interface{}{
		"user_id":        data.UserID,
		"permissions":    data.Permissions,
		"status":         data.Status,
		"token_version":  data.TokenVersion,
		"schema_version": schemaVer,
	}

	pipe := rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set session cache: %w", err)
	}

	return nil
}

// =============================================================================
// InvalidateSession — eliminación de la sesión de un usuario
// =============================================================================

// InvalidateSession elimina la entrada de cache de sesión para un usuario.
// Idempotente: no retorna error si la key no existe.
// Single-session: solo hay una key por usuario ({auth}:session:{userID}).
func InvalidateSession(ctx context.Context, rdb *redis.Client, userID string) error {
	key := keyForSession(userID)
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("invalidate session: %w", err)
	}
	return nil
}

// =============================================================================
// GetOrSetSession — cache-aside con fallback a DB
// =============================================================================

// GetOrSetSession implementa el patrón cache-aside: si la sesión existe en
// Dragonfly la retorna. Si no, llama a fn() para obtener los datos desde la DB,
// los guarda en el cache y los retorna.
//
// fn() solo se llama en cache miss. Dos requests concurrentes que caen en miss
// ambos llamarán a fn() y ambos harán HSet. Como los datos son los mismos,
// el último write gana (idempotente, sin corrupción).
func GetOrSetSession(ctx context.Context, rdb *redis.Client, userID string, ttl time.Duration, fn func() (*SessionData, error)) (*SessionData, error) {
	// Cache hit
	data, err := GetSession(ctx, rdb, userID)
	if err != nil {
		return nil, err
	}
	if data != nil {
		return data, nil
	}

	// Cache miss — llamar a la fuente de datos (DB)
	data, err = fn()
	if err != nil {
		return nil, fmt.Errorf("get or set session fn: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	// Guardar en cache (best-effort, no bloquea en error)
	if setErr := SetSession(ctx, rdb, userID, data, ttl); setErr != nil {
		// Log pero no fallar — el dato se retorna igual desde DB
		_ = setErr
	}

	return data, nil
}

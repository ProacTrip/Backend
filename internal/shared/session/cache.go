// Paquete session expone funciones compartidas para lectura/escritura del cache
// de sesión en DragonflyDB. El contrato de clave es {auth}:session:{sessionID} —
// documentado en PM-SPEC-004. El middleware de autenticación usa este paquete para
// evitar consultas a la DB en cada request.
//
// Formato de clave: {auth}:session:{sessionID} — hash con campos permissions,
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
// Se almacena como un hash {auth}:session:{sessionID} en DragonflyDB.
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
	SessionTTL = 5 * time.Minute

	// SchemaVersionActual es la versión actual del schema de cache.
	// Incrementar cuando cambie la estructura del hash para forzar repoblación.
	SchemaVersionActual = "1"
)

// keyForSession genera la clave Dragonfly para el hash de sesión.
// Formato: {auth}:session:{sessionID} — el hashtag {auth} asegura que
// todas las claves de sesión caigan en el mismo shard.
func keyForSession(sessionID string) string {
	return fmt.Sprintf("{auth}:session:%s", sessionID)
}

// =============================================================================
// GetSession — lectura del hash {auth}:session:{sessionID}
// =============================================================================

// GetSession obtiene los datos de sesión cacheados desde Dragonfly.
// Retorna nil, nil en cache miss (hash inexistente o vacío).
// Retorna error solo si Dragonfly falla.
func GetSession(ctx context.Context, rdb *redis.Client, sessionID string) (*SessionData, error) {
	key := keyForSession(sessionID)

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
// SetSession — escritura del hash {auth}:session:{sessionID}
// =============================================================================

// SetSession guarda los datos de sesión en Dragonfly como un hash con TTL.
// El TTL se resetea en cada escritura (sliding expiration).
// Si data es nil, no hace nada (no-op).
func SetSession(ctx context.Context, rdb *redis.Client, sessionID string, data *SessionData, ttl time.Duration) error {
	if data == nil {
		return nil
	}

	key := keyForSession(sessionID)
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
// InvalidateSession — eliminación de una sesión específica
// =============================================================================

// InvalidateSession elimina una sesión específica del cache.
// Idempotente: no retorna error si la key no existe.
func InvalidateSession(ctx context.Context, rdb *redis.Client, sessionID string) error {
	key := keyForSession(sessionID)
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("invalidate session: %w", err)
	}
	return nil
}

// =============================================================================
// InvalidateAllUserSessions — eliminación de todas las sesiones de un usuario
// =============================================================================

// InvalidateAllUserSessions elimina todas las entradas de cache de sesión
// para un usuario específico. Escanea todas las claves {auth}:session:* y
// solo elimina aquellas cuyo campo user_id coincide con el userID dado.
// Usa el hashtag {auth} para mantener las operaciones en el mismo shard.
// Limitado a 100 keys por iteración de SCAN para evitar bloquear Dragonfly.
//
// Si falla el HGET para alguna key, esa key se omite (no se elimina) —
// el token_version mismatch la invalidará eventualmente.
func InvalidateAllUserSessions(ctx context.Context, rdb *redis.Client, userID string) error {
	pattern := "{auth}:session:*"
	var cursor uint64

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("invalidate all sessions scan: %w", err)
		}

		for _, key := range keys {
			// Verificar que esta sesión pertenece al usuario antes de eliminar
			ownerID, err := rdb.HGet(ctx, key, "user_id").Result()
			if err != nil {
				// Si no podemos leer el hash, omitimos esta key (best-effort).
				// El token_version mismatch la invalidará eventualmente.
				continue
			}
			if ownerID == userID {
				rdb.Del(ctx, key)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
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
func GetOrSetSession(ctx context.Context, rdb *redis.Client, sessionID string, ttl time.Duration, fn func() (*SessionData, error)) (*SessionData, error) {
	// Cache hit
	data, err := GetSession(ctx, rdb, sessionID)
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
	if setErr := SetSession(ctx, rdb, sessionID, data, ttl); setErr != nil {
		// Log pero no fallar — el dato se retorna igual desde DB
		_ = setErr
	}

	return data, nil
}

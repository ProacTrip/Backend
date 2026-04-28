package crypto

// =============================================================================
// Key Derivation - Genera claves seguras para cache e idempotency
// =============================================================================

import (
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/hash"
)

// CacheKey genera una clave segura para cache usando BLAKE3 KDF
// Contexto evita colisiones entre diferentes tipos de cache
func CacheKey(cacheType string, identifier string) string {
	// Usar BLAKE3 KDF con contexto único por tipo de cache
	return hash.DeriveKeyHex([]byte(identifier), "cache:"+cacheType)
}

// IdempotencyKey genera una clave de idempotencia derivando del请求
func IdempotencyKey(endpoint string, payloadHash string) string {
	// Combinar endpoint + hash del payload para clave única
	return hash.DeriveKeyHex([]byte(payloadHash), "idempotency:"+endpoint)
}

// SessionKey genera una clave para sesiones
func SessionKey(userID string, sessionID string) string {
	return hash.DeriveKeyHex([]byte(sessionID), "session:"+userID)
}

// PermissionKey genera una clave para permisos de usuario
func PermissionKey(userID string) string {
	return hash.DeriveKeyHex([]byte(userID), "permissions")
}

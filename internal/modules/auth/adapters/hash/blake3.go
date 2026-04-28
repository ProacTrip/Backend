package hash

import (
	"encoding/hex"

	"lukechampine.com/blake3"
)

// Funciones de hashing rápido con BLAKE3.
// Casos de uso: cache keys, deduplicación, idempotency, firmas.

// Hash genera un hash BLAKE3 de 32 bytes (256 bits)
// Útil para: cache keys, deduplicación, verificación de integridad
func Hash(data []byte) string {
	h := blake3.Sum256(data)
	return hex.EncodeToString(h[:])
}

// HashString versión para strings
func HashString(data string) string {
	return Hash([]byte(data))
}

// HashBytes32 retorna los 32 bytes directamente (sin hex)
func HashBytes(data []byte) [32]byte {
	return blake3.Sum256(data)
}

// KeyedHash genera un hash con clave secreta.
// Útil para: firmas de tokens API, integrity de mensajes firmados
// Parameters: outputLen (max 64), key (32 bytes recommended)
func KeyedHash(data []byte, outputLen int, key []byte) string {
	h := blake3.New(outputLen, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// KeyedHashString versión para strings
func KeyedHashString(data string, key []byte) string {
	return KeyedHash([]byte(data), 32, key)
}

// DeriveKey deriva una subclave de una clave maestra usando BLAKE3 KDF
// Útil para: deriving de claves de sesión, sub-claves por usuario
// API: DeriveKey(subKey []byte, ctx string, srcKey []byte)
// subKey debe tener el tamaño deseado (outputLen)
func DeriveKey(masterKey []byte, context string, outputLen int) []byte {
	subKey := make([]byte, outputLen)
	blake3.DeriveKey(subKey, context, masterKey)
	return subKey
}

// DeriveKeyHex versión que retorna hex
func DeriveKeyHex(masterKey []byte, context string) string {
	return hex.EncodeToString(DeriveKey(masterKey, context, 32))
}

// FileHash genera un hash de archivo para deduplicación
func FileHash(data []byte) string {
	return Hash(data)
}

// FileHashMultiChunk para archivos grandes (streaming)
func FileHashMultiChunk(chunks [][]byte) string {
	h := blake3.New(32, nil) // 32 bytes output, no key
	for _, chunk := range chunks {
		h.Write(chunk)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateIdempotencyKey genera una key idempotente desde datos del request
// Combina: email + timestamp + body_hash para máxima uniqueness
func GenerateIdempotencyKey(email, timestamp, bodyHash string) string {
	h := blake3.New(32, nil)
	h.Write([]byte(email))
	h.Write([]byte(timestamp))
	h.Write([]byte(bodyHash))
	return hex.EncodeToString(h.Sum(nil))
}

// HashBody genera hash del body para Idempotency-Key
func HashBody(body []byte) string {
	return Hash(body)
}

// CacheKey genera una clave de cache basada en múltiples parámetros
func CacheKey(prefix string, params ...string) string {
	h := blake3.New(32, nil)
	h.Write([]byte(prefix))
	for _, p := range params {
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// InvalidateByTag genera key para invalidar cache por tag
func InvalidateByTag(tag string) string {
	return "invalidate:" + HashString(tag)
}

// =============================================================================
// Casos de uso en Proactrip:
// =============================================================================
//
// 1. Deduplicación de imágenes de hoteles:
//    - Cuando suben imagen, calcular hash
//    - Si hash existe, no guardar duplicado
//
// 2. Cache invalidation:
//    - Clave de cache: "hotel:123:reviews:" + hash(params)
//
// 3. Token de verificación (alternativo a PASETO):
//    - BLAKE3 hash como token (más rápido)
//
// 4. Idempotency-Key:
//    - Hash(email + timestamp + body) como fallback
//
// =============================================================================

// Ejemplo de uso:
//
// // Hash simple
// h := hash.HashString("data-123")
//
// // Hash con clave para tokens
// key := hash.DeriveKey(masterKey, "verification-token", 32)
// token := hash.KeyedHashString(payload, key)
//
// // Cache key
// cacheKey := hash.CacheKey("hotel", "123", "reviews", "es")
//
// // Idempotency
// bodyHash := hash.HashBody(requestBody)
// idemKey := hash.GenerateIdempotencyKey(email, timestamp, bodyHash)

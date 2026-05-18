package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Hasher de contraseñas usando Argon2id (recomendado por OWASP).

func generateSalt(length uint32) ([]byte, error) {
	salt := make([]byte, length)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, fmt.Errorf("generar salt: %w", err)
	}
	return salt, nil
}

func encodeHash(salt, hash []byte) string {
	saltB64 := base64.StdEncoding.EncodeToString(salt)
	hashB64 := base64.StdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		64*1024, // memory
		3,       // iterations
		4,       // parallel
		saltB64,
		hashB64,
	)
}

// HashParams contiene los parámetros del hash almacenado
type HashParams struct {
	Memory     uint32
	Iterations uint32
	Parallel   uint8
}

func decodeHash(encoded string) (salt, hash []byte, params HashParams, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return nil, nil, HashParams{}, fmt.Errorf("formato de hash inválido")
	}

	// Parsear parámetros: $argon2id$v=VERSION$m=MEMORY,t=ITERATIONS,p=PARALLEL$SALT$HASH
	paramPart := parts[3] // "m=65536,t=3,p=4"
	if _, err := fmt.Sscanf(paramPart, "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallel); err != nil {
		return nil, nil, HashParams{}, fmt.Errorf("error parseando parámetros: %w", err)
	}

	// Strip padding before decoding — accepts both padded (StdEncoding) and
	// unpadded (RawStdEncoding) base64. Pre-computed seed hashes may use either format.
	saltStr := strings.TrimRight(parts[4], "=")
	hashStr := strings.TrimRight(parts[5], "=")

	salt, err = base64.RawStdEncoding.DecodeString(saltStr)
	if err != nil {
		return nil, nil, HashParams{}, fmt.Errorf("error decodificando salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(hashStr)
	if err != nil {
		return nil, nil, HashParams{}, fmt.Errorf("error decodificando hash: %w", err)
	}

	return salt, hash, params, nil
}

func constantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// =============================================================================
// Password Hasher - Usa Argon2id (OWASP recommended)
// =============================================================================

// Hasher implementa hashing de contraseñas con Argon2id
type Hasher struct {
	memory     uint32
	iterations uint32
	parallel   uint8
	saltLen    uint32
	keyLen     uint32
}

// NewHasher crea un nuevo hasher con parámetros por defecto
func NewHasher() *Hasher {
	return &Hasher{
		memory:     64 * 1024, // 64 MB
		iterations: 3,
		parallel:   4,
		saltLen:    16,
		keyLen:     32,
	}
}

// Hash crea el hash de una contraseña
func (h *Hasher) Hash(password string) (string, error) {
	salt, err := generateSalt(h.saltLen)
	if err != nil {
		return "", fmt.Errorf("error generando salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		h.iterations,
		h.memory,
		h.parallel,
		h.keyLen,
	)

	return encodeHash(salt, hash), nil
}

// Verify compara una contraseña con un hash almacenado
// Usa los PARÁMETROS DEL HASH ALMACENADO para verificar (compatibilidad hacia atrás)
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	salt, hash, params, err := decodeHash(encoded)
	if err != nil {
		return false, fmt.Errorf("error decodificando hash: %w", err)
	}

	// Usar los parámetros DEL HASH ALMACENADO, no los actuales
	// Esto permite que hashes viejos sigan funcionando
	newHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallel,
		h.keyLen,
	)

	// Comparación constante para evitar timing attacks
	return constantTimeCompare(hash, newHash), nil
}

// NeedsUpgrade verifica si el hash necesita ser actualizado
// Compara los parámetros del hash almacenado con los parámetros actuales
func (h *Hasher) NeedsUpgrade(encoded string) bool {
	_, _, params, err := decodeHash(encoded)
	if err != nil {
		// Si no se puede parsear, asumir que necesita upgrade
		return true
	}

	// Si los parámetros son distintos, necesita upgrade
	return params.Memory != h.memory ||
		params.Iterations != h.iterations ||
		params.Parallel != h.parallel
}

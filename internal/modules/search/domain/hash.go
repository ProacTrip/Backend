// Utilidades de hashing para cache keys.
// Genera claves deterministas usando blake3.
package domain

import (
	"encoding/hex"

	"lukechampine.com/blake3"
)

// HashKey returns a hex-encoded blake3 hash of data, truncated to 32 hex chars.
func HashKey(data []byte) string {
	hash := blake3.Sum256(data)
	return hex.EncodeToString(hash[:16])
}

// Adaptador de hashing usando blake3.
// Implementa el puerto domain.HashService para deduplicación de búsquedas.
package hash

import (
	"fmt"

	"lukechampine.com/blake3"
)

// Blake3Service implementa domain.HashService usando blake3.
type Blake3Service struct{}

// NewBlake3Service crea un nuevo servicio de hashing blake3.
func NewBlake3Service() *Blake3Service {
	return &Blake3Service{}
}

// Hash calcula el hash blake3 de los datos y retorna el hex string.
// Usado para deduplicación de búsquedas guardadas (no criptográfico).
func (s *Blake3Service) Hash(data []byte) string {
	h := blake3.Sum256(data)
	return fmt.Sprintf("%x", h)
}

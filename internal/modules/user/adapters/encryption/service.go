// Adapter: Servicio de encriptación con ChaCha20-Poly1305.
// Usa nonce aleatorio por operación, prepended al ciphertext.
package encryption

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

// =============================================================================
// Service — Encriptación simétrica con ChaCha20-Poly1305
// =============================================================================

// Service implementa el cifrado y descifrado de datos sensibles del usuario.
// Usa XChaCha20-Poly1305 (AEAD) con nonce aleatorio de 24 bytes.
type Service struct {
	aead cipher.AEAD
}

// NewService crea un nuevo servicio de encriptación.
// La clave debe ser exactamente de 32 bytes (ChaCha20-Poly1305).
func NewService(key []byte) (*Service, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, errors.New("clave de encriptación inválida: debe ser de 32 bytes")
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	return &Service{aead: aead}, nil
}

// Encrypt encripta el texto plano.
// Genera un nonce aleatorio de 24 bytes y lo prepende al ciphertext.
// Retorna: nonce(24 bytes) + ciphertext + tag(16 bytes)
func (s *Service) Encrypt(plaintext string) ([]byte, error) {
	// Generar nonce aleatorio de 24 bytes (XChaCha20-Poly1305)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	// Encriptar con nonce aleatorio
	// Seal: nonce, plaintext, additionalData (nil)
	ciphertext := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)

	return ciphertext, nil
}

// Decrypt desencripta ciphertext.
// Espera nonce(24 bytes) + ciphertext + tag(16 bytes)
func (s *Service) Decrypt(ciphertext []byte) (string, error) {
	nonceSize := s.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext demasiado corto: falta el nonce")
	}

	// Extraer nonce del inicio
	nonce := ciphertext[:nonceSize]
	encrypted := ciphertext[nonceSize:]

	// Desencriptar
	plaintext, err := s.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

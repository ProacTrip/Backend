package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Cifrado AES-GCM para datos sensibles.
// Reemplazo de ChaCha20-Poly1305, sin dependencias externas.

type SimpleAESCipher struct {
	block cipher.Block
}

// NewSimpleAES crea una nueva instancia de cifrado AES-GCM
// La clave debe ser exactamente 32 bytes (AES-256)
func NewSimpleAES(key []byte) (*SimpleAESCipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &SimpleAESCipher{block: block}, nil
}

// GenerateKey genera una clave aleatoria de 32 bytes para AES-256
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt cifra datos sensibles usando AES-GCM
// Retorna: nonce (12 bytes) + ciphertext + tag (16 bytes), todo codificado en base64
func (c *SimpleAESCipher) Encrypt(plaintext []byte) (string, error) {
	// Generar nonce aleatorio (12 bytes para AES-GCM)
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Crear selador GCM
	gcm, err := cipher.NewGCM(c.block)
	if err != nil {
		return "", err
	}

	// Cifrar
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt descifra datos cifrados con Encrypt
// Input: ciphertext codificado en base64
func (c *SimpleAESCipher) Decrypt(encoded string) ([]byte, error) {
	// Decodificar base64
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	// Validar tamaño mínimo (nonce + tag mínimo)
	if len(ciphertext) < 12+16 {
		return nil, errors.New("ciphertext demasiado corto")
	}

	// Extraer nonce del inicio
	nonce := ciphertext[:12]
	data := ciphertext[12:]

	// Crear descifrador GCM
	gcm, err := cipher.NewGCM(c.block)
	if err != nil {
		return nil, err
	}

	// Descifrar
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptString versión convenience para strings
func (c *SimpleAESCipher) EncryptString(plaintext string) (string, error) {
	return c.Encrypt([]byte(plaintext))
}

// DecryptString versión convenience para strings
func (c *SimpleAESCipher) DecryptString(encoded string) (string, error) {
	plaintext, err := c.Decrypt(encoded)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

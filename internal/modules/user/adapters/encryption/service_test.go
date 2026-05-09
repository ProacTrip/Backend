package encryption

import (
	"crypto/rand"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Compile-time interface check
var _ domain.EncryptionService = (*Service)(nil)

func generateTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("no se pudo generar clave: %v", err)
	}
	return key
}

func TestNewService_ValidKey(t *testing.T) {
	key := generateTestKey(t)
	svc, err := NewService(key)
	if err != nil {
		t.Fatalf("NewService con clave válida falló: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService devolvió nil")
	}
}

func TestNewService_InvalidKeySize(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{"16 bytes", 16},
		{"24 bytes", 24},
		{"31 bytes", 31},
		{"33 bytes", 33},
		{"64 bytes", 64},
		{"0 bytes", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			_, err := rand.Read(key)
			if err != nil {
				t.Skipf("rand read error: %v", err)
			}

			svc, err := NewService(key)
			if err == nil {
				t.Errorf("se esperaba error para clave de %d bytes, pero no hubo", tt.keySize)
			}
			if svc != nil {
				t.Error("se esperaba svc nil cuando hay error")
			}
		})
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		name      string
		plaintext string
	}{
		{"texto simple", "hola mundo"},
		{"texto largo", "este es un texto más largo con caracteres especiales: áéíóú ñ €"},
		{"vacío", ""},
		{"JSON", `{"blood_type":"A+","allergies":["penicilina","sulfa"]}`},
		{"número documento", "X12345678"},
		{"email", "usuario@ejemplo.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := svc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt falló: %v", err)
			}
			if len(ciphertext) == 0 {
				t.Fatal("ciphertext vacío")
			}

			decrypted, err := svc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt falló: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("round-trip falló: se esperaba %q, se obtuvo %q", tt.plaintext, decrypted)
			}
		})
	}
}

func TestEncrypt_ProducesDifferentNonce(t *testing.T) {
	svc := newTestService(t)
	plaintext := "mismo texto"

	cipher1, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt #1 falló: %v", err)
	}

	cipher2, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt #2 falló: %v", err)
	}

	// Con nonce aleatorio, dos cifrados del mismo texto deben ser diferentes
	if string(cipher1) == string(cipher2) {
		t.Error("dos cifrados del mismo texto produjeron ciphertexts idénticos — nonce no es aleatorio")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	svc := newTestService(t)

	ciphertext, err := svc.Encrypt("texto original")
	if err != nil {
		t.Fatalf("Encrypt falló: %v", err)
	}

	// Alterar el ciphertext (después del nonce)
	if len(ciphertext) > 25 {
		ciphertext[25] ^= 0xFF
	}

	_, err = svc.Decrypt(ciphertext)
	if err == nil {
		t.Error("se esperaba error al desencriptar ciphertext alterado, pero no hubo")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	svc1 := newTestService(t)
	svc2 := newTestService(t)

	ciphertext, err := svc1.Encrypt("secreto")
	if err != nil {
		t.Fatalf("Encrypt falló: %v", err)
	}

	_, err = svc2.Decrypt(ciphertext)
	if err == nil {
		t.Error("se esperaba error al desencriptar con clave diferente, pero no hubo")
	}
}

func TestDecrypt_ShortCiphertext(t *testing.T) {
	svc := newTestService(t)

	// Ciphertext más corto que el nonce (24 bytes)
	_, err := svc.Decrypt([]byte("corto"))
	if err == nil {
		t.Error("se esperaba error con ciphertext muy corto, pero no hubo")
	}
}

// newTestService creates a service with a random key for testing
func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(generateTestKey(t))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

package encryption_test

import (
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/encryption"
)

// testAESKey es una clave de 32 bytes para AES-256.
var testAESKey = []byte("abcdefghijklmnopqrstuvwxyz123456")

// newCipher crea un SimpleAESCipher para testing.
func newCipher(t *testing.T) *encryption.SimpleAESCipher {
	t.Helper()

	c, err := encryption.NewSimpleAES(testAESKey)
	if err != nil {
		t.Fatalf("NewSimpleAES: %v", err)
	}
	return c
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 1: Encrypt + Decrypt roundtrip → texto original
// ──────────────────────────────────────────────────────────────────────────────

func TestEncryptDecrypt_Roundtrip_DevuelveTextoOriginal(t *testing.T) {
	c := newCipher(t)

	tests := []struct {
		nombre string
		texto  string
	}{
		{"texto simple", "hola mundo"},
		{"texto con caracteres especiales", "¡áéíóú! €ñÑ"},
		{"texto largo", strings.Repeat("datos sensibles de prueba ", 50)},
		{"unicode", "🚀 Proactrip — viajes ✈️"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			encrypted, err := c.EncryptString(tt.texto)
			if err != nil {
				t.Fatalf("EncryptString: %v", err)
			}

			decrypted, err := c.DecryptString(encrypted)
			if err != nil {
				t.Fatalf("DecryptString: %v", err)
			}

			if decrypted != tt.texto {
				t.Errorf("DecryptString = %q, esperado %q", decrypted, tt.texto)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 2: Encrypt string vacía → decrypt string vacía
// ──────────────────────────────────────────────────────────────────────────────

func TestEncryptDecrypt_Vacio_RoundtripCorrecto(t *testing.T) {
	c := newCipher(t)

	encrypted, err := c.EncryptString("")
	if err != nil {
		t.Fatalf("EncryptString(\"\"): %v", err)
	}

	decrypted, err := c.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}

	if decrypted != "" {
		t.Errorf("DecryptString = %q, esperado %q", decrypted, "")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 3: Decrypt con clave incorrecta → error
// ──────────────────────────────────────────────────────────────────────────────

func TestDecrypt_ClaveIncorrecta_DevuelveError(t *testing.T) {
	c := newCipher(t)

	encrypted, err := c.EncryptString("dato-secreto")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	// Crear un cipher con clave DIFERENTE.
	wrongKey := []byte("zyxwvutsrqponmlkjihgfedcba123456")
	c2, err := encryption.NewSimpleAES(wrongKey)
	if err != nil {
		t.Fatalf("NewSimpleAES (wrong key): %v", err)
	}

	_, err = c2.DecryptString(encrypted)
	if err == nil {
		t.Fatal("se esperaba error al descifrar con clave incorrecta, se obtuvo nil")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 4: Encrypt produce diferente output cada vez (nonce aleatorio)
// ──────────────────────────────────────────────────────────────────────────────

func TestEncrypt_MismoTexto_DiferenteOutputCadaVez(t *testing.T) {
	c := newCipher(t)

	enc1, err := c.EncryptString("mismo-texto")
	if err != nil {
		t.Fatalf("EncryptString (1): %v", err)
	}

	enc2, err := c.EncryptString("mismo-texto")
	if err != nil {
		t.Fatalf("EncryptString (2): %v", err)
	}

	if enc1 == enc2 {
		t.Error("dos cifrados del mismo texto no deberían ser idénticos (nonce aleatorio)")
	}

	// Ambos deben descifrar al mismo texto.
	dec1, err := c.DecryptString(enc1)
	if err != nil {
		t.Fatalf("DecryptString (1): %v", err)
	}
	dec2, err := c.DecryptString(enc2)
	if err != nil {
		t.Fatalf("DecryptString (2): %v", err)
	}
	if dec1 != "mismo-texto" || dec2 != "mismo-texto" {
		t.Errorf("dec1=%q, dec2=%q, ambos deberían ser 'mismo-texto'", dec1, dec2)
	}
}

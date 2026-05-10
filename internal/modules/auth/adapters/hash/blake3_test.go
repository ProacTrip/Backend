package hash_test

import (
	"encoding/hex"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/hash"
)

// testBlake3Key es una clave de ejemplo para KeyedHash (32 bytes).
var testBlake3Key = []byte("clave-secreta-de-32-bytes!!!!!!!")

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 1: Hash determinista — mismo input = mismo output
// ──────────────────────────────────────────────────────────────────────────────

func TestHash_Determinista_MismoInputMismoOutput(t *testing.T) {
	data := []byte("datos-para-hashear")

	h1 := hash.Hash(data)
	h2 := hash.Hash(data)

	if h1 != h2 {
		t.Errorf("Hash determinista falló: h1=%q, h2=%q", h1, h2)
	}
	if h1 == "" {
		t.Error("Hash devolvió string vacío")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 2: Diferentes inputs → diferentes hashes
// ──────────────────────────────────────────────────────────────────────────────

func TestHash_DiferentesInputs_DiferentesHashes(t *testing.T) {
	h1 := hash.HashString("primer-input")
	h2 := hash.HashString("segundo-input-distinto")

	if h1 == h2 {
		t.Errorf("hashes no deberían ser iguales para diferentes inputs: %q", h1)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 3: KeyedHash con clave → diferente del hash sin clave
// ──────────────────────────────────────────────────────────────────────────────

func TestKeyedHash_ConClave_DiferenteDeSinClave(t *testing.T) {
	data := []byte("datos-firmados")

	simple := hash.Hash(data)
	keyed := hash.KeyedHash(data, 32, testBlake3Key)

	if simple == keyed {
		t.Error("KeyedHash debería ser diferente del Hash sin clave para el mismo input")
	}
	if keyed == "" {
		t.Error("KeyedHash devolvió string vacío")
	}

	// KeyedHash debe ser determinista con la misma clave.
	keyed2 := hash.KeyedHash(data, 32, testBlake3Key)
	if keyed != keyed2 {
		t.Errorf("KeyedHash debería ser determinista: %q != %q", keyed, keyed2)
	}

	// Con clave diferente, output diferente.
	differentKey := []byte("otra-clave-diferente-de-32-bytes")
	keyed3 := hash.KeyedHash(data, 32, differentKey)
	if keyed == keyed3 {
		t.Error("KeyedHash con claves diferentes no debería dar el mismo resultado")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 4: DeriveKey retorna el largo correcto
// ──────────────────────────────────────────────────────────────────────────────

func TestDeriveKey_RetornaLargoCorrecto(t *testing.T) {
	masterKey := []byte("clave-maestra-de-32-bytes-para-kdf!")

	tests := []struct {
		nombre    string
		contexto  string
		outputLen int
	}{
		{"32 bytes", "subkey-32", 32},
		{"16 bytes", "subkey-16", 16},
		{"64 bytes", "subkey-64", 64},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			subKey := hash.DeriveKey(masterKey, tt.contexto, tt.outputLen)

			if len(subKey) != tt.outputLen {
				t.Errorf("DeriveKey largo = %d, esperado %d", len(subKey), tt.outputLen)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Extra: DeriveKey es determinista con los mismos parámetros
// ──────────────────────────────────────────────────────────────────────────────

func TestDeriveKey_Determinista_MismosParametrosMismoOutput(t *testing.T) {
	masterKey := []byte("clave-maestra-determinista-de-32-by")

	k1 := hash.DeriveKey(masterKey, "contexto-fijo", 32)
	k2 := hash.DeriveKey(masterKey, "contexto-fijo", 32)

	if hex.EncodeToString(k1) != hex.EncodeToString(k2) {
		t.Error("DeriveKey debería ser determinista con los mismos parámetros")
	}

	// Contexto diferente → output diferente
	k3 := hash.DeriveKey(masterKey, "contexto-diferente", 32)
	if hex.EncodeToString(k1) == hex.EncodeToString(k3) {
		t.Error("DeriveKey con contextos diferentes no debería dar el mismo resultado")
	}
}

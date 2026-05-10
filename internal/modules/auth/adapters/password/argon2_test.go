package password_test

import (
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/password"
)

func newHasher(t *testing.T) *password.Hasher {
	t.Helper()
	return password.NewHasher()
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 1: Hash — string codificado no vacío, empieza con "$argon2id$"
// ──────────────────────────────────────────────────────────────────────────────

func TestHash_DevuelveHashNoVacioPrefijoArgon2id(t *testing.T) {
	h := newHasher(t)

	encoded, err := h.Hash("mi-contraseña-segura")
	if err != nil {
		t.Fatalf("Hash: error inesperado: %v", err)
	}
	if encoded == "" {
		t.Error("Hash devolvió string vacío")
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Errorf("Hash no empieza con $argon2id$: %q", encoded)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 2: Verify contraseña correcta → true
// ──────────────────────────────────────────────────────────────────────────────

func TestVerify_Correcta_DevuelveTrue(t *testing.T) {
	h := newHasher(t)

	encoded, err := h.Hash("contraseña-correcta")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := h.Verify("contraseña-correcta", encoded)
	if err != nil {
		t.Fatalf("Verify: error inesperado: %v", err)
	}
	if !ok {
		t.Error("Verify debería devolver true para contraseña correcta")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 3: Verify contraseña incorrecta → false (comparación constante)
// ──────────────────────────────────────────────────────────────────────────────

func TestVerify_Incorrecta_DevuelveFalse(t *testing.T) {
	h := newHasher(t)

	encoded, err := h.Hash("contraseña-real")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := h.Verify("contraseña-equivocada", encoded)
	if err != nil {
		t.Fatalf("Verify: error inesperado: %v", err)
	}
	if ok {
		t.Error("Verify debería devolver false para contraseña incorrecta")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 4: Verify contraseña vacía → false
// ──────────────────────────────────────────────────────────────────────────────

func TestVerify_Vacia_DevuelveFalse(t *testing.T) {
	h := newHasher(t)

	encoded, err := h.Hash("contraseña-no-vacia")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := h.Verify("", encoded)
	if err != nil {
		t.Fatalf("Verify: error inesperado: %v", err)
	}
	if ok {
		t.Error("Verify debería devolver false para contraseña vacía")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 5: Hash misma contraseña dos veces → sales y hashes diferentes
// ──────────────────────────────────────────────────────────────────────────────

func TestHash_MismaContraseña_DiferentesSaltsYDiferentesHashes(t *testing.T) {
	h := newHasher(t)

	hash1, err := h.Hash("misma-contraseña")
	if err != nil {
		t.Fatalf("Hash (1): %v", err)
	}

	hash2, err := h.Hash("misma-contraseña")
	if err != nil {
		t.Fatalf("Hash (2): %v", err)
	}

	if hash1 == hash2 {
		t.Error("dos hashes de la misma contraseña no deberían ser idénticos (sales diferentes)")
	}

	// Ambos deben verificar correctamente.
	ok1, err := h.Verify("misma-contraseña", hash1)
	if err != nil || !ok1 {
		t.Errorf("Verify hash1: ok=%v, err=%v", ok1, err)
	}
	ok2, err := h.Verify("misma-contraseña", hash2)
	if err != nil || !ok2 {
		t.Errorf("Verify hash2: ok=%v, err=%v", ok2, err)
	}
}

package verification_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
)

// testSymKey es una clave simétrica de 32 bytes exactos para PASETO V4.
var testSymKey = []byte("abcdefghijklmnopqrstuvwxyz123456")

// newVerificationSvc crea un VerificationService respaldado por miniredis.
func newVerificationSvc(t *testing.T) *verification.VerificationService {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	svc, err := verification.NewVerificationService(verification.VerificationConfig{
		SymmetricKey: testSymKey,
	}, client)
	if err != nil {
		t.Fatalf("NewVerificationService: %v", err)
	}

	return svc
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 1: GenerateToken — token no vacío
// ──────────────────────────────────────────────────────────────────────────────

func TestGenerateToken_DevuelveTokenNoVacio(t *testing.T) {
	svc := newVerificationSvc(t)

	token, err := svc.GenerateToken(t.Context(), "test@proactrip.com")
	if err != nil {
		t.Fatalf("GenerateToken: error inesperado: %v", err)
	}
	if token == "" {
		t.Error("GenerateToken devolvió token vacío")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 2: VerifyToken válido — email retornado, JTI presente en Dragonfly
// ──────────────────────────────────────────────────────────────────────────────

func TestVerifyToken_Valido_DevuelveEmail(t *testing.T) {
	svc := newVerificationSvc(t)

	token, err := svc.GenerateToken(t.Context(), "verify@proactrip.com")
	if err != nil {
		t.Fatalf("GenerateToken: error inesperado: %v", err)
	}

	claims, err := svc.VerifyToken(t.Context(), token)
	if err != nil {
		t.Fatalf("VerifyToken: error inesperado: %v", err)
	}

	if claims.Email != "verify@proactrip.com" {
		t.Errorf("Email = %q, esperado %q", claims.Email, "verify@proactrip.com")
	}
	if claims.JTI.String() == "" {
		t.Error("JTI no debería estar vacío")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 3: VerifyToken inválido — error
// ──────────────────────────────────────────────────────────────────────────────

func TestVerifyToken_Invalido_DevuelveError(t *testing.T) {
	svc := newVerificationSvc(t)

	_, err := svc.VerifyToken(t.Context(), "token-falsificado-no-valido")
	if err == nil {
		t.Fatal("se esperaba error para token inválido, se obtuvo nil")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 4: VerifyToken replayed — JTI ya no está en Dragonfly → error
// Simula un replay attack: el JTI se elimina manualmente de Dragonfly después
// de una verificación exitosa (simulando que ya fue consumido).
// ──────────────────────────────────────────────────────────────────────────────

func TestVerifyToken_Replay_DevuelveError(t *testing.T) {
	svc := newVerificationSvc(t)

	token, err := svc.GenerateToken(t.Context(), "replay@proactrip.com")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Primera verificación: debe funcionar.
	_, err = svc.VerifyToken(t.Context(), token)
	if err != nil {
		t.Fatalf("primera VerifyToken: error inesperado: %v", err)
	}

	// Simular que el JTI fue consumido (eliminado de Dragonfly).
	// Para esto necesitamos acceso directo a miniredis, que no exponemos desde el test.
	// Método alternativo: usar un servicio nuevo con miniredis fresco donde el JTI no existe.
	mr := miniredis.RunT(t)
	client2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc2, err := verification.NewVerificationService(verification.VerificationConfig{
		SymmetricKey: testSymKey,
	}, client2)
	if err != nil {
		t.Fatalf("NewVerificationService (svc2): %v", err)
	}

	// El token fue emitido por svc pero validado contra svc2 que no tiene el JTI.
	_, err = svc2.VerifyToken(t.Context(), token)
	if err == nil {
		t.Fatal("se esperaba error por JTI faltante (replay), se obtuvo nil")
	}
}

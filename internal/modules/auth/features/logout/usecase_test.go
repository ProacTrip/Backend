package logout_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/logout"
)

// =============================================================================
// Mock TokenService para tests del usecase de logout
// =============================================================================

type mockTokenService struct {
	claims *token.AccessClaims
	err    error
}

func (m *mockTokenService) ValidateAccessToken(_ context.Context, _ string) (*token.AccessClaims, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.claims, nil
}

// =============================================================================
// Helpers
// =============================================================================

func newLogoutUseCase(t *testing.T, tokenSvc *mockTokenService, mr *miniredis.Miniredis) *logout.UseCase {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return logout.NewUseCase(logout.UseCaseDeps{
		TokenSvc:    tokenSvc,
		DragonflyDB: client,
	})
}

func jtiBlacklistKey(jti uuid.UUID) string {
	return "{auth}:blacklist:jti:" + jti.String()
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_BlacklistJTI_JTIRegistradoEnDragonfly verifica que después de Execute,
// el JTI queda registrado en Dragonfly con SET y TTL de 15 minutos.
func TestExecute_BlacklistJTI_JTIRegistradoEnDragonfly(t *testing.T) {
	mr := miniredis.RunT(t)

	jti := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	tokenSvc := &mockTokenService{
		claims: &token.AccessClaims{
			UserID: userID,
			Email:  "user@example.com",
			RoleID: uuid.Must(uuid.NewV7()),
			Role:   "client",
			JTI:    jti,
		},
	}

	uc := newLogoutUseCase(t, tokenSvc, mr)

	resp, err := uc.Execute(t.Context(), logout.Command{
		Token: "valid-token",
	})
	if err != nil {
		t.Fatalf("Execute() error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != "Sesión cerrada exitosamente." {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, "Sesión cerrada exitosamente.")
	}

	// Verificar que el JTI está presente en Dragonfly
	key := jtiBlacklistKey(jti)
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("JTI no encontrado en Dragonfly: %v", err)
	}
	if val != "1" {
		t.Errorf("valor en blacklist = %q, se esperaba %q", val, "1")
	}

	// Verificar que el TTL está configurado (~15 minutos)
	ttl := mr.TTL(key)
	if ttl <= 0 {
		t.Errorf("TTL = %v, se esperaba TTL > 0 (15 minutos)", ttl)
	}
	expectedTTL := 15 * time.Minute
	if ttl < expectedTTL-2*time.Second || ttl > expectedTTL {
		t.Errorf("TTL = %v, se esperaba ~%v", ttl, expectedTTL)
	}
}

// TestExecute_DragonflyNoDisponible_FailOpenSinError verifica que con token inválido y
// Dragonfly caído, el usecase retorna nil (fail-open) porque el early-return en validación
// de token no depende de Dragonfly.
func TestExecute_DragonflyNoDisponible_FailOpenSinError(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis.Start: %v", err)
	}

	// Token inválido — el usecase retorna éxito temprano sin tocar Dragonfly
	tokenSvc := &mockTokenService{
		err: domain.ErrTokenExpired,
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	uc := logout.NewUseCase(logout.UseCaseDeps{
		TokenSvc:    tokenSvc,
		DragonflyDB: client,
	})

	// Cerrar Dragonfly antes de ejecutar (simula caída)
	mr.Close()
	client.Close()

	resp, err := uc.Execute(t.Context(), logout.Command{
		Token: "token-expirado",
	})
	if err != nil {
		t.Errorf("Execute() debería ser fail-open sin error (token inválido + Dragonfly caído), obtuvo: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != "Sesión cerrada exitosamente." {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, "Sesión cerrada exitosamente.")
	}
}

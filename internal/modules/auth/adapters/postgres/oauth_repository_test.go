// Test: OAuthRepository — constructor, interface satisfaction, and structural verification.
// Cannot easily mock pgx; these tests verify compile-time contracts and construction.
package postgres_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Compile-time check: OAuthRepository implements domain.OAuthRepository
// =============================================================================

func TestOAuthRepository_InterfaceSatisfaction(t *testing.T) {
	// Si esto no compila, OAuthRepository no satisface domain.OAuthRepository.
	var _ domain.OAuthRepository = (*postgres.OAuthRepository)(nil) //nolint:unused
}

// =============================================================================
// Constructor: NewOAuthRepository crea un repositorio no-nil
// =============================================================================

func TestNewOAuthRepository_NoNil(t *testing.T) {
	repo := postgres.NewOAuthRepository(nil)
	if repo == nil {
		t.Fatal("NewOAuthRepository retornó nil")
	}
}

// =============================================================================
// Structural: verifica que el struct tiene el campo pool del tipo esperado
// =============================================================================

func TestOAuthRepository_TieneCampoPool(t *testing.T) {
	repo := postgres.NewOAuthRepository(nil)
	if repo.Pool() != nil {
		t.Error("esperaba pool nil cuando se construye con nil")
	}
	t.Log("OAuthRepository creado correctamente — las queries SQL requieren base de datos real para pruebas de integración")
}

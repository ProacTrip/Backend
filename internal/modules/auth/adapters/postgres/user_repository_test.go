// Test: UserRepository — constructor, interface satisfaction, and structural verification.
// Cannot easily mock pgx; these tests verify compile-time contracts and construction.
package postgres_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Compile-time check: UserRepository implements domain.UserRepository
// =============================================================================

func TestUserRepository_InterfaceSatisfaction(t *testing.T) {
	// Si esto no compila, UserRepository no satisface domain.UserRepository.
	var _ domain.UserRepository = (*postgres.UserRepository)(nil) //nolint:unused
}

// =============================================================================
// Constructor: NewUserRepository crea un repositorio no-nil
// =============================================================================

func TestNewUserRepository_NoNil(t *testing.T) {
	repo := postgres.NewUserRepository(nil)
	if repo == nil {
		t.Fatal("NewUserRepository retornó nil")
	}
}

// =============================================================================
// Structural: verifica que el struct tiene el campo pool del tipo esperado
// =============================================================================

func TestUserRepository_TieneCampoPool(t *testing.T) {
	repo := postgres.NewUserRepository(nil)
	if repo.Pool() != nil {
		t.Error("esperaba pool nil cuando se construye con nil")
	}
	t.Log("UserRepository creado correctamente — las queries SQL requieren base de datos real para pruebas de integración")
}

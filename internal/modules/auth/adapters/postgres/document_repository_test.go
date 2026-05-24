// Tests estructurales del DocumentRepository adapter.
// Verifica contratos de compilación y comportamiento del constructor.
// Tests de integración completos requieren DB de test (saltados en -short).
package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Compile-time checks
// =============================================================================

func TestDocumentRepository_ImplementsAllInterfaces(t *testing.T) {
	// Verifica que DocumentRepository implementa las 3 interfaces del dashboard.
	var _ interface {
		GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
		GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
		GetHistory(ctx context.Context, documentID uuid.UUID) ([]domain.HistoryEntry, error)
		InsertHistory(ctx context.Context, entry domain.HistoryEntry) error
		UpdateVerificationStatus(ctx context.Context, id uuid.UUID, status string) error
		UpdateOCRStatus(ctx context.Context, id uuid.UUID, status string) error
		GetUserDocuments(ctx context.Context, userID uuid.UUID) ([]domain.DocumentSummary, error)
	} = (*postgres.DocumentRepository)(nil) //nolint:unused
}

// =============================================================================
// Constructor
// =============================================================================

func TestNewDocumentRepository_NoNil(t *testing.T) {
	repo := postgres.NewDocumentRepository(nil)
	if repo == nil {
		t.Fatal("NewDocumentRepository retornó nil")
	}
}

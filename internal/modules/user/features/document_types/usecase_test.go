// Tests del usecase document_types.
// Table-driven: success, repo-error.
package document_types

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockTypeRepo struct {
	getTypesFn func(ctx context.Context) ([]domain.DocumentType, error)
}

func (m *mockTypeRepo) GetTypes(ctx context.Context) ([]domain.DocumentType, error) {
	if m.getTypesFn != nil {
		return m.getTypesFn(ctx)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestDocumentTypes_Success(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		TypeRepo: &mockTypeRepo{
			getTypesFn: func(_ context.Context) ([]domain.DocumentType, error) {
				return []domain.DocumentType{
					{ID: uuid.Must(uuid.NewV7()), Code: "passport", Name: "Pasaporte", IsActive: true},
					{ID: uuid.Must(uuid.NewV7()), Code: "visa", Name: "Visa", IsActive: true},
				}, nil
			},
		},
	})

	types, err := uc.Execute(t.Context())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("expected 2 document types, got %d", len(types))
	}
	if types[0].Code != "passport" {
		t.Errorf("types[0].Code = %q, want %q", types[0].Code, "passport")
	}
}

func TestDocumentTypes_RepoError(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		TypeRepo: &mockTypeRepo{
			getTypesFn: func(_ context.Context) ([]domain.DocumentType, error) {
				return nil, errors.New("DB error")
			},
		},
	})

	_, err := uc.Execute(t.Context())
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

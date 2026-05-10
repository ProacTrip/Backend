// Tests para el usecase list_documents.
// Valida listado con/sin filtros y lista vacía.
package list_documents

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type testDocRepo struct {
	getByUserIDFn         func(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error)
	getByUserIDFilteredFn func(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error)
}

func (m *testDocRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *testDocRepo) GetByUserIDFiltered(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error) {
	if m.getByUserIDFilteredFn != nil {
		return m.getByUserIDFilteredFn(ctx, userID, status, docType)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestListDocumentsUseCase_WithoutFilters(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mime := "image/png"
	size := 512

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				return []*domain.UserDocument{
					{
						ID:        uuid.Must(uuid.NewV7()),
						UserID:    userID,
						FileName:  "foto.png",
						MimeType:  &mime,
						FileSize:  &size,
						OCRStatus: domain.OCRStatusCompleted,
					},
					{
						ID:        uuid.Must(uuid.NewV7()),
						UserID:    userID,
						FileName:  "doc.pdf",
						MimeType:  new(string),
						FileSize:  &size,
						OCRStatus: domain.OCRStatusQueued,
					},
				}, nil
			},
		},
	})

	docs, err := uc.Execute(t.Context(), userID.String(), "", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("len(docs) = %d, se esperaba 2", len(docs))
	}
}

func TestListDocumentsUseCase_WithFilters(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	filteredCalled := false
	unfilteredCalled := false

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				unfilteredCalled = true
				return []*domain.UserDocument{}, nil
			},
			getByUserIDFilteredFn: func(ctx context.Context, uid uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error) {
				filteredCalled = true
				return []*domain.UserDocument{}, nil
			},
		},
	})

	// Con filtro status
	_, err := uc.Execute(t.Context(), userID.String(), "completed", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !filteredCalled {
		t.Error("GetByUserIDFiltered debería ser llamado cuando hay filtro")
	}
	if unfilteredCalled {
		t.Error("GetByUserID no debería ser llamado cuando hay filtro")
	}
}

func TestListDocumentsUseCase_EmptyList(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				return nil, nil
			},
		},
	})

	docs, err := uc.Execute(t.Context(), userID.String(), "", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if docs == nil {
		t.Error("docs no debería ser nil, debería ser slice vacío")
	}
	if len(docs) != 0 {
		t.Errorf("len(docs) = %d, se esperaba 0", len(docs))
	}
}

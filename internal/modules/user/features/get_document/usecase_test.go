// Tests para el usecase get_document.
// Valida documento encontrado con extracted_data, not found, y wrong user.
package get_document

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

type testDocRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

func (m *testDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestGetDocumentUseCase_SuccessfulGet(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	mime := "application/pdf"
	size := 1024

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return &domain.UserDocument{
					ID:        docID,
					UserID:    userID,
					FileName:  "pasaporte.pdf",
					MimeType:  &mime,
					FileSize:  &size,
					OCRStatus: domain.OCRStatusCompleted,
				}, nil
			},
		},
	})

	doc, err := uc.Execute(t.Context(), docID.String(), userID.String())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if doc.FileName != "pasaporte.pdf" {
		t.Errorf("FileName = %s, se esperaba pasaporte.pdf", doc.FileName)
	}
	if doc.UserID != userID {
		t.Error("UserID no coincide")
	}
}

func TestGetDocumentUseCase_NotFound(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return nil, domain.ErrDocumentNotFound
			},
		},
	})

	_, err := uc.Execute(t.Context(), docID.String(), userID.String())
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

func TestGetDocumentUseCase_WrongUser(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return &domain.UserDocument{ID: docID, UserID: otherUserID}, nil
			},
		},
	})

	_, err := uc.Execute(t.Context(), docID.String(), userID.String())
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

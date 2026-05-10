// Tests para el usecase verify_document.
// Valida MarkVerified true/false y escenarios de error.
package verify_document

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
	updateFn  func(ctx context.Context, doc *domain.UserDocument) error
}

func (m *testDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *testDocRepo) Update(ctx context.Context, doc *domain.UserDocument) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, doc)
	}
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestVerifyDocumentUseCase_MarkVerifiedTrue(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())
	doc := &domain.UserDocument{
		ID:       docID,
		UserID:   uuid.Must(uuid.NewV7()),
		FileName: "pasaporte.pdf",
	}

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) { return doc, nil },
			updateFn:  func(ctx context.Context, d *domain.UserDocument) error { return nil },
		},
		Dragonfly: nil,
	})

	cmd := VerifyCommand{
		DocumentID: docID.String(),
		IsVerified: true,
		VerifiedBy: adminID.String(),
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !resp.IsVerified {
		t.Error("IsVerified debería ser true")
	}
	if !doc.IsVerified {
		t.Error("documento no fue marcado como verificado")
	}
	if doc.VerifiedBy == nil || *doc.VerifiedBy != adminID {
		t.Error("VerifiedBy no coincide con el adminID")
	}
}

func TestVerifyDocumentUseCase_MarkVerifiedFalse(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())
	verifiedBy := uuid.Must(uuid.NewV7())
	doc := &domain.UserDocument{
		ID:         docID,
		UserID:     uuid.Must(uuid.NewV7()),
		FileName:   "doc.pdf",
		IsVerified: true,
		VerifiedBy: &verifiedBy,
	}

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) { return doc, nil },
			updateFn:  func(ctx context.Context, d *domain.UserDocument) error { return nil },
		},
		Dragonfly: nil,
	})

	cmd := VerifyCommand{
		DocumentID: docID.String(),
		IsVerified: false,
		VerifiedBy: adminID.String(),
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.IsVerified {
		t.Error("IsVerified debería ser false")
	}
	if doc.IsVerified {
		t.Error("documento debería estar desmarcado como verificado")
	}
	if doc.VerifiedBy != nil {
		t.Error("VerifiedBy debería ser nil después de desmarcar")
	}
}

func TestVerifyDocumentUseCase_NotFound(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return nil, domain.ErrDocumentNotFound
			},
		},
		Dragonfly: nil,
	})

	cmd := VerifyCommand{
		DocumentID: docID.String(),
		IsVerified: true,
		VerifiedBy: adminID.String(),
	}

	_, err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

func TestVerifyDocumentUseCase_InvalidDocumentID(t *testing.T) {
	adminID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo:   &testDocRepo{},
		Dragonfly: nil,
	})

	cmd := VerifyCommand{
		DocumentID: "no-es-un-uuid",
		IsVerified: true,
		VerifiedBy: adminID.String(),
	}

	_, err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

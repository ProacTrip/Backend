// Tests para el usecase delete_document.
// Valida eliminación exitosa, not found, y wrong user.
package delete_document

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
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *testDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *testDocRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

type testR2Client struct {
	deleteFn      func(ctx context.Context, bucket, key string) error
	listObjectsFn func(ctx context.Context, bucket, prefix string) ([]string, error)
}

func (m *testR2Client) Delete(ctx context.Context, bucket, key string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, bucket, key)
	}
	return nil
}
func (m *testR2Client) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	if m.listObjectsFn != nil {
		return m.listObjectsFn(ctx, bucket, prefix)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestDeleteDocumentUseCase_SuccessfulDelete(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	deleteCalled := false

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return &domain.UserDocument{ID: docID, UserID: userID}, nil
			},
			deleteFn: func(ctx context.Context, id uuid.UUID) error {
				deleteCalled = true
				return nil
			},
		},
		R2: &testR2Client{
			listObjectsFn: func(ctx context.Context, bucket, prefix string) ([]string, error) {
				return nil, nil
			},
		},
	})

	err := uc.Execute(t.Context(), docID.String(), userID.String())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !deleteCalled {
		t.Error("Delete no fue llamado")
	}
}

func TestDeleteDocumentUseCase_NotFound(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return nil, domain.ErrDocumentNotFound
			},
		},
		R2: &testR2Client{},
	})

	err := uc.Execute(t.Context(), docID.String(), userID.String())
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

func TestDeleteDocumentUseCase_WrongUser(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return &domain.UserDocument{ID: docID, UserID: otherUserID}, nil
			},
		},
		R2: &testR2Client{},
	})

	err := uc.Execute(t.Context(), docID.String(), userID.String())
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

func TestDeleteDocumentUseCase_InvalidDocumentID(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{},
		R2:      &testR2Client{},
	})

	err := uc.Execute(t.Context(), "no-es-un-uuid", userID.String())
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("error = %v, se esperaba ErrDocumentNotFound", err)
	}
}

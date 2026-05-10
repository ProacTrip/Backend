// Tests para el handler DELETE /v1/user/documents/:document_id.
package delete_document

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// =============================================================================
// Mocks
// =============================================================================

type mockDocRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockDocRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

type mockR2Client struct {
	deleteFn      func(ctx context.Context, bucket, key string) error
	listObjectsFn func(ctx context.Context, bucket, prefix string) ([]string, error)
}

func (m *mockR2Client) Delete(ctx context.Context, bucket, key string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, bucket, key)
	}
	return nil
}
func (m *mockR2Client) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	if m.listObjectsFn != nil {
		return m.listObjectsFn(ctx, bucket, prefix)
	}
	return nil, nil
}

// =============================================================================
// Helpers
// =============================================================================

func testClaims(userID uuid.UUID) *sharedauth.AccessClaims {
	return &sharedauth.AccessClaims{
		UserID: userID, Email: "test@example.com",
		RoleID: uuid.Must(uuid.NewV7()), Role: "client",
		SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7()),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestDeleteDocumentHandler_Handle(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	t.Run("debe retornar 200 cuando elimina documento exitosamente", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodDelete, "/v1/user/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", testClaims(userID))

		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return &domain.UserDocument{ID: docID, UserID: userID}, nil
				},
				deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil },
			},
			R2: &mockR2Client{
				listObjectsFn: func(ctx context.Context, bucket, prefix string) ([]string, error) { return nil, nil },
			},
		})

		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, se esperaba 200", rec.Code)
		}
	})

	t.Run("debe retornar error cuando no hay claims", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodDelete, "/v1/user/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{},
			R2:      &mockR2Client{},
		})
		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code == http.StatusOK {
			t.Errorf("status = %d, se esperaba error cuando no hay claims", rec.Code)
		}
	})

	t.Run("debe retornar 404 cuando documento no existe", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodDelete, "/v1/user/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", testClaims(userID))

		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return nil, domain.ErrDocumentNotFound
				},
			},
			R2: &mockR2Client{},
		})
		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, se esperaba 404", rec.Code)
		}
	})

	t.Run("debe retornar 404 cuando documento no pertenece al usuario", func(t *testing.T) {
		e := echo.New()
		otherUserID := uuid.Must(uuid.NewV7())
		req := httptest.NewRequest(http.MethodDelete, "/v1/user/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", testClaims(userID))

		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return &domain.UserDocument{ID: docID, UserID: otherUserID}, nil
				},
			},
			R2: &mockR2Client{},
		})
		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, se esperaba 404 (ownership fail)", rec.Code)
		}
	})
}

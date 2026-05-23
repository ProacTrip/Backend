// Tests para el handler GET /v1/user/documents.
package list_documents

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
	getByUserIDFn         func(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error)
	getByUserIDFilteredFn func(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error)
}

func (m *mockDocRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockDocRepo) GetByUserIDFiltered(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error) {
	if m.getByUserIDFilteredFn != nil {
		return m.getByUserIDFilteredFn(ctx, userID, status, docType)
	}
	return nil, nil
}

// =============================================================================
// Helpers
// =============================================================================

func testClaims() *sharedauth.AccessClaims {
	return &sharedauth.AccessClaims{
		UserID: uuid.Must(uuid.NewV7()), Email: "test@example.com",
		RoleID: uuid.Must(uuid.NewV7()), Role: "client",
		JTI: uuid.Must(uuid.NewV7()),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestListDocumentsHandler_Handle(t *testing.T) {
	tests := []struct {
		name       string
		claims     *sharedauth.AccessClaims
		mockRepo   *mockDocRepo
		wantStatus int
	}{
		{
			name:   "debe retornar 200 con lista de documentos",
			claims: testClaims(),
			mockRepo: &mockDocRepo{
				getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error) {
					return []*domain.UserDocument{}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "debe retornar 401 cuando no hay claims (no autenticado)",
			mockRepo:   &mockDocRepo{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "debe retornar error cuando repo falla",
			claims: testClaims(),
			mockRepo: &mockDocRepo{
				getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error) {
					return nil, domain.ErrDocumentNotFound
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/user/documents", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tc.claims != nil {
				c.Set("user_claims", tc.claims)
			}
			uc := NewUseCase(UseCaseDeps{DocRepo: tc.mockRepo})
			h := NewHandler(uc)
			_ = h.Handle(c)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, se esperaba %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

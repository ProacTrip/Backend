// Tests para el handler PUT /v1/user/documents/:document_id/verify.
package verify_document

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// =============================================================================
// Mocks
// =============================================================================

type mockVerifyDocRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	updateFn  func(ctx context.Context, doc *domain.UserDocument) error
}

func (m *mockVerifyDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockVerifyDocRepo) Update(ctx context.Context, doc *domain.UserDocument) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, doc)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func testClaims() *sharedauth.AccessClaims {
	return &sharedauth.AccessClaims{
		UserID:    uuid.Must(uuid.NewV7()),
		Email:     "admin@example.com",
		RoleID:    uuid.Must(uuid.NewV7()),
		Role:      "admin",
		SessionID: uuid.Must(uuid.NewV7()),
		JTI:       uuid.Must(uuid.NewV7()),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestVerifyDocumentHandler_Handle(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	doc := &domain.UserDocument{
		ID:       docID,
		UserID:   uuid.Must(uuid.NewV7()),
		FileName: "doc.pdf",
	}

	tests := []struct {
		name       string
		claims     *sharedauth.AccessClaims
		body       string
		mockRepo   *mockVerifyDocRepo
		wantStatus int
	}{
		{
			name:   "debe retornar 200 con verificacion exitosa",
			claims: testClaims(),
			body:   `{"is_verified":true}`,
			mockRepo: &mockVerifyDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) { return doc, nil },
				updateFn:  func(ctx context.Context, d *domain.UserDocument) error { return nil },
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "debe retornar error cuando no hay claims",
			body:       `{"is_verified":true}`,
			mockRepo:   &mockVerifyDocRepo{},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:   "debe retornar 404 cuando documento no existe",
			claims: testClaims(),
			body:   `{"is_verified":false}`,
			mockRepo: &mockVerifyDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return nil, domain.ErrDocumentNotFound
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/v1/user/documents/"+docID.String()+"/verify", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/v1/user/documents/:document_id/verify")
			c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
			if tc.claims != nil {
				c.Set("user_claims", tc.claims)
			}
			uc := NewUseCase(UseCaseDeps{
				DocRepo:   tc.mockRepo,
				Dragonfly: nil,
			})
			h := NewHandler(uc)
			_ = h.Handle(c)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, se esperaba %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

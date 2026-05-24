// Tests para el handler GET /v1/user/profile/documents.
package list_documents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
			req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents", nil)
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

// =============================================================================
// DTO Shape Tests — verifica que el handler mapea domain → DTO
// =============================================================================

func TestListDocumentsHandler_ResponseUsesDTO(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	confidence := 0.97
	docType := "passport"
	now := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

	claims := &sharedauth.AccessClaims{
		UserID: userID, Email: "test@example.com",
		RoleID: uuid.Must(uuid.NewV7()), Role: "client",
		JTI: uuid.Must(uuid.NewV7()),
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_claims", claims)

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &mockDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				size := 1024
				return []*domain.UserDocument{{
					ID:                 docID,
					UserID:             userID,
					DocumentTypeID:     uuid.Nil,
					FileName:           "pasaporte.pdf",
					FileSize:           &size,
					StorageKey:         "documents/raw/secret-key",
					DocumentType:       &docType,
					OCRStatus:          domain.OCRStatusCompleted,
					OCRConfidence:      &confidence,
					VerificationStatus: domain.VerificationStatusVerified,
					ExtractedData:      json.RawMessage(`{"first_name":"Aurelio"}`),
					CreatedAt:          now,
					UpdatedAt:          now,
				}}, nil
			},
		},
	})
	h := NewHandler(uc)
	_ = h.Handle(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200", rec.Code)
	}

	body := rec.Body.String()

	// Verifica campos del DTO están presentes
	requiredFields := []string{"id", "file_name", "document_type", "ocr_status",
		"ocr_confidence", "verification_status", "created_at"}
	for _, f := range requiredFields {
		if !strings.Contains(body, `"`+f+`"`) {
			t.Errorf("campo requerido %q no está en la respuesta: %s", f, body)
		}
	}

	// NO deben exponerse campos internos
	forbiddenFields := []string{"storage_key", "extracted_data", "document_type_id",
		"user_id", "ocr_data", "file_size", "mime_type", "updated_at"}
	for _, f := range forbiddenFields {
		if strings.Contains(body, `"`+f+`"`) {
			t.Errorf("campo prohibido %q expuesto en respuesta: %s", f, body)
		}
	}

	// Verifica valores concretos del DTO
	if !strings.Contains(body, `"file_name":"pasaporte.pdf"`) {
		t.Error("file_name no coincide")
	}
	if !strings.Contains(body, `"ocr_status":"completed"`) {
		t.Error("ocr_status no coincide")
	}
	if !strings.Contains(body, `"verification_status":"verified"`) {
		t.Error("verification_status no coincide")
	}
	if !strings.Contains(body, `"document_type":"passport"`) {
		t.Error("document_type no coincide")
	}
	if !strings.Contains(body, docID.String()) {
		t.Error("id del documento no presente")
	}
}

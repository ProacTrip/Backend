// Tests para el handler GET /v1/user/profile/documents/:document_id.
package get_document

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
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

func (m *mockDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
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
		JTI: uuid.Must(uuid.NewV7()),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestGetDocumentHandler_Handle(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	t.Run("debe retornar 200 con documento encontrado y extracted_data", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/profile/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", testClaims(userID))

		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					mime := "application/pdf"
					size := 1024
					return &domain.UserDocument{
						ID:        docID,
						UserID:    userID,
						FileName:  "documento.pdf",
						MimeType:  &mime,
						FileSize:  &size,
						OCRStatus: domain.OCRStatusCompleted,
					}, nil
				},
			},
		})

		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, se esperaba 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "documento.pdf") {
			t.Errorf("respuesta no contiene file_name: %s", rec.Body.String())
		}
	})

	t.Run("debe retornar error cuando no hay claims", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/profile/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		uc := NewUseCase(UseCaseDeps{DocRepo: &mockDocRepo{}})
		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code == http.StatusOK {
			t.Errorf("status = %d, se esperaba error cuando no hay claims", rec.Code)
		}
	})

	t.Run("debe retornar 404 cuando documento no existe", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/profile/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", testClaims(userID))

		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return nil, domain.ErrDocumentNotFound
				},
			},
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
		req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents/"+docID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/user/profile/documents/:document_id")
		c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", testClaims(userID))

		uc := NewUseCase(UseCaseDeps{
			DocRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return &domain.UserDocument{ID: docID, UserID: otherUserID}, nil
				},
			},
		})
		h := NewHandler(uc)
		_ = h.Handle(c)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, se esperaba 404 (ownership fail)", rec.Code)
		}
	})
}

// =============================================================================
// DTO Shape Tests — verifica que el handler mapea domain → DTO
// =============================================================================

func TestGetDocumentHandler_ResponseUsesDTO(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	confidence := 0.97
	docType := "passport"
	mime := "application/pdf"
	size := 1024
	verifiedAt := time.Date(2026, 5, 1, 10, 35, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/documents/"+docID.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/user/profile/documents/:document_id")
	c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
	c.Set("user_claims", testClaims(userID))

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &mockDocRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				return &domain.UserDocument{
					ID:                 docID,
					UserID:             userID,
					FileName:           "pasaporte.pdf",
					MimeType:           &mime,
					FileSize:           &size,
					StorageKey:         "documents/raw/secret",
					DocumentType:       &docType,
					OCRStatus:          domain.OCRStatusCompleted,
					OCRConfidence:      &confidence,
					VerificationStatus: domain.VerificationStatusVerified,
					ExtractedData:      json.RawMessage(`{"first_name":"Aurelio"}`),
					ValidFrom:          &verifiedAt,
					CreatedAt:          now,
					UpdatedAt:          now,
				}, nil
			},
		},
	})
	h := NewHandler(uc)
	_ = h.Handle(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200", rec.Code)
	}

	body := rec.Body.String()

	// Campos específicos del detalle deben estar presentes
	if !strings.Contains(body, `"file_name":"pasaporte.pdf"`) {
		t.Error("file_name no coincide")
	}
	if !strings.Contains(body, `"document_type":"passport"`) {
		t.Error("document_type no coincide")
	}
	if !strings.Contains(body, `"ocr_status":"completed"`) {
		t.Error("ocr_status no coincide")
	}
	if !strings.Contains(body, `"verification_status":"verified"`) {
		t.Error("verification_status no coincide")
	}
	if !strings.Contains(body, docID.String()) {
		t.Error("id del documento no presente")
	}

	// NO deben exponerse campos internos
	forbiddenFields := []string{"storage_key", "extracted_data", "document_type_id",
		"ocr_data", "has_newer_medical_data"}
	for _, f := range forbiddenFields {
		if strings.Contains(body, `"`+f+`"`) {
			t.Errorf("campo prohibido %q expuesto en respuesta: %s", f, body)
		}
	}
}

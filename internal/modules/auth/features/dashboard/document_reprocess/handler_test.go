// Tests del handler HTTP de reprocesamiento OCR.
// DR-REQ-1: POST 202 Accepted.
// Los stubs (stubDocReprocessRepo) y fixtures están en usecase_test.go (mismo package).
package document_reprocess_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	docreprocess "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/document_reprocess"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Domain error mappers — registrados en init() para que httperr.MapError
// pueda convertir domain.ErrDocumentNotFound → 404.
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrDocumentNotFound):
			return serrors.ErrNotFound("Documento no encontrado", err)
		case errors.Is(err, domain.ErrInvalidInput),
			errors.Is(err, domain.ErrValidationError):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)
		}
		return nil
	})
}

// =============================================================================
// Fixtures locales del handler
// =============================================================================

func injectClaims(c *echo.Context, userID uuid.UUID) {
	c.Set("user_claims", &sharedauth.AccessClaims{
		UserID: userID,
		Email:  "admin@test.com",
		Role:   "admin",
	})
}

// =============================================================================
// Tests
// =============================================================================

// TestHandler_Reprocess_Success — DR-1.1: POST retorna 202.
func TestHandler_Reprocess_Success(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocReprocessRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDoc(docID, "failed"), nil
		},
		updateOCR: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
	}

	uc := docreprocess.NewUseCase(repo, nil, nil)
	h := docreprocess.NewHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/documents/"+docID.String()+"/reprocess", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/dashboard/documents/:id/reprocess")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: docID.String()}})
	injectClaims(c, adminID)

	err := h.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusAccepted)
	}
}

// TestHandler_Reprocess_NotFound — DR-1.2: POST 404.
func TestHandler_Reprocess_NotFound(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocReprocessRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return nil, domain.ErrDocumentNotFound
		},
	}

	uc := docreprocess.NewUseCase(repo, nil, nil)
	h := docreprocess.NewHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/documents/"+docID.String()+"/reprocess", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/dashboard/documents/:id/reprocess")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: docID.String()}})
	injectClaims(c, adminID)

	_ = h.Handle(c)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandler_Reprocess_NoAuth — sin claims → validation error.
func TestHandler_Reprocess_NoAuth(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocReprocessRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDoc(docID, "failed"), nil
		},
		updateOCR: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
	}

	uc := docreprocess.NewUseCase(repo, nil, nil)
	h := docreprocess.NewHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/documents/"+docID.String()+"/reprocess", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/dashboard/documents/:id/reprocess")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: docID.String()}})
	// No se inyectan claims

	_ = h.Handle(c)

	if rec.Code < 400 {
		t.Errorf("status = %d, expected validation error (actorID nil)", rec.Code)
	}
}

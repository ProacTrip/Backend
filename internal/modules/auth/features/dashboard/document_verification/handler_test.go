// Tests del handler HTTP de verificación de documentos.
// DV-REQ-1: GET 200 con historial.
// DV-REQ-2: PATCH 200 con cambio de estado.
// Los stubs (stubDocVerificationRepo, testDocRow) están definidos en usecase_test.go
// (mismo package document_verification_test).
package document_verification_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	docverification "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/document_verification"
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

func injectAdminClaims(c *echo.Context, userID uuid.UUID) {
	c.Set("user_claims", &sharedauth.AccessClaims{
		UserID: userID,
		Email:  "admin@test.com",
		Role:   "admin",
	})
}

func newTestHandler(repo *stubDocVerificationRepo) *docverification.Handler {
	uc := docverification.NewUseCase(repo)
	return docverification.NewHandler(uc)
}

func newEchoContext(method, path, docID string, body string) (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path+"/"+docID+"/verification", strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(path + "/:id/verification")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: docID}})
	return e, c, rec
}

// =============================================================================
// GET Tests
// =============================================================================

// TestHandleGet_Success — DV-1.1: GET 200 con historial.
func TestHandleGet_Success(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "verified"), nil
		},
		getHistory: func(ctx context.Context, docID uuid.UUID) ([]domain.HistoryEntry, error) {
			return []domain.HistoryEntry{
				{NewStatus: "verified", VerifiedBy: adminID},
			}, nil
		},
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodGet, "/v1/dashboard/documents", docID.String(), "")
	injectAdminClaims(c, adminID)

	err := h.HandleGet(c)
	if err != nil {
		t.Fatalf("HandleGet() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"document_id"`) {
		t.Errorf("response missing document_id: %s", rec.Body.String())
	}
}

// TestHandleGet_NotFound — DV-1.3: GET 404.
func TestHandleGet_NotFound(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return nil, domain.ErrDocumentNotFound
		},
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodGet, "/v1/dashboard/documents", docID.String(), "")

	_ = h.HandleGet(c)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleGet_NeverVerified — DV-1.2: documento sin historial.
func TestHandleGet_NeverVerified(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "unverified"), nil
		},
		getHistory: func(ctx context.Context, docID uuid.UUID) ([]domain.HistoryEntry, error) {
			return []domain.HistoryEntry{}, nil
		},
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodGet, "/v1/dashboard/documents", docID.String(), "")

	err := h.HandleGet(c)
	if err != nil {
		t.Fatalf("HandleGet() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"unverified"`) {
		t.Errorf("response missing unverified status: %s", rec.Body.String())
	}
}

// =============================================================================
// PATCH Tests
// =============================================================================

// TestHandlePatch_Success — DV-2.1: PATCH 200.
func TestHandlePatch_Success(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "unverified"), nil
		},
		updateStatus: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
		insertHistory: func(ctx context.Context, entry domain.HistoryEntry) error {
			return nil
		},
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodPatch, "/v1/dashboard/documents", docID.String(),
		`{"status":"verified"}`)
	injectAdminClaims(c, adminID)

	err := h.HandlePatch(c)
	if err != nil {
		t.Fatalf("HandlePatch() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusOK)
	}
}

// TestHandlePatch_InvalidStatus — DV-2.5: pending rechazado.
func TestHandlePatch_InvalidStatus(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: nil, // no debería llamarse porque falla en Validate()
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodPatch, "/v1/dashboard/documents", docID.String(),
		`{"status":"pending"}`)
	injectAdminClaims(c, adminID)

	_ = h.HandlePatch(c)

	if rec.Code < 400 {
		t.Errorf("status = %d, expected >= 400 for invalid status 'pending'", rec.Code)
	}
}

// TestHandlePatch_NotFound — DV-2.8: documento no encontrado.
func TestHandlePatch_NotFound(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return nil, domain.ErrDocumentNotFound
		},
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodPatch, "/v1/dashboard/documents", docID.String(),
		`{"status":"rejected"}`)
	injectAdminClaims(c, adminID)

	_ = h.HandlePatch(c)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandlePatch_NoAuth — sin claims → validation error.
func TestHandlePatch_NoAuth(t *testing.T) {
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "unverified"), nil
		},
		updateStatus: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
		insertHistory: func(ctx context.Context, entry domain.HistoryEntry) error {
			return nil
		},
	}

	h := newTestHandler(repo)
	_, c, rec := newEchoContext(http.MethodPatch, "/v1/dashboard/documents", docID.String(),
		`{"status":"verified"}`)
	// No se inyectan claims — extractActorID retorna uuid.Nil

	_ = h.HandlePatch(c)

	if rec.Code < 400 {
		t.Errorf("status = %d, expected validation error (verified_by is nil UUID)", rec.Code)
	}
}

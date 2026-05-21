package register

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Test setup — register domain error mappers for test isolation
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrInvalidEmail):
			return serrors.ErrValidationError("Dirección de correo inválida", err)
		case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrValidationError):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)
		case errors.Is(err, domain.ErrPasswordTooShort), errors.Is(err, domain.ErrWeakPassword):
			return serrors.ErrBadRequest("La contraseña no cumple los requisitos de seguridad", err)
		case errors.Is(err, domain.ErrEmailAlreadyExists):
			return serrors.ErrConflict("El email ya está registrado", err)
		}
		return nil
	})
}

// =============================================================================
// Test: handler acepta petición válida y retorna 201 Created.
// El usecase ya no resuelve environment defaults — el evento se publica
// con campos de entorno vacíos. El user consumer los resuelve por su cuenta.
// =============================================================================

func TestHandler_ValidRequest_ReturnsCreated(t *testing.T) {
	e := echo.New()

	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "vt-valid"},
		Hasher:         &mockPasswordHasher{},
		EventPublisher: publisher,
	})

	handler := NewHandler(uc)

	body := `{"email":"valid@test.com","password":"Password123!","first_name":"Juan"}`
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Idempotency-Key", "019d5439-cb43-716d-90b5-51dcbe980908")
	req.RemoteAddr = "203.0.113.42:54321"

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status code = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Verify event was published
	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}
}

// =============================================================================
// Test: handler rechaza petición sin Idempotency-Key.
// =============================================================================

func TestHandler_MissingIdempotencyKey_ReturnsError(t *testing.T) {
	e := echo.New()

	repo := newMockUserRepo()

	uc := NewUseCase(UseCaseDeps{
		Repo:      repo,
		VerifySvc: &mockVerificationService{token: "vt"},
		Hasher:    &mockPasswordHasher{},
	})

	handler := NewHandler(uc)

	body := `{"email":"noidem@test.com","password":"Password123!"}`
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	// Sin Idempotency-Key header

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handle(c)

	// MapError handles the error internally (writes to response, returns nil)
	if err != nil {
		t.Logf("Handle returned error (also valid): %v", err)
	}

	if rec.Code < 400 || rec.Code >= 500 {
		t.Errorf("expected 4xx status for missing Idempotency-Key, got %d. Body: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// Test: handler con body malformado (JSON inválido).
// =============================================================================

func TestHandler_InvalidJSON_ReturnsError(t *testing.T) {
	e := echo.New()

	repo := newMockUserRepo()

	uc := NewUseCase(UseCaseDeps{
		Repo:      repo,
		VerifySvc: &mockVerificationService{token: "vt"},
		Hasher:    &mockPasswordHasher{},
	})

	handler := NewHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{invalid json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Idempotency-Key", "019d5439-cb43-716d-90b5-51dcbe980908")

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handle(c)

	if err != nil {
		t.Logf("Handle returned error (also valid): %v", err)
	}

	if rec.Code < 400 || rec.Code >= 500 {
		t.Errorf("expected 4xx status for invalid JSON, got %d. Body: %s", rec.Code, rec.Body.String())
	}
}

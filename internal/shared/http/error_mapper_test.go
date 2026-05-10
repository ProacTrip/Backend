package http_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	sharedhttp "github.com/ProacTrip/Backend/internal/shared/http"
)

// =============================================================================
// Helper: registrar un subset de auth domain error mappers para testing.
// =============================================================================

func registerAuthDomainMappers() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		// Autenticación y credenciales
		case errors.Is(err, domain.ErrInvalidCredentials):
			return serrors.ErrUnauthorized("Credenciales inválidas", err)
		case errors.Is(err, domain.ErrNotAuthenticated):
			return serrors.ErrUnauthorized("se requiere autenticación", err)
		case errors.Is(err, domain.ErrTokenInvalid):
			return serrors.ErrUnauthorized("token inválido o expirado", err)

		// Cuenta
		case errors.Is(err, domain.ErrAccountLocked):
			return serrors.ErrTooManyRequests("Cuenta bloqueada temporalmente", err)
		case errors.Is(err, domain.ErrAccountSuspended):
			return serrors.ErrForbidden("Cuenta suspendida", err)
		case errors.Is(err, domain.ErrUserNotFound):
			return serrors.ErrNotFound("Usuario no encontrado", err)
		case errors.Is(err, domain.ErrEmailAlreadyExists):
			return serrors.ErrConflict("El email ya está registrado", err)

		// OAuth
		case errors.Is(err, domain.ErrOAuthStateInvalid):
			return serrors.ErrBadRequest("state OAuth inválido o expirado", err)
		case errors.Is(err, domain.ErrOAuthExchangeFailed):
			return serrors.ErrUnauthorized("Error de autenticación OAuth", err)

		// Roles / Permisos
		case errors.Is(err, domain.ErrPermissionDenied):
			return serrors.ErrForbidden("Permiso denegado", err)
		}
		return nil
	})
}

// =============================================================================
// Helper: crear echo.Context con httptest.
// =============================================================================

func newTestContext(method, path string) (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return e, c, rec
}

// =============================================================================
// Escenario 1: map_domain_to_rfc9457 — convertir echo.HTTPError a Problem RFC 9457.
// =============================================================================

func TestMapError_EchoHTTPError_ConvierteARFC9457ProblemDetails(t *testing.T) {
	e, c, rec := newTestContext(http.MethodGet, "/api/v1/test")

	// Crear un error HTTP de Echo
	he := echo.NewHTTPError(http.StatusNotFound, "recurso no encontrado")

	err := sharedhttp.MapError(c, he)
	if err != nil {
		t.Fatalf("MapError devolvió error: %v", err)
	}

	// Verificar Content-Type RFC 9457
	contentType := rec.Header().Get(echo.HeaderContentType)
	if contentType != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/problem+json")
	}

	// Verificar status code
	if rec.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusNotFound)
	}

	// Verificar campos del Problem
	var problem serrors.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("error al deserializar Problem: %v", err)
	}

	if problem.Type != serrors.ProblemTypeNotFound {
		t.Errorf("Type = %q, want %q", problem.Type, serrors.ProblemTypeNotFound)
	}
	if problem.Title != "Not Found" {
		t.Errorf("Title = %q, want %q", problem.Title, "Not Found")
	}
	if problem.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", problem.Status, http.StatusNotFound)
	}
	if problem.Detail != "recurso no encontrado" {
		t.Errorf("Detail = %q, want %q", problem.Detail, "recurso no encontrado")
	}
	if problem.Instance != "/api/v1/test" {
		t.Errorf("Instance = %q, want %q", problem.Instance, "/api/v1/test")
	}
	// trace_id debe ser un UUID no vacío
	if problem.TraceID == "" {
		t.Error("TraceID está vacío")
	}
	if _, err := uuid.Parse(problem.TraceID); err != nil {
		t.Errorf("TraceID no es un UUID válido: %v", err)
	}

	_ = e // suppress unused
}

// =============================================================================
// Escenario 2: preserva_x_trace_id — X-Trace-Id seteado previamente se preserva.
// =============================================================================

func TestMapError_PreservaXTraceIdExistente_EnResponseBody(t *testing.T) {
	e, c, rec := newTestContext(http.MethodGet, "/api/v1/test")

	// Simular que un middleware ya seteó X-Trace-Id en el response header
	knownTraceID := "018f3a00-1111-7abc-8000-000000000001"
	c.Response().Header().Set("X-Trace-Id", knownTraceID)

	he := echo.NewHTTPError(http.StatusBadRequest, "error de validación")

	err := sharedhttp.MapError(c, he)
	if err != nil {
		t.Fatalf("MapError devolvió error: %v", err)
	}

	var problem serrors.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("error al deserializar Problem: %v", err)
	}

	if problem.TraceID != knownTraceID {
		t.Errorf("TraceID = %q, want %q (debe preservar el X-Trace-Id existente)", problem.TraceID, knownTraceID)
	}

	// Verificar que el header también contiene el trace_id
	respTraceID := rec.Header().Get("X-Trace-Id")
	if respTraceID != knownTraceID {
		t.Errorf("X-Trace-Id header = %q, want %q", respTraceID, knownTraceID)
	}

	_ = e
}

// =============================================================================
// Escenario 3: errores_conocidos — table-driven sobre errores de dominio → HTTP.
// =============================================================================

func TestMapError_ErroresDominioConocidos_MapeaAHTTPCorrecto(t *testing.T) {
	registerAuthDomainMappers()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantTitle  string
	}{
		{
			name:       "ErrInvalidCredentials → 401 Unauthorized",
			err:        domain.ErrInvalidCredentials,
			wantStatus: http.StatusUnauthorized,
			wantTitle:  "Unauthorized",
		},
		{
			name:       "ErrUserNotFound → 404 Not Found",
			err:        domain.ErrUserNotFound,
			wantStatus: http.StatusNotFound,
			wantTitle:  "Not Found",
		},
		{
			name:       "ErrAccountLocked → 429 Too Many Requests",
			err:        domain.ErrAccountLocked,
			wantStatus: http.StatusTooManyRequests,
			wantTitle:  "Too Many Requests",
		},
		{
			name:       "ErrNotAuthenticated → 401 Unauthorized",
			err:        domain.ErrNotAuthenticated,
			wantStatus: http.StatusUnauthorized,
			wantTitle:  "Unauthorized",
		},
		{
			name:       "ErrEmailAlreadyExists → 409 Conflict",
			err:        domain.ErrEmailAlreadyExists,
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict",
		},
		{
			name:       "ErrPermissionDenied → 403 Forbidden",
			err:        domain.ErrPermissionDenied,
			wantStatus: http.StatusForbidden,
			wantTitle:  "Forbidden",
		},
		{
			name:       "ErrAccountSuspended → 403 Forbidden",
			err:        domain.ErrAccountSuspended,
			wantStatus: http.StatusForbidden,
			wantTitle:  "Forbidden",
		},
		{
			name:       "ErrTokenInvalid → 401 Unauthorized",
			err:        domain.ErrTokenInvalid,
			wantStatus: http.StatusUnauthorized,
			wantTitle:  "Unauthorized",
		},
		{
			name:       "ErrOAuthStateInvalid → 400 Bad Request",
			err:        domain.ErrOAuthStateInvalid,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Bad Request",
		},
		{
			name:       "ErrOAuthExchangeFailed → 401 Unauthorized",
			err:        domain.ErrOAuthExchangeFailed,
			wantStatus: http.StatusUnauthorized,
			wantTitle:  "Unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newTestContext(http.MethodGet, "/api/v1/auth/test")

			err := sharedhttp.MapError(c, tt.err)
			if err != nil {
				t.Fatalf("MapError devolvió error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatus)
			}

			var problem serrors.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("error al deserializar Problem: %v", err)
			}

			if problem.Status != tt.wantStatus {
				t.Errorf("Problem.Status = %d, want %d", problem.Status, tt.wantStatus)
			}
			if problem.Title != tt.wantTitle {
				t.Errorf("Problem.Title = %q, want %q", problem.Title, tt.wantTitle)
			}

			// Campos obligatorios RFC 9457
			if problem.Type == "" {
				t.Error("Problem.Type está vacío")
			}
			if problem.Detail == "" {
				t.Error("Problem.Detail está vacío")
			}
			if problem.TraceID == "" {
				t.Error("Problem.TraceID está vacío")
			}
		})
	}
}

// =============================================================================
// Escenario extra: error no conocido → fallback genérico 500.
// =============================================================================

func TestMapError_ErrorNoConocido_Devuelve500Generico(t *testing.T) {
	_, c, rec := newTestContext(http.MethodGet, "/api/v1/test")

	// Error genérico que no está en ningún mapper
	unknownErr := errors.New("algo explotó internamente")

	err := sharedhttp.MapError(c, unknownErr)
	if err != nil {
		t.Fatalf("MapError devolvió error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var problem serrors.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("error al deserializar Problem: %v", err)
	}

	if problem.Type != serrors.ProblemTypeInternalError {
		t.Errorf("Type = %q, want %q", problem.Type, serrors.ProblemTypeInternalError)
	}
	if problem.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", problem.Status, http.StatusInternalServerError)
	}
}

// =============================================================================
// Escenario extra: serrors.Problem ya convertido → se retorna tal cual.
// =============================================================================

func TestMapError_ProblemYaConvertido_SeRetornaDirecto(t *testing.T) {
	_, c, rec := newTestContext(http.MethodGet, "/api/v1/test")

	// Crear un Problem directamente (simula un error ya convertido)
	original := serrors.ErrConflict("recurso duplicado", nil)
	original.TraceID = "018f3a00-2222-7abc-8000-000000000002"

	err := sharedhttp.MapError(c, original)
	if err != nil {
		t.Fatalf("MapError devolvió error: %v", err)
	}

	if rec.Code != http.StatusConflict {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusConflict)
	}

	var problem serrors.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("error al deserializar Problem: %v", err)
	}

	if problem.TraceID != original.TraceID {
		t.Errorf("TraceID = %q, want %q", problem.TraceID, original.TraceID)
	}
	if problem.Title != "Conflict" {
		t.Errorf("Title = %q, want %q", problem.Title, "Conflict")
	}
}

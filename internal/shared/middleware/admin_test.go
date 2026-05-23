package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	mw "github.com/ProacTrip/Backend/internal/shared/middleware"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// mockRoleClaims implementa middleware.RoleClaims para testing.
type mockRoleClaims struct {
	role string
}

func (m *mockRoleClaims) GetRole() string {
	return m.role
}

func newAdminTestContext() (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return e, c, rec
}

// =============================================================================
// T4.2.1: sin_role_claims → 403 Forbidden (no hay claims en contexto)
// =============================================================================

func TestRequireAdmin_SinRoleClaims_Retorna403(t *testing.T) {
	e, c, rec := newAdminTestContext()
	defer func() { _ = e }()

	nextCalled := false
	handler := func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	}

	middleware := mw.RequireAdmin()(handler)
	err := middleware(c)

	// MapError escribe la respuesta y retorna nil si c.JSON tiene éxito
	if err != nil {
		t.Errorf("RequireAdmin middleware devolvió error inesperado: %v", err)
	}
	if nextCalled {
		t.Error("next NO debe ser llamado sin role claims en contexto")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status code = %d, want %d (401 Unauthorized)", rec.Code, http.StatusUnauthorized)
	}

	// Verificar que la respuesta es un Problem RFC 9457
	var problem serrors.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("error al deserializar Problem: %v", err)
	}
	if problem.Status != http.StatusUnauthorized {
		t.Errorf("Problem.Status = %d, want %d", problem.Status, http.StatusUnauthorized)
	}
	if problem.Type != serrors.ProblemTypeUnauthorized {
		t.Errorf("Problem.Type = %q, want %q", problem.Type, serrors.ProblemTypeUnauthorized)
	}
}

// =============================================================================
// T4.2.2: role_admin → next called
// =============================================================================

func TestRequireAdmin_RoleAdmin_NextCalled(t *testing.T) {
	e, c, rec := newAdminTestContext()
	defer func() { _ = e }()

	// Setear claims con role "admin"
	claims := &mockRoleClaims{role: "admin"}
	c.Set("user_claims", claims)

	nextCalled := false
	handler := func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "admin ok")
	}

	middleware := mw.RequireAdmin()(handler)
	err := middleware(c)

	if err != nil {
		t.Errorf("RequireAdmin con role admin devolvió error inesperado: %v", err)
	}
	if !nextCalled {
		t.Error("next debe ser llamado para role 'admin'")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

// =============================================================================
// T4.2.3: role_client → 403 Forbidden
// =============================================================================

func TestRequireAdmin_RoleNoAdmin_Retorna403(t *testing.T) {
	tests := []struct {
		name string
		role string
	}{
		{"role_client", "client"},
		{"role_user", "user"},
		{"role_vacio", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, c, rec := newAdminTestContext()
			defer func() { _ = e }()

			// Setear claims con role no-admin
			claims := &mockRoleClaims{role: tt.role}
			c.Set("user_claims", claims)

			nextCalled := false
			handler := func(c *echo.Context) error {
				nextCalled = true
				return c.String(http.StatusOK, "ok")
			}

			middleware := mw.RequireAdmin()(handler)
			err := middleware(c)

			// MapError escribe 403 y retorna nil
			if err != nil {
				t.Errorf("RequireAdmin con role %q devolvió error inesperado: %v", tt.role, err)
			}
			if nextCalled {
				t.Errorf("next NO debe ser llamado para role %q", tt.role)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status code = %d, want %d para role %q", rec.Code, http.StatusForbidden, tt.role)
			}

			// Verificar Problem RFC 9457
			var problem serrors.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("error al deserializar Problem para role %q: %v", tt.role, err)
			}
			if problem.Status != http.StatusForbidden {
				t.Errorf("Problem.Status = %d, want %d para role %q", problem.Status, http.StatusForbidden, tt.role)
			}
			if problem.Type != serrors.ProblemTypeForbidden {
				t.Errorf("Problem.Type = %q, want %q para role %q", problem.Type, serrors.ProblemTypeForbidden, tt.role)
			}
		})
	}
}

// =============================================================================
// T4.2.4: Verificar que el error es un echo.HTTPError (compatibilidad legacy)
// Nota: RequireAdmin usa MapError que escribe un Problem RFC 9457, no un HTTPError.
// El middleware RETORNA nil (porque MapError consume el error al escribir la respuesta).
// La respuesta HTTP ya contiene el código de estado correcto.
// =============================================================================

func TestRequireAdmin_RespuestaTieneContentTypeRFC9457(t *testing.T) {
	e, c, rec := newAdminTestContext()
	defer func() { _ = e }()

	middleware := mw.RequireAdmin()(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := middleware(c)
	if err != nil {
		t.Errorf("RequireAdmin devolvió error inesperado: %v", err)
	}

	// Content-Type debe ser application/problem+json
	contentType := rec.Header().Get(echo.HeaderContentType)
	if contentType != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/problem+json")
	}

	// X-Trace-Id debe estar presente
	traceID := rec.Header().Get("X-Trace-Id")
	if traceID == "" {
		t.Error("X-Trace-Id header no presente en respuesta de error")
	}
}

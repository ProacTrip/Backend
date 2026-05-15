package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v5"
)

// =============================================================================
// mockClaims — implementa PermissionClaims para testing
// =============================================================================

type mockClaims struct {
	permissions []string
}

func (m mockClaims) GetPermissions() []string { return m.permissions }

// =============================================================================
// setupEcho — crea un contexto Echo para testing del middleware
// =============================================================================

func setupEcho(t *testing.T, permissions []string, path string) (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Inyectar claims en el contexto
	c.Set("user_claims", mockClaims{permissions: permissions})

	return e, c, rec
}

func TestRequirePermission_HasPermission(t *testing.T) {
	os.Unsetenv("AUTHZ_ENFORCE_MODE") // Modo enforce (default)
	t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

	e, c, rec := setupEcho(t, []string{"users:read", "users:write"}, "/v1/dashboard/users")

	mw := RequirePermission("users:read")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	_ = e
}

func TestRequirePermission_MissingPermission(t *testing.T) {
	os.Unsetenv("AUTHZ_ENFORCE_MODE") // Modo enforce
	t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

	_, c, rec := setupEcho(t, []string{"users:read"}, "/v1/dashboard/users")

	mw := RequirePermission("users:write")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	// MapError devuelve JSON, así que verificamos el código
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_ObserveModeAllowsWithoutPermission(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe")
	t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

	_, c, rec := setupEcho(t, []string{"users:read"}, "/v1/dashboard/users")

	mw := RequirePermission("users:write")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	// En modo observe, debe permitir el request aunque falte el permiso
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (observe mode should allow)", rec.Code, http.StatusOK)
	}
}

func TestRequirePermission_ObserveModeAllowsWithPermission(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe")
	t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

	_, c, rec := setupEcho(t, []string{"users:read", "users:write"}, "/v1/dashboard/users")

	mw := RequirePermission("users:read")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequirePermission_EmptyPermissions(t *testing.T) {
	os.Unsetenv("AUTHZ_ENFORCE_MODE")
	t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

	_, c, rec := setupEcho(t, []string{}, "/v1/dashboard/users")

	mw := RequirePermission("users:read")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_NilPermissionsSlice(t *testing.T) {
	os.Unsetenv("AUTHZ_ENFORCE_MODE")
	t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

	_, c, rec := setupEcho(t, nil, "/v1/dashboard/users")

	mw := RequirePermission("users:read")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (nil slice should be treated as empty)", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_MissingClaimsContext(t *testing.T) {
	// No claims in context → should be 500 internal error
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// Not setting user_claims

	mw := RequirePermission("users:read")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestIsObserveMode(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{"observe mode", "observe", true},
		{"enforce mode", "enforce", false},
		{"empty string", "", false},
		{"unknown value", "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("AUTHZ_ENFORCE_MODE", tt.env)
			t.Cleanup(func() { os.Unsetenv("AUTHZ_ENFORCE_MODE") })

			if got := isObserveMode(); got != tt.want {
				t.Errorf("isObserveMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Ensure mockClaims satisface la interfaz en compile-time.
var _ PermissionClaims = mockClaims{}

// Ensure context cancellation propagation doesn't interfere.
func TestRequirePermission_ContextCancelled(t *testing.T) {
	os.Unsetenv("AUTHZ_ENFORCE_MODE")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Cancel the context
	ctx, cancel := context.WithCancel(c.Request().Context())
	cancel()
	req = req.WithContext(ctx)
	c = e.NewContext(req, rec)
	c.Set("user_claims", mockClaims{permissions: []string{"users:read"}})

	mw := RequirePermission("users:read")
	handler := mw(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Handler should still work (or return context error from downstream)
	_ = handler(c)
	// No panic
}

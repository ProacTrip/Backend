package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	mw "github.com/ProacTrip/Backend/internal/shared/middleware"
)

func newSecurityHeadersTestContext() (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return e, c, rec
}

// =============================================================================
// T4.3.1: todos_los_headers — verificar TODOS los headers de seguridad presentes
// =============================================================================

func TestSecurityHeaders_TodosLosHeaders_Presentes(t *testing.T) {
	e, c, rec := newSecurityHeadersTestContext()
	defer func() { _ = e }()

	nextCalled := false
	handler := func(c *echo.Context) error {
		nextCalled = true
		return nil
	}

	middleware := mw.SecurityHeaders()(handler)
	err := middleware(c)

	if err != nil {
		t.Fatalf("SecurityHeaders middleware devolvió error: %v", err)
	}
	if !nextCalled {
		t.Error("el handler next debe ser llamado")
	}

	headers := rec.Header()

	// CSP
	if got := headers.Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("Content-Security-Policy = %q, want %q", got, "default-src 'self'")
	}

	// X-Content-Type-Options
	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}

	// X-Frame-Options
	if got := headers.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want %q", got, "DENY")
	}

	// Referrer-Policy
	if got := headers.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q, want %q", got, "strict-origin-when-cross-origin")
	}

	// HSTS
	hsts := headers.Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("Strict-Transport-Security header no presente")
	}
}

// =============================================================================
// T4.3.2: hsts_include_subdomains — verificar que HSTS contiene includeSubDomains
// =============================================================================

func TestSecurityHeaders_HSTS_ContieneIncludeSubDomains(t *testing.T) {
	e, c, rec := newSecurityHeadersTestContext()
	defer func() { _ = e }()

	middleware := mw.SecurityHeaders()(func(c *echo.Context) error {
		return nil
	})
	err := middleware(c)

	if err != nil {
		t.Fatalf("SecurityHeaders middleware devolvió error: %v", err)
	}

	hsts := rec.Header().Get("Strict-Transport-Security")
	if hsts != "max-age=31536000; includeSubDomains" {
		t.Errorf("Strict-Transport-Security = %q, want %q", hsts, "max-age=31536000; includeSubDomains")
	}
}

// =============================================================================
// T4.3.3: next_called — verificar que el next handler es invocado después de
// setear los headers.
// =============================================================================

func TestSecurityHeaders_NextCalled_DespuesDeSetearHeaders(t *testing.T) {
	e, c, rec := newSecurityHeadersTestContext()
	defer func() { _ = e }()

	headerSetInNext := false
	nextCalled := false

	handler := func(c *echo.Context) error {
		nextCalled = true
		// Verificar que los headers YA están seteados cuando next se ejecuta
		if c.Response().Header().Get("X-Frame-Options") == "DENY" {
			headerSetInNext = true
		}
		return nil
	}

	middleware := mw.SecurityHeaders()(handler)
	err := middleware(c)

	if err != nil {
		t.Fatalf("SecurityHeaders middleware devolvió error: %v", err)
	}
	if !nextCalled {
		t.Error("el handler next no fue llamado")
	}
	if !headerSetInNext {
		t.Error("los headers de seguridad NO estaban seteados cuando next fue llamado")
	}

	_ = rec
}

// =============================================================================
// T4.3 extra: verificar que los headers persisten en la respuesta final
// =============================================================================

func TestSecurityHeaders_HeadersPersistenEnRespuestaFinal(t *testing.T) {
	e, c, rec := newSecurityHeadersTestContext()
	defer func() { _ = e }()

	middleware := mw.SecurityHeaders()(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	err := middleware(c)

	if err != nil {
		t.Fatalf("SecurityHeaders middleware devolvió error: %v", err)
	}

	// Verificar que todos los headers están en el recorder final
	headers := rec.Header()

	assertHeader := func(name, want string) {
		t.Helper()
		if got := headers.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	assertHeader("Content-Security-Policy", "default-src 'self'")
	assertHeader("X-Content-Type-Options", "nosniff")
	assertHeader("X-Frame-Options", "DENY")
	assertHeader("Referrer-Policy", "strict-origin-when-cross-origin")
	assertHeader("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

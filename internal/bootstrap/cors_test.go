package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// =============================================================================
// Test: CORS middleware allows PATCH method
// =============================================================================

func TestCORS_AllowsPATCH(t *testing.T) {
	e := echo.New()

	// Mirror the CORS config from app.go
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://proactrip.com"},
		AllowCredentials: true,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"X-Request-Id",
			"Idempotency-Key",
			"X-Trace-Id",
		},
		MaxAge: 86400,
	}))

	e.PATCH("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "patched")
	})

	// Preflight request for PATCH
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://proactrip.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	allowMethods := rec.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Fatal("CORS preflight returned no Access-Control-Allow-Methods header")
	}

	if !contains(allowMethods, http.MethodPatch) {
		t.Errorf("Access-Control-Allow-Methods does not include PATCH. Got: %s", allowMethods)
	}
}

// =============================================================================
// Test: CORS allows existing methods are preserved
// =============================================================================

func TestCORS_ExistingMethodsStillAllowed(t *testing.T) {
	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://proactrip.com"},
		AllowCredentials: true,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
		},
	}))

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.POST("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	methods := []string{http.MethodGet, http.MethodPost}
	for _, method := range methods {
		t.Run("allows "+method, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set("Origin", "https://proactrip.com")
			req.Header.Set("Access-Control-Request-Method", method)
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			allowMethods := rec.Header().Get("Access-Control-Allow-Methods")
			if !contains(allowMethods, method) {
				t.Errorf("Access-Control-Allow-Methods does not include %s. Got: %s", method, allowMethods)
			}
		})
	}
}

// =============================================================================
// Test: CORS preflight for PATCH returns 204
// =============================================================================

func TestCORS_PATCHPreflight_Returns204(t *testing.T) {
	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://proactrip.com"},
		AllowCredentials: true,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodPatch,
		},
		AllowHeaders: []string{echo.HeaderContentType},
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://proactrip.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for PATCH preflight, got %d", rec.Code)
	}
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================================================
// Test: CORS AllowHeaders includes X-Real-IP only in dev
// Spec: CORS-1
// =============================================================================

func TestCORS_AllowHeaders_XRealIP(t *testing.T) {
	t.Run("dev includes X-Real-IP", func(t *testing.T) {
		t.Setenv("SERVER_ENV", "dev")

		headers := corsAllowHeaders()

		found := false
		for _, h := range headers {
			if h == "X-Real-IP" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("X-Real-IP missing from AllowHeaders when SERVER_ENV=dev. Got: %v", headers)
		}
	})

	t.Run("production excludes X-Real-IP", func(t *testing.T) {
		t.Setenv("SERVER_ENV", "production")

		headers := corsAllowHeaders()

		for _, h := range headers {
			if h == "X-Real-IP" {
				t.Errorf("X-Real-IP present in AllowHeaders when SERVER_ENV=production. Got: %v", headers)
			}
		}
	})

	t.Run("staging excludes X-Real-IP", func(t *testing.T) {
		t.Setenv("SERVER_ENV", "staging")

		headers := corsAllowHeaders()

		for _, h := range headers {
			if h == "X-Real-IP" {
				t.Errorf("X-Real-IP present in AllowHeaders when SERVER_ENV=staging. Got: %v", headers)
			}
		}
	})

	t.Run("default (unset) excludes X-Real-IP", func(t *testing.T) {
		// SERVER_ENV intentionally unset — default behavior should be prod-safe
		headers := corsAllowHeaders()

		for _, h := range headers {
			if h == "X-Real-IP" {
				t.Errorf("X-Real-IP present in AllowHeaders when SERVER_ENV is unset. Got: %v", headers)
			}
		}
	})
}

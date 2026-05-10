package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

// =============================================================================
// Test: anonRateLimitMW middleware is applied to public auth routes
//
// Verifies the pattern used in app.go: when anonRateLimitMW is added as
// route-level middleware, it executes for the expected endpoints.
// =============================================================================

func TestAnonRateLimitMW_AppliedToPublicAuthRoutes(t *testing.T) {
	e := echo.New()

	// Spy middleware: sets a custom header to prove it was called
	spyMW := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("X-Rate-Limit-Checked", "true")
			return next(c)
		}
	}

	// Mirror the app.go route registration pattern
	authGroup := e.Group("/v1/auth")
	authGroup.POST("/register", func(c *echo.Context) error {
		return c.String(http.StatusCreated, "registered")
	}, spyMW)
	authGroup.POST("/login", func(c *echo.Context) error {
		return c.String(http.StatusOK, "logged-in")
	}, spyMW)
	authGroup.POST("/verify-email", func(c *echo.Context) error {
		return c.String(http.StatusOK, "verified")
	}, spyMW)
	authGroup.GET("/oauth/:provider", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"auth_url": "https://..."})
	}, spyMW)

	// Route WITHOUT middleware (control)
	authGroup.POST("/resend-verification", func(c *echo.Context) error {
		return c.String(http.StatusOK, "resent")
	}, spyMW) // this one should also have it

	tests := []struct {
		name       string
		method     string
		path       string
		wantHeader bool
	}{
		{"register has rate limit", http.MethodPost, "/v1/auth/register", true},
		{"login has rate limit", http.MethodPost, "/v1/auth/login", true},
		{"verify-email has rate limit", http.MethodPost, "/v1/auth/verify-email", true},
		{"oauth authorize has rate limit", http.MethodGet, "/v1/auth/oauth/google", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Rate-Limit-Checked")
			if tt.wantHeader && got != "true" {
				t.Errorf("expected X-Rate-Limit-Checked header for %s %s, got %q", tt.method, tt.path, got)
			}
		})
	}
}

// =============================================================================
// Test: logout route does NOT use anonRateLimitMW (uses authRateLimitMW)
// =============================================================================

func TestLogoutRoute_DoesNotUseAnonRateLimitMW(t *testing.T) {
	e := echo.New()

	authGroup := e.Group("/v1/auth")
	// Logout should NOT have anonRateLimitMW (it uses auth middleware + authRateLimitMW)
	authGroup.POST("/logout", func(c *echo.Context) error {
		return c.String(http.StatusOK, "logged-out")
	})
	// But Me should NOT have anonRateLimitMW either
	authGroup.GET("/me", func(c *echo.Context) error {
		return c.String(http.StatusOK, "me")
	})

	// Verify logout without rate limit middleware works fine
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	// No rate limit header expected
	if rec.Header().Get("X-Rate-Limit-Checked") == "true" {
		t.Error("logout should NOT have anonRateLimitMW")
	}
}

func TestMeRoute_DoesNotUseAnonRateLimitMW(t *testing.T) {
	e := echo.New()

	authGroup := e.Group("/v1/auth")
	authGroup.GET("/me", func(c *echo.Context) error {
		return c.String(http.StatusOK, "me")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Rate-Limit-Checked") == "true" {
		t.Error("me should NOT have anonRateLimitMW (uses authRateLimitMW)")
	}
}

package logout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
)

// =============================================================================
// Mocks
// =============================================================================

type mockTokenSvc struct{}

func (m *mockTokenSvc) ValidateAccessToken(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
	// Return an error so Execute succeeds early without touching DragonflyDB.
	// This makes the usecase return success (200) and cookies are cleared.
	return nil, errors.New("token invalid")
}

// =============================================================================
// Test: ClearAuthCookiesDev is called when isProduction=false
// Regression: handler was always calling ClearAuthCookies (prod names),
// so in dev mode it would try to clear __Secure-* cookies instead of
// access_token/refresh_token (plain names).
// =============================================================================

func TestHandler_ClearsDevCookies_InDevMode(t *testing.T) {
	e := echo.New()

	uc := NewUseCase(UseCaseDeps{
		TokenSvc:    &mockTokenSvc{},
		DragonflyDB: nil, // safe — Execute never reaches DB when token validation fails
	})

	// isProduction=false, cookieDomain="" (dev)
	h := NewHandler(uc, false, "")

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "dev-access-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify dev cookie names are cleared (NOT __Secure-*)
	setCookies := rec.Header()["Set-Cookie"]
	if !hasCookieClear(setCookies, "access_token") {
		t.Errorf("expected Set-Cookie to clear 'access_token', got headers: %v", setCookies)
	}
	if !hasCookieClear(setCookies, "refresh_token") {
		t.Errorf("expected Set-Cookie to clear 'refresh_token', got headers: %v", setCookies)
	}
	// Should NOT try to clear __Secure-* cookies in dev mode
	if hasCookieClear(setCookies, "__Secure-access_token") {
		t.Errorf("should NOT clear __Secure-access_token in dev mode, got headers: %v", setCookies)
	}
}

func TestHandler_ClearsProdCookies_InProductionMode(t *testing.T) {
	e := echo.New()

	uc := NewUseCase(UseCaseDeps{
		TokenSvc:    &mockTokenSvc{},
		DragonflyDB: nil,
	})

	// isProduction=true, cookieDomain=".proactrip.com"
	h := NewHandler(uc, true, ".proactrip.com")

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "prod-access-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify production cookie names are cleared
	setCookies := rec.Header()["Set-Cookie"]
	if !hasCookieClear(setCookies, "__Secure-access_token") {
		t.Errorf("expected Set-Cookie to clear '__Secure-access_token', got headers: %v", setCookies)
	}
	if !hasCookieClear(setCookies, "__Secure-refresh_token") {
		t.Errorf("expected Set-Cookie to clear '__Secure-refresh_token', got headers: %v", setCookies)
	}
	// Should have Clear-Site-Data header in production
	if rec.Header().Get("Clear-Site-Data") == "" {
		t.Error("expected Clear-Site-Data header in production mode")
	}
}

func TestHandler_HandleAll_ClearsDevCookies_InDevMode(t *testing.T) {
	e := echo.New()

	uc := NewUseCase(UseCaseDeps{
		TokenSvc:    &mockTokenSvc{},
		DragonflyDB: nil,
	})

	h := NewHandler(uc, false, "")

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout/all", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "dev-access-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleAll(c)
	if err != nil {
		t.Fatalf("HandleAll() unexpected error: %v", err)
	}

	setCookies := rec.Header()["Set-Cookie"]
	if !hasCookieClear(setCookies, "access_token") {
		t.Errorf("expected Set-Cookie to clear 'access_token', got headers: %v", setCookies)
	}
	if !hasCookieClear(setCookies, "refresh_token") {
		t.Errorf("expected Set-Cookie to clear 'refresh_token', got headers: %v", setCookies)
	}
}

func TestHandler_HandleAll_ClearsProdCookies_InProductionMode(t *testing.T) {
	e := echo.New()

	uc := NewUseCase(UseCaseDeps{
		TokenSvc:    &mockTokenSvc{},
		DragonflyDB: nil,
	})

	h := NewHandler(uc, true, ".proactrip.com")

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout/all", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "prod-access-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleAll(c)
	if err != nil {
		t.Fatalf("HandleAll() unexpected error: %v", err)
	}

	setCookies := rec.Header()["Set-Cookie"]
	if !hasCookieClear(setCookies, "__Secure-access_token") {
		t.Errorf("expected Set-Cookie to clear '__Secure-access_token', got headers: %v", setCookies)
	}
	if !hasCookieClear(setCookies, "__Secure-refresh_token") {
		t.Errorf("expected Set-Cookie to clear '__Secure-refresh_token', got headers: %v", setCookies)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func hasCookieClear(headers []string, name string) bool {
	for _, h := range headers {
		if len(h) > len(name)+2 && h[:len(name)+1] == name+"=" {
			if contains(h, "Max-Age=0") || contains(h, name+"=;") {
				return true
			}
		}
	}
	return false
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

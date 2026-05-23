package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/middleware"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Mock Token Service — implementa middleware.TokenService
// =============================================================================

type mockTokenSvc struct {
	validateAccessFn    func(ctx context.Context, tokenStr string) (*token.AccessClaims, error)
	validateRefreshFn   func(ctx context.Context, tokenStr string) (*token.RefreshClaims, error)
	validateAndRotateFn func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error)
	rotateWithVersionFn func(ctx context.Context, refreshToken string, tokenVersion int) (*token.RefreshClaims, string, string, error)
}

func (m *mockTokenSvc) ValidateAccessToken(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
	if m.validateAccessFn != nil {
		return m.validateAccessFn(ctx, tokenStr)
	}
	return nil, errors.New("mock: ValidateAccessToken no implementado")
}

func (m *mockTokenSvc) ValidateRefreshToken(ctx context.Context, tokenStr string) (*token.RefreshClaims, error) {
	if m.validateRefreshFn != nil {
		return m.validateRefreshFn(ctx, tokenStr)
	}
	return nil, errors.New("mock: ValidateRefreshToken no implementado")
}

func (m *mockTokenSvc) ValidateAndRotateRefresh(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
	if m.validateAndRotateFn != nil {
		return m.validateAndRotateFn(ctx, refreshToken)
	}
	return nil, "", "", errors.New("mock: ValidateAndRotateRefresh no implementado")
}

func (m *mockTokenSvc) ValidateAndRotateRefreshWithVersion(ctx context.Context, refreshToken string, tokenVersion int) (*token.RefreshClaims, string, string, error) {
	if m.rotateWithVersionFn != nil {
		return m.rotateWithVersionFn(ctx, refreshToken, tokenVersion)
	}
	return nil, "", "", errors.New("mock: ValidateAndRotateRefreshWithVersion no implementado")
}

// =============================================================================
// Helpers
// =============================================================================

var (
	testAccessClaims = &token.AccessClaims{
		UserID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Email:  "test@proactrip.com",
		RoleID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Role:   "client",
		JTI:    uuid.MustParse("44444444-4444-4444-4444-444444444444"),
	}

	testRefreshClaims = &token.RefreshClaims{
		UserID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Email:  "test@proactrip.com",
		RoleID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Role:   "client",
		JTI:    uuid.MustParse("55555555-5555-5555-5555-555555555555"),
	}
)

// findCookie busca una cookie por nombre en el slice de respuesta.
func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// =============================================================================
// TestHandle — AuthMiddleware.Handle()
// =============================================================================

func TestHandle(t *testing.T) {
	tests := []struct {
		name         string
		isProduction bool
		cookies      []*http.Cookie
		mock         *mockTokenSvc
		wantNext     bool
		wantStatus   int
		wantSetCookies []string // nombres de cookies esperadas en Set-Cookie
		wantClearCookies []string // nombres de cookies que deben estar limpias (Max-Age=0)
		wantClaims   bool
	}{
		{
			name:     "REQ-4.1 sin_cookies",
			isProduction: false,
			cookies:  nil,
			mock:     &mockTokenSvc{},
			wantNext: true,
			wantStatus: http.StatusOK,
			wantClaims: false,
		},
		{
			name:     "REQ-4.2 access_token_valido",
			isProduction: false,
			cookies: []*http.Cookie{
				{Name: "access_token", Value: "token-valido"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					if tokenStr == "token-valido" {
						return testAccessClaims, nil
					}
					return nil, domain.ErrTokenInvalid
				},
			},
			wantNext:   true,
			wantStatus: http.StatusOK,
			wantClaims: true,
		},
		{
			name:     "REQ-4.3 access_expirado_refresh_valido",
			isProduction: false,
			cookies: []*http.Cookie{
				{Name: "access_token", Value: "token-expirado"},
				{Name: "refresh_token", Value: "refresh-valido"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					return nil, domain.ErrTokenExpired
				},
				validateAndRotateFn: func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
					if refreshToken == "refresh-valido" {
						return testRefreshClaims, "nuevo-access-token", "nuevo-refresh-token", nil
					}
					return nil, "", "", domain.ErrTokenInvalid
				},
			},
			wantNext:       true,
			wantStatus:     http.StatusOK,
			wantSetCookies: []string{"access_token", "refresh_token"},
			wantClaims:     true,
		},
		{
			name:     "REQ-4.4 ambos_invalidos",
			isProduction: false,
			cookies: []*http.Cookie{
				{Name: "access_token", Value: "token-malo"},
				{Name: "refresh_token", Value: "refresh-malo"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					return nil, domain.ErrTokenInvalid
				},
				validateAndRotateFn: func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
					return nil, "", "", domain.ErrTokenExpired
				},
			},
			wantNext:         false,
			wantStatus:       http.StatusUnauthorized,
			wantClearCookies: []string{"access_token", "refresh_token"},
			wantClaims:       false,
		},
		{
			name:     "REQ-4.5 cookie_names_produccion",
			isProduction: true,
			cookies: []*http.Cookie{
				{Name: "__Secure-access_token", Value: "token-prod-valido"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					if tokenStr == "token-prod-valido" {
						return testAccessClaims, nil
					}
					return nil, domain.ErrTokenInvalid
				},
			},
			wantNext:   true,
			wantStatus: http.StatusOK,
			wantClaims: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, ck := range tt.cookies {
				req.AddCookie(ck)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
				IsProduction: tt.isProduction,
				TokenSvc:     tt.mock,
			})

			nextCalled := false
			handler := mw.Handle(func(c *echo.Context) error {
				nextCalled = true
				return c.String(http.StatusOK, "ok")
			})

			err := handler(c)
			if err != nil {
				c.Echo().HTTPErrorHandler(c, err)
			}

			// Verificar nextCalled
			if nextCalled != tt.wantNext {
				t.Errorf("nextCalled = %v, esperaba %v", nextCalled, tt.wantNext)
			}

			// Verificar código de estado
			if rec.Code != tt.wantStatus {
				t.Errorf("código de estado = %d, esperaba %d", rec.Code, tt.wantStatus)
			}

			// Verificar claims en contexto
			if tt.wantClaims {
				raw := c.Get("user_claims")
				if raw == nil {
					t.Error("esperaba claims en contexto, pero c.Get(\"user_claims\") es nil")
				}
			}

			// Verificar cookies seteadas (rotación silenciosa)
			respCookies := rec.Result().Cookies()
			for _, name := range tt.wantSetCookies {
				ck := findCookie(respCookies, name)
				if ck == nil {
					t.Errorf("esperaba cookie Set-Cookie %q, no encontrada en %v", name, cookieNames(respCookies))
					continue
				}
				if ck.Value == "" {
					t.Errorf("cookie %q seteada pero Value vacío", name)
				}
				if !ck.HttpOnly {
					t.Errorf("cookie %q debe ser HttpOnly", name)
				}
			}

			// Verificar cookies limpiadas (Max-Age=0)
			for _, name := range tt.wantClearCookies {
				ck := findCookie(respCookies, name)
				if ck == nil {
					t.Errorf("esperaba cookie limpiada %q en Set-Cookie, no encontrada en %v", name, cookieNames(respCookies))
					continue
				}
				if ck.MaxAge != 0 {
					t.Errorf("cookie %q debe tener Max-Age=0, tiene Max-Age=%d", name, ck.MaxAge)
				}
				if ck.Value != "" {
					t.Errorf("cookie %q debe tener Value vacío, tiene %q", name, ck.Value)
				}
			}
		})
	}
}

// =============================================================================
// TestOptional — AuthMiddleware.Optional()
// =============================================================================

func TestOptional(t *testing.T) {
	tests := []struct {
		name            string
		isProduction    bool
		cookies         []*http.Cookie
		mock            *mockTokenSvc
		wantNext        bool
		wantStatus      int
		wantClaims      bool
		wantNoSetCookie bool // true si NO deben aparecer Set-Cookie headers
	}{
		{
			name:     "REQ-5.1 sin_cookies",
			isProduction: false,
			cookies:  nil,
			mock:     &mockTokenSvc{},
			wantNext: true,
			wantStatus: http.StatusOK,
			wantClaims: false,
		},
		{
			name:     "REQ-5.2 token_valido",
			isProduction: false,
			cookies: []*http.Cookie{
				{Name: "access_token", Value: "token-valido"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					if tokenStr == "token-valido" {
						return testAccessClaims, nil
					}
					return nil, domain.ErrTokenInvalid
				},
			},
			wantNext:   true,
			wantStatus: http.StatusOK,
			wantClaims: true,
		},
		{
			name:     "REQ-5.3 token_invalido",
			isProduction: false,
			cookies: []*http.Cookie{
				{Name: "access_token", Value: "token-malo"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					return nil, domain.ErrTokenInvalid
				},
			},
			wantNext:        true,
			wantStatus:      http.StatusOK,
			wantClaims:      false,
			wantNoSetCookie: true,
		},
		{
			name:     "REQ-5.4 no_limpia_cookies",
			isProduction: false,
			cookies: []*http.Cookie{
				{Name: "access_token", Value: "token-malo"},
				{Name: "refresh_token", Value: "refresh-malo"},
			},
			mock: &mockTokenSvc{
				validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
					return nil, domain.ErrTokenExpired
				},
				validateAndRotateFn: func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
					return nil, "", "", domain.ErrTokenRevoked
				},
			},
			wantNext:        true,
			wantStatus:      http.StatusOK,
			wantClaims:      false,
			wantNoSetCookie: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, ck := range tt.cookies {
				req.AddCookie(ck)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
				IsProduction: tt.isProduction,
				TokenSvc:     tt.mock,
			})

			nextCalled := false
			handler := mw.Optional()(func(c *echo.Context) error {
				nextCalled = true
				return c.String(http.StatusOK, "ok")
			})

			err := handler(c)

			// Optional NUNCA debe devolver error 401
			if err != nil {
				t.Errorf("Optional() devolvió error inesperado: %v", err)
			}

			// Verificar nextCalled
			if nextCalled != tt.wantNext {
				t.Errorf("nextCalled = %v, esperaba %v", nextCalled, tt.wantNext)
			}

			// Verificar código de estado
			if rec.Code != tt.wantStatus {
				t.Errorf("código de estado = %d, esperaba %d", rec.Code, tt.wantStatus)
			}

			// Verificar claims en contexto
			if tt.wantClaims {
				raw := c.Get("user_claims")
				if raw == nil {
					t.Error("esperaba claims en contexto, pero c.Get(\"user_claims\") es nil")
				}
			}

			// Verificar que NO se limpiaron cookies (REQ-5.4)
			if tt.wantNoSetCookie {
				setCookies := rec.Header()["Set-Cookie"]
				if len(setCookies) > 0 {
					t.Errorf("Optional() no debe limpiar cookies, pero hay Set-Cookie headers: %v", setCookies)
				}
			}
		})
	}
}

// =============================================================================
// TestOptional_Production_RefreshRotation — rotación silenciosa en prod
// Verifica que Optional() también hace rotación con __Secure- cookies en prod
// =============================================================================

func TestOptional_Production_RefreshRotation(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "expired"})
	req.AddCookie(&http.Cookie{Name: "__Secure-refresh_token", Value: "refresh-prod"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	svc := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return nil, domain.ErrTokenExpired
		},
		validateAndRotateFn: func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
			return testRefreshClaims, "nuevo-access", "nuevo-refresh", nil
		},
	}

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: true,
		TokenSvc:     svc,
	})

	nextCalled := false
	handler := mw.Optional()(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("Optional() error inesperado: %v", err)
	}

	if !nextCalled {
		t.Error("esperaba nextCalled = true")
	}

	// Verificar que se setearon nuevas cookies de producción
	respCookies := rec.Result().Cookies()
	accessCk := findCookie(respCookies, "__Secure-access_token")
	if accessCk == nil {
		t.Error("esperaba __Secure-access_token en Set-Cookie tras rotación en prod")
	} else if accessCk.Value != "nuevo-access" {
		t.Errorf("access cookie value = %q, esperaba %q", accessCk.Value, "nuevo-access")
	}

	refreshCk := findCookie(respCookies, "__Secure-refresh_token")
	if refreshCk == nil {
		t.Error("esperaba __Secure-refresh_token en Set-Cookie tras rotación en prod")
	} else if refreshCk.Value != "nuevo-refresh" {
		t.Errorf("refresh cookie value = %q, esperaba %q", refreshCk.Value, "nuevo-refresh")
	}
}

// =============================================================================
// TestHandle_Produccion_SilentRotation — rotación con __Secure- cookies en prod
// =============================================================================

func TestHandle_Produccion_SilentRotation(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "expired"})
	req.AddCookie(&http.Cookie{Name: "__Secure-refresh_token", Value: "refresh-prod"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	svc := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return nil, domain.ErrTokenExpired
		},
		validateAndRotateFn: func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
			return testRefreshClaims, "nuevo-access-prod", "nuevo-refresh-prod", nil
		},
	}

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: true,
		TokenSvc:     svc,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	_ = handler(c)

	if !nextCalled {
		t.Error("esperaba nextCalled = true")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("código = %d, esperaba 200", rec.Code)
	}

	respCookies := rec.Result().Cookies()
	accessCk := findCookie(respCookies, "__Secure-access_token")
	if accessCk == nil {
		t.Error("esperaba __Secure-access_token en Set-Cookie tras rotación")
	} else {
		if accessCk.Value != "nuevo-access-prod" {
			t.Errorf("access = %q, esperaba %q", accessCk.Value, "nuevo-access-prod")
		}
		if !accessCk.Secure {
			t.Error("cookie de producción debe tener Secure=true")
		}
	}

	refreshCk := findCookie(respCookies, "__Secure-refresh_token")
	if refreshCk == nil {
		t.Error("esperaba __Secure-refresh_token en Set-Cookie tras rotación")
	} else {
		if refreshCk.Value != "nuevo-refresh-prod" {
			t.Errorf("refresh = %q, esperaba %q", refreshCk.Value, "nuevo-refresh-prod")
		}
		if !refreshCk.Secure {
			t.Error("cookie de producción debe tener Secure=true")
		}
	}
}

// =============================================================================
// TestHandle_RefreshRevocado — refresh revocado → 401 + cookies limpias
// =============================================================================

func TestHandle_RefreshRevocado(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "expired"})
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "revoked"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	svc := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return nil, domain.ErrTokenExpired
		},
		validateAndRotateFn: func(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
			return nil, "", "", domain.ErrTokenRevoked
		},
	}

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     svc,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		c.Echo().HTTPErrorHandler(c, err)
	}

	if nextCalled {
		t.Error("nextCalled = true, esperaba false (no debe llamar a next si el token está revocado)")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("código = %d, esperaba 401", rec.Code)
	}

	// Verificar cookies limpias
	respCookies := rec.Result().Cookies()
	for _, name := range []string{"access_token", "refresh_token"} {
		ck := findCookie(respCookies, name)
		if ck == nil {
			t.Errorf("esperaba cookie limpiada %q, no encontrada", name)
			continue
		}
		if ck.MaxAge != 0 {
			t.Errorf("cookie %q Max-Age=%d, esperaba 0", name, ck.MaxAge)
		}
	}
}

// =============================================================================
// Helpers de tests
// =============================================================================

// cookieNames devuelve los nombres de cookies en un slice para mensajes de error.
func cookieNames(cookies []*http.Cookie) []string {
	names := make([]string, len(cookies))
	for i, c := range cookies {
		names[i] = c.Name
	}
	return names
}

// =============================================================================
// Mock UserRepository para pruebas del pipeline de autorización
// =============================================================================

type mockUserRepo struct {
	users map[uuid.UUID]*domain.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[uuid.UUID]*domain.User)}
}

func (m *mockUserRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}

func (m *mockUserRepo) Create(_ context.Context, user *domain.User) error { return nil }
func (m *mockUserRepo) Update(_ context.Context, user *domain.User) error { return nil }
func (m *mockUserRepo) GetRoleByName(_ context.Context, name string) (*domain.Role, error) {
	return nil, domain.ErrRoleNotFound
}

// Compile-time check
var _ domain.UserRepository = (*mockUserRepo)(nil)

// =============================================================================
// userFixture crea un usuario activo con los campos mínimos.
// =============================================================================

func userFixture(id uuid.UUID, email string, tokenVersion int, status domain.UserStatus) *domain.User {
	return &domain.User{
		ID:           id,
		Email:        email,
		RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		RoleName:     "client",
		Status:       status,
		TokenVersion: tokenVersion,
	}
}

// =============================================================================
// TestHandle_ObserveMode_DisabledUser_PassesThrough — REQ: AS-SPEC-004
// En modo observe, un usuario disabled pasa el pipeline sin bloqueo.
// =============================================================================

func TestHandle_ObserveMode_DisabledUser_PassesThrough(t *testing.T) {
	// Forzar modo observe
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe")
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "token-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	mockUser := userFixture(userID, "disabled@test.com", 1, domain.StatusDisabled)
	repo := newMockUserRepo()
	repo.users[userID] = mockUser

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID:       userID,
				Email:        "disabled@test.com",
				RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:         "client",
				JTI:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				TokenVersion: 1,
			}, nil
		},
	}

	// Sin Dragonfly → el pipeline va directo a UserRepo
	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		UserRepo:     repo,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	_ = handler(c)

	if !nextCalled {
		t.Error("observe mode: se esperaba nextCalled=true, disabled user debe pasar")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("observe mode: código = %d, esperaba 200 (observar no bloquea)", rec.Code)
	}

	// Verificar que los claims están en el contexto
	raw := c.Get("user_claims")
	if raw == nil {
		t.Error("esperaba claims en contexto, pero c.Get(\"user_claims\") es nil")
	}
}

// =============================================================================
// TestHandle_EnforceMode_DisabledUser_Returns403 — REQ: AS-SPEC-004
// En modo global, un usuario disabled recibe 403 ACCOUNT_DISABLED.
// =============================================================================

func TestHandle_EnforceMode_DisabledUser_Returns403(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "global")
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "token-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	mockUser := userFixture(userID, "disabled@test.com", 1, domain.StatusDisabled)
	repo := newMockUserRepo()
	repo.users[userID] = mockUser

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID:       userID,
				Email:        "disabled@test.com",
				RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:         "client",
				JTI:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				TokenVersion: 1,
			}, nil
		},
	}

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		UserRepo:     repo,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		c.Echo().HTTPErrorHandler(c, err)
	}

	if nextCalled {
		t.Error("enforce mode: se esperaba nextCalled=false, disabled user debe ser bloqueado")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("enforce mode: código = %d, esperaba 403", rec.Code)
	}
}

// =============================================================================
// TestHandle_ObserveMode_JTIBlacklisted_PassesThrough
// En modo observe, un JTI blacklisteado pasa con log pero sin bloqueo.
// =============================================================================

func TestHandle_ObserveMode_JTIBlacklisted_PassesThrough(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe")
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	// Usar miniredis para simular Dragonfly con JTI blacklisteado
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	jti := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	// Blacklistear el JTI en Dragonfly
	jtiKey := "{auth}:blacklist:jti:" + jti.String()
	mr.Set(jtiKey, "1")

	// Poblar cache de sesión para que el pipeline tenga datos
	sessionKey := "{auth}:session:" + userID.String()
	mr.HSet(sessionKey,
		"permissions", "users:read,users:write",
		"status", "active",
		"token_version", "1",
	)

	mockUser := userFixture(userID, "test@proactrip.com", 1, domain.StatusActive)
	repo := newMockUserRepo()
	repo.users[userID] = mockUser

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID:       userID,
				Email:        "test@proactrip.com",
				RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:         "client",
				JTI:          jti,
				TokenVersion: 1,
			}, nil
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "token-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		UserRepo:     repo,
		RedisClient:  rdb,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	_ = handler(c)

	if !nextCalled {
		t.Error("observe mode: JTI blacklisteado debe pasar (observar no bloquea)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("observe mode: código = %d, esperaba 200", rec.Code)
	}
}

// =============================================================================
// TestHandle_EnforceMode_JTIBlacklisted_Returns401
// En modo global, un JTI blacklisteado recibe 401.
// =============================================================================

func TestHandle_EnforceMode_JTIBlacklisted_Returns401(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "global")
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	jti := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	jtiKey := "{auth}:blacklist:jti:" + jti.String()
	mr.Set(jtiKey, "1")

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				Email:        "test@proactrip.com",
				RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:         "client",
				JTI:          jti,
				TokenVersion: 1,
			}, nil
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "token-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		RedisClient:  rdb,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		c.Echo().HTTPErrorHandler(c, err)
	}

	if nextCalled {
		t.Error("enforce mode: JTI blacklisteado debe ser bloqueado")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("enforce mode: código = %d, esperaba 401", rec.Code)
	}
}

// =============================================================================
// TestHandle_RefreshRotation_DisabledUser_Rejected — REQ: AS-SPEC-008
// La rotación de refresh token rechaza usuarios disabled (siempre enforce).
// =============================================================================

func TestHandle_RefreshRotation_DisabledUser_Rejected(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe") // observe no debería afectar rotación
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	mockUser := userFixture(userID, "disabled@test.com", 1, domain.StatusDisabled)
	repo := newMockUserRepo()
	repo.users[userID] = mockUser

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "expired"})
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "refresh-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return nil, domain.ErrTokenExpired
		},
		validateRefreshFn: func(ctx context.Context, tokenStr string) (*token.RefreshClaims, error) {
			return &token.RefreshClaims{
				UserID:    userID,
				Email:     "disabled@test.com",
				RoleID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:      "client",
				JTI:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			}, nil
		},
	}

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		UserRepo:     repo,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		c.Echo().HTTPErrorHandler(c, err)
	}

	if nextCalled {
		t.Error("rotación con usuario disabled: se esperaba nextCalled=false")
	}
	// La rotación siempre rechaza disabled → 401 (sesión expirada, vuelve a loguear)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("código = %d, esperaba 401 (rotación rechazada para disabled)", rec.Code)
	}
}

// =============================================================================
// TestHandle_ObserveMode_SuspendedUser_PassesThrough
// =============================================================================

func TestHandle_ObserveMode_SuspendedUser_PassesThrough(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe")
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "token-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	mockUser := userFixture(userID, "suspended@test.com", 1, domain.StatusSuspended)
	repo := newMockUserRepo()
	repo.users[userID] = mockUser

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID:       userID,
				Email:        "suspended@test.com",
				RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:         "client",
				JTI:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				TokenVersion: 1,
			}, nil
		},
	}

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		UserRepo:     repo,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "ok")
	})

	_ = handler(c)

	if !nextCalled {
		t.Error("observe mode: suspended user debe pasar (observar no bloquea)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("observe mode: código = %d, esperaba 200", rec.Code)
	}
}

// =============================================================================
// TestHandle_ObserveMode_TokenVersionMismatch_PassesThrough
// En observe, token_version mismatch loguea warning pero continúa.
// =============================================================================

func TestHandle_ObserveMode_TokenVersionMismatch_PassesThrough(t *testing.T) {
	os.Setenv("AUTHZ_ENFORCE_MODE", "observe")
	defer os.Unsetenv("AUTHZ_ENFORCE_MODE")

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Usar miniredis para cache de sesión (con token_version=5)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sessionKey := "{auth}:session:" + userID.String()
	mr.HSet(sessionKey,
		"permissions", "users:read",
		"status", "active",
		"token_version", "5",
	)

	mockTokens := &mockTokenSvc{
		validateAccessFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID:       userID,
				Email:        "test@proactrip.com",
				RoleID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				Role:         "client",
				JTI:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				TokenVersion: 1, // Token tiene versión 1, cache tiene 5 → mismatch
			}, nil
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "token-ok"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := middleware.NewAuthMiddleware(middleware.AuthConfig{
		IsProduction: false,
		TokenSvc:     mockTokens,
		RedisClient:  rdb,
	})

	nextCalled := false
	handler := mw.Handle(func(c *echo.Context) error {
		nextCalled = true
		// Verificar que claims tienen TokenVersion actualizado desde cache
		raw := c.Get("user_claims")
		if raw == nil {
			t.Error("claims no encontrados en contexto")
			return c.String(http.StatusOK, "ok")
		}
		claims, ok := raw.(*token.AccessClaims)
		if !ok {
			t.Errorf("claims tiene tipo %T, esperaba *token.AccessClaims", raw)
			return c.String(http.StatusOK, "ok")
		}
		if claims.TokenVersion != 5 {
			t.Errorf("TokenVersion = %d, esperaba 5 (actualizado desde cache en observe mode)", claims.TokenVersion)
		}
		// Verificar que los permisos del cache se inyectaron
		if len(claims.Permissions) == 0 || claims.Permissions[0] != "users:read" {
			t.Errorf("Permissions = %v, esperaba [\"users:read\"]", claims.Permissions)
		}
		return c.String(http.StatusOK, "ok")
	})

	_ = handler(c)

	if !nextCalled {
		t.Error("observe mode: token_version mismatch debe pasar (observar no bloquea)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("código = %d, esperaba 200", rec.Code)
	}
}

package me

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Test setup — register domain error mappers for test isolation
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrNotAuthenticated):
			return serrors.ErrUnauthorized("se requiere autenticación", err)
		case errors.Is(err, domain.ErrTokenInvalid):
			return serrors.ErrUnauthorized("token inválido o expirado", err)
		case errors.Is(err, domain.ErrUserNotFound):
			return serrors.ErrNotFound("usuario no encontrado", err)
		}
		return nil
	})
}

// =============================================================================
// Mocks
// =============================================================================

type mockTokenSvc struct {
	validateFn func(ctx context.Context, tokenStr string) (*token.AccessClaims, error)
}

func (m *mockTokenSvc) ValidateAccessToken(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
	return m.validateFn(ctx, tokenStr)
}

type mockUserRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return m.getByIDFn(ctx, id)
}

// =============================================================================
// Tests
// =============================================================================

func TestHandler_Handle_NoCookie(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := NewHandler(&mockTokenSvc{}, &mockUserRepo{})
	_ = h.Handle(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestHandler_Handle_InvalidToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "bad-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	tokenSvc := &mockTokenSvc{
		validateFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return nil, domain.ErrTokenInvalid
		},
	}

	h := NewHandler(tokenSvc, &mockUserRepo{})
	_ = h.Handle(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestHandler_Handle_Success(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "valid-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	userID := uuid.Must(uuid.NewV7())

	tokenSvc := &mockTokenSvc{
		validateFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{
				UserID: userID,
			}, nil
		},
	}

	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return &domain.User{
				ID:            userID,
				Email:         "user@test.com",
				EmailVerified: true,
				RoleName:      "client",
			}, nil
		},
	}

	h := NewHandler(tokenSvc, userRepo)
	err := h.Handle(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !contains(body, `"id"`) {
		t.Errorf("expected response to contain user id, got: %s", body)
	}
	if !contains(body, `"email":"user@test.com"`) {
		t.Errorf("expected response to contain email, got: %s", body)
	}
}

func TestHandler_Handle_DevFallbackCookie(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "dev-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	userID := uuid.Must(uuid.NewV7())

	tokenSvc := &mockTokenSvc{
		validateFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			if tokenStr != "dev-token" {
				return nil, domain.ErrTokenInvalid
			}
			return &token.AccessClaims{UserID: userID}, nil
		},
	}

	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return &domain.User{
				ID:            userID,
				Email:         "dev@test.com",
				EmailVerified: false,
				RoleName:      "client",
			}, nil
		},
	}

	h := NewHandler(tokenSvc, userRepo)
	err := h.Handle(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestHandler_Handle_UserNotFound(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "__Secure-access_token", Value: "valid-token"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	userID := uuid.Must(uuid.NewV7())

	tokenSvc := &mockTokenSvc{
		validateFn: func(ctx context.Context, tokenStr string) (*token.AccessClaims, error) {
			return &token.AccessClaims{UserID: userID}, nil
		},
	}

	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
	}

	h := NewHandler(tokenSvc, userRepo)
	_ = h.Handle(c)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RED phase — handler tests for user_detail.
// Tests HTTP-level behavior: path param extraction, response format, error handling.
package user_detail_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	userdetail "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/user_detail"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Domain error mappers — registrados en init() para aislamiento de tests
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrUserNotFound):
			return serrors.ErrNotFound("Usuario no encontrado", err)
		case errors.Is(err, domain.ErrInvalidInput),
			errors.Is(err, domain.ErrValidationError):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)
		}
		return nil
	})
}

// =============================================================================
// Fixtures
// =============================================================================

func newHandler(
	getByID func(ctx context.Context, id uuid.UUID) (*domain.User, error),
	resolveFn func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error),
) *userdetail.Handler {
	repo := &stubUserRepo{getByID: getByID}
	resolver := &stubPermissionResolver{resolveFn: resolveFn}
	uc := userdetail.NewUseCase(repo, resolver)
	return userdetail.NewHandler(uc)
}

// =============================================================================
// Tests
// =============================================================================

// TestHandler_UserDetail_Success verifies 200 with correct JSON response.
func TestHandler_UserDetail_Success(t *testing.T) {
	e := echo.New()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	h := newHandler(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			u := &domain.User{
				ID:            userID,
				Email:         "detail@test.com",
				EmailVerified: true,
				RoleID:        roleID,
				RoleName:      "client",
				Status:        domain.StatusActive,
			}
			return u, nil
		},
		func(ctx context.Context, uid, rid uuid.UUID) ([]string, error) {
			return []string{"users:read"}, nil
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users/"+userID.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/dashboard/users/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: userID.String()}})

	err := h.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !contains(body, `"effective_permissions"`) {
		t.Errorf("response missing 'effective_permissions' key: %s", body)
	}
	if !contains(body, `"email":"detail@test.com"`) {
		t.Errorf("response missing user email: %s", body)
	}
	if !contains(body, `"users:read"`) {
		t.Errorf("response missing permission 'users:read': %s", body)
	}
}

// TestHandler_UserDetail_NotFound returns 404.
func TestHandler_UserDetail_NotFound(t *testing.T) {
	e := echo.New()

	h := newHandler(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users/"+uuid.Must(uuid.NewV7()).String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/dashboard/users/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: uuid.Must(uuid.NewV7()).String()}})

	_ = h.Handle(c)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandler_UserDetail_InvalidUUID returns 400.
func TestHandler_UserDetail_InvalidUUID(t *testing.T) {
	e := echo.New()

	h := newHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users/invalid-uuid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/dashboard/users/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "invalid-uuid"}})

	_ = h.Handle(c)

	if rec.Code < 400 {
		t.Errorf("status = %d, expected >= 400 for invalid UUID", rec.Code)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

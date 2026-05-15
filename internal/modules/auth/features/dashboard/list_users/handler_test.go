// RED phase — handler tests for list_users.
// Tests HTTP-level behavior: query param parsing, response format, error handling.
package list_users_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	listusers "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/list_users"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Domain error mappers — registrados en init() para aislamiento de tests
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrInvalidInput),
			errors.Is(err, domain.ErrValidationError):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)
		}
		return nil
	})
}

// =============================================================================
// Fixtures para handler tests
// =============================================================================

func newHandler(listFn func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error)) *listusers.Handler {
	repo := &stubUserListRepo{listFn: listFn}
	uc := listusers.NewUseCase(repo)
	return listusers.NewHandler(uc)
}

// =============================================================================
// Tests
// =============================================================================

// TestHandler_ListUsers_Success verifies 200 with correct JSON response.
func TestHandler_ListUsers_Success(t *testing.T) {
	e := echo.New()
	roleID := uuid.Must(uuid.NewV7())
	id := uuid.Must(uuid.NewV7())

	h := newHandler(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return []listusers.UserRow{
			{ID: id, Email: "test@test.com", Status: string(domain.StatusActive), RoleID: roleID, RoleName: "client", EmailVerified: true},
		}, 1, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users?limit=20", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, expected %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if body == "" {
		t.Fatal("response body should not be empty")
	}
	// Verify key fields are in the JSON response
	if !contains(body, `"users"`) {
		t.Errorf("response missing 'users' key: %s", body)
	}
	if !contains(body, `"meta"`) {
		t.Errorf("response missing 'meta' key: %s", body)
	}
	if !contains(body, `"email":"test@test.com"`) {
		t.Errorf("response missing user email: %s", body)
	}
}

// TestHandler_ListUsers_InvalidLimit returns 400.
func TestHandler_ListUsers_InvalidLimit(t *testing.T) {
	e := echo.New()
	h := newHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users?limit=101", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.Handle(c)

	if rec.Code < 400 {
		t.Errorf("status = %d, expected >= 400 for limit > 100", rec.Code)
	}
}

// TestHandler_ListUsers_RepoError returns 500.
func TestHandler_ListUsers_RepoError(t *testing.T) {
	e := echo.New()
	h := newHandler(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return nil, 0, errors.New("db down")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.Handle(c)

	if rec.Code < 500 {
		t.Errorf("status = %d, expected >= 500 for repo error", rec.Code)
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

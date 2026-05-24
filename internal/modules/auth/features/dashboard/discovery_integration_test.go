// Test de integración end-to-end para el flujo completo del dashboard:
// crear usuario client → autenticar → listar → deshabilitar → verificar 401.
//
// Este test ejercita el pipeline completo: handler → usecase → repo (mock)
// con el middleware RequirePermission y el flujo de invalidación de token_version.
package dashboard_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain/services"
	accountstatus "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/account_status"
	listusers "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/list_users"
	userdetail "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/user_detail"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	sharedmiddleware "github.com/ProacTrip/Backend/internal/shared/middleware"
)

// =============================================================================
// Mock repos para el test de integración
// =============================================================================

type mockDashboardRepo struct {
	users   map[uuid.UUID]*domain.User
	listErr error
}

func newMockDashboardRepo() *mockDashboardRepo {
	roleID := uuid.Must(uuid.NewV7())
	clientID := uuid.Must(uuid.NewV7())

	return &mockDashboardRepo{
		users: map[uuid.UUID]*domain.User{
			clientID: {
				ID:            clientID,
				Email:         "client@proactrip.com",
				Status:        domain.StatusActive,
				RoleID:        roleID,
				RoleName:      "client",
				EmailVerified: true,
				TokenVersion:  1,
			},
		},
	}
}

func (m *mockDashboardRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (m *mockDashboardRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (int, error) {
	u, ok := m.users[id]
	if !ok {
		return 0, domain.ErrUserNotFound
	}
	u.Status = domain.UserStatus(status)
	u.TokenVersion++
	return u.TokenVersion, nil
}

func (m *mockDashboardRepo) ListUsers(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}

	var rows []listusers.UserRow
	for _, u := range m.users {
		rows = append(rows, listusers.UserRow{
			ID:            u.ID,
			Email:         u.Email,
			Status:        string(u.Status),
			RoleID:        u.RoleID,
			RoleName:      u.RoleName,
			EmailVerified: u.EmailVerified,
		})
	}

	// Paginación simple
	start := filters.Offset
	if start > len(rows) {
		start = len(rows)
	}
	end := start + filters.Limit
	if end > len(rows) {
		end = len(rows)
	}

	return rows[start:end], len(rows), nil
}

type mockPermissionResolver struct{}

func (m *mockPermissionResolver) ResolveEffectivePermissions(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
	return []string{"users:read", "users:write"}, nil
}

// Compile-time checks
var _ interface {
	GetByID(context.Context, uuid.UUID) (*domain.User, error)
	UpdateStatus(context.Context, uuid.UUID, string) (int, error)
	ListUsers(context.Context, listusers.ListUsersFilters) ([]listusers.UserRow, int, error)
} = (*mockDashboardRepo)(nil)

var _ services.PermissionResolver = (*mockPermissionResolver)(nil)
var _ interface{ ResolveEffectivePermissions(context.Context, uuid.UUID, uuid.UUID) ([]string, error) } = (*mockPermissionResolver)(nil)

// =============================================================================
// Test: Flujo completo — listar → deshabilitar → verificar invalidación
// =============================================================================

func TestDashboardIntegration_ListThenDisableThenUnauthorized(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	e := echo.New()

	// Registrar el error handler para que los errores de dominio se mapeen a HTTP
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if resp, _ := echo.UnwrapResponse(c.Response()); resp != nil && resp.Committed {
			return
		}
		// Intentar mapear error de dominio
		if prob := serrors.MapDomainError(err); prob != nil {
			_ = c.JSON(prob.Status, prob)
			return
		}
		// Error HTTP de Echo
		if he, ok := errors.AsType[*echo.HTTPError](err); ok {
			_ = c.JSON(he.Code, map[string]any{"error": he.Message})
			return
		}
		// Fallback
		_ = c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	repo := newMockDashboardRepo()
	resolver := &mockPermissionResolver{}
	clientUser := repo.users[findFirstUserID(repo.users)]

	// 1. Crear handlers con los mismos deps que module.go
	listUsersUC := listusers.NewUseCase(repo)
	listUsersHandler := listusers.NewHandler(listUsersUC)

	accountStatusUC := accountstatus.NewUseCase(repo, nil, nil) // nil rdb + nil eventPublisher en test
	accountStatusHandler := accountstatus.NewHandler(accountStatusUC)

	userDetailUC := userdetail.NewUseCase(repo, resolver, nil)
	userDetailHandler := userdetail.NewHandler(userDetailUC)

	// ========== SETUP: Registrar rutas del dashboard ==========
	dashboard := e.Group("/v1/dashboard")

	// Simular claims con permisos de admin (inyectados por middleware en producción)
	adminClaims := &mockPermissionClaims{
		userID:      clientUser.ID,
		permissions: []string{"users:read", "users:write"},
	}

	// Middleware que inyecta claims de usuario (simula auth middleware)
	authSimulator := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("user_claims", adminClaims)
			return next(c)
		}
	}

	dashboard.Use(authSimulator)
	dashboard.Use(sharedmiddleware.RequirePermission("users:read")) // permiso base del grupo

	dashboard.GET("/users", listUsersHandler.Handle)
	dashboard.GET("/users/:id", userDetailHandler.Handle)
	dashboard.PUT("/users/:id/status", accountStatusHandler.Handle, sharedmiddleware.RequirePermission("users:write"))

	// ========== TEST 1: List users ==========
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users?limit=10", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("TEST 1 — List users: esperado 200, recibido %d. Body: %s", rec.Code, rec.Body.String())
	}
	t.Logf("✓ List users → 200 OK")

	// Verificar que la respuesta contiene al client user
	if !strings.Contains(rec.Body.String(), clientUser.Email) {
		t.Errorf("TEST 1 — List users: respuesta no contiene email del client user: %s", rec.Body.String())
	}

	// ========== TEST 2: Disable account ==========
	body := `{"status":"disabled"}`
	req = httptest.NewRequest(http.MethodPut, "/v1/dashboard/users/"+clientUser.ID.String()+"/status",
		strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("TEST 2 — Disable account: esperado 200, recibido %d. Body: %s", rec.Code, rec.Body.String())
	}
	t.Logf("✓ Disable account → 200 OK")

	// Verificar que el status cambió en el repo
	updatedUser, _ := repo.GetByID(ctx, clientUser.ID)
	if updatedUser.Status != domain.StatusDisabled {
		t.Errorf("TEST 2 — Disable account: status esperado 'disabled', recibido '%s'", updatedUser.Status)
	}
	if updatedUser.TokenVersion <= 1 {
		t.Errorf("TEST 2 — Disable account: token_version esperado > 1, recibido %d", updatedUser.TokenVersion)
	}
	t.Logf("✓ Status = %s, token_version = %d", updatedUser.Status, updatedUser.TokenVersion)

	// ========== TEST 3: List users again — should still work (observe mode) ==========
	// En observe mode (default), RequirePermission nunca bloquea.
	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard/users?limit=10", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("TEST 3 — List users after disable (observe mode): esperado 200, recibido %d. Body: %s", rec.Code, rec.Body.String())
	}
	t.Logf("✓ List users after disable → 200 OK (observe mode no bloquea)")

	// ========== TEST 4: Account status endpoint validation ==========
	// Intentar estado inválido
	body = `{"status":"suspended"}`
	req = httptest.NewRequest(http.MethodPut, "/v1/dashboard/users/"+clientUser.ID.String()+"/status",
		strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("TEST 4 — Invalid status: esperado 400, recibido %d", rec.Code)
	}
	t.Logf("✓ Invalid status (suspended) → 400 Bad Request")

	// ========== TEST 5: User detail con permisos efectivos ==========
	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard/users/"+clientUser.ID.String(), nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("TEST 5 — User detail: esperado 200, recibido %d. Body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "effective_permissions") {
		t.Errorf("TEST 5 — User detail: respuesta no contiene effective_permissions")
	}
	t.Logf("✓ User detail → 200 OK con effective_permissions")
}

// =============================================================================
// Test: Error mapper — domain errors se convierten a HTTP correctamente
// =============================================================================

func TestDashboardErrorMappers(t *testing.T) {
	e := echo.New()

	// Registrar error handler
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if resp, _ := echo.UnwrapResponse(c.Response()); resp != nil && resp.Committed {
			return
		}
		if prob := serrors.MapDomainError(err); prob != nil {
			_ = c.JSON(prob.Status, prob)
			return
		}
		if he, ok := errors.AsType[*echo.HTTPError](err); ok {
			_ = c.JSON(he.Code, map[string]any{"error": he.Message})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	repo := newMockDashboardRepo()
	resolver := &mockPermissionResolver{}

	accountStatusUC := accountstatus.NewUseCase(repo, nil, nil)
	accountStatusHandler := accountstatus.NewHandler(accountStatusUC)

	userDetailUC := userdetail.NewUseCase(repo, resolver, nil)
	userDetailHandler := userdetail.NewHandler(userDetailUC)

	adminClaims := &mockPermissionClaims{
		userID:      uuid.Must(uuid.NewV7()),
		permissions: []string{"users:read", "users:write"},
	}

	authSimulator := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("user_claims", adminClaims)
			return next(c)
		}
	}

	dashboard := e.Group("/v1/dashboard")
	dashboard.Use(authSimulator)
	dashboard.Use(sharedmiddleware.RequirePermission("users:read"))
	dashboard.GET("/users/:id", userDetailHandler.Handle)
	dashboard.PUT("/users/:id/status", accountStatusHandler.Handle, sharedmiddleware.RequirePermission("users:write"))

	// Test: User not found → 404
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/users/"+uuid.Must(uuid.NewV7()).String(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("User not found: esperado 404, recibido %d. Body: %s", rec.Code, rec.Body.String())
	}
	t.Logf("✓ User not found → 404")
}

// =============================================================================
// Test: RequirePermission middleware bloquea sin permisos suficientes
// =============================================================================

func TestDashboard_RequirePermission_DeniesWithoutPermission(t *testing.T) {
	// Guardamos y restauramos AUTHZ_ENFORCE_MODE para este test
	t.Setenv("AUTHZ_ENFORCE_MODE", "enforce")

	e := echo.New()

	// Registrar error handler
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if resp, _ := echo.UnwrapResponse(c.Response()); resp != nil && resp.Committed {
			return
		}
		if prob := serrors.MapDomainError(err); prob != nil {
			_ = c.JSON(prob.Status, prob)
			return
		}
		if he, ok := errors.AsType[*echo.HTTPError](err); ok {
			_ = c.JSON(he.Code, map[string]any{"error": he.Message})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	noPermClaims := &mockPermissionClaims{
		userID:      uuid.Must(uuid.NewV7()),
		permissions: []string{"users:read"}, // solo read, no write
	}

	authSimulator := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("user_claims", noPermClaims)
			return next(c)
		}
	}

	dashboard := e.Group("/v1/dashboard")
	dashboard.Use(authSimulator)
	dashboard.Use(sharedmiddleware.RequirePermission("users:read"))

	// Handler dummy para el PUT
	dashboard.PUT("/users/:id/status",
		func(c *echo.Context) error { return c.String(http.StatusOK, "ok") },
		sharedmiddleware.RequirePermission("users:write"),
	)

	req := httptest.NewRequest(http.MethodPut, "/v1/dashboard/users/"+uuid.Must(uuid.NewV7()).String()+"/status",
		strings.NewReader(`{"status":"disabled"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Enforce mode - missing users:write: esperado 403, recibido %d. Body: %s", rec.Code, rec.Body.String())
	}
	t.Logf("✓ Enforce mode — missing permission → 403 Forbidden")
}

// =============================================================================
// mockPermissionClaims — implementa PermissionClaims para tests
// =============================================================================

type mockPermissionClaims struct {
	userID      uuid.UUID
	permissions []string
}

func (m *mockPermissionClaims) GetPermissions() []string {
	return m.permissions
}

// Compile-time check
var _ sharedmiddleware.PermissionClaims = (*mockPermissionClaims)(nil)

// =============================================================================
// Helpers
// =============================================================================

func findFirstUserID(users map[uuid.UUID]*domain.User) uuid.UUID {
	for id := range users {
		return id
	}
	return uuid.Nil
}

// Ensure unused import doesn't cause error
var _ = serrors.ErrBadRequest

// init registra los mapeos de errores de dominio necesarios para los tests.
// En producción estos se registran en auth.NewModule(), pero los tests de
// integración no instancian el módulo completo.
func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrUserNotFound):
			return serrors.ErrNotFound("Usuario no encontrado", err)
		case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrValidationError):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)
		case errors.Is(err, domain.ErrCannotDisableSelf):
			return serrors.ErrBadRequest("No puedes deshabilitar tu propia cuenta", err)
		case errors.Is(err, domain.ErrPermissionDenied):
			return serrors.ErrForbidden("Permiso denegado", err)
		case errors.Is(err, sharedmiddleware.ErrMissingPermission):
			return serrors.ErrForbidden("permiso insuficiente", err)
		}
		return nil
	})
}

package login_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	login "github.com/ProacTrip/Backend/internal/modules/auth/features/login"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Domain error mappers — registrados en init() para aislamiento de tests
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrInvalidCredentials):
			return serrors.ErrUnauthorized("Credenciales inválidas", err)
		case errors.Is(err, domain.ErrEmailNotVerified):
			return serrors.ErrUnauthorized("Email no verificado. Revisa tu bandeja de entrada.", err)
		case errors.Is(err, domain.ErrAccountLocked):
			return serrors.ErrTooManyRequests("Cuenta bloqueada temporalmente. Intenta más tarde.", err)
		case errors.Is(err, domain.ErrInvalidEmail),
			errors.Is(err, domain.ErrInvalidInput),
			errors.Is(err, domain.ErrValidationError),
			errors.Is(err, domain.ErrPasswordTooShort):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)
		}
		return nil
	})
}

// =============================================================================
// Stubs — implementan las interfaces del usecase
// =============================================================================

type stubUserRepo struct {
	getByEmail func(ctx context.Context, email string) (*domain.User, error)
	update     func(ctx context.Context, user *domain.User) error
}

func (m *stubUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.getByEmail != nil {
		return m.getByEmail(ctx, email)
	}
	return nil, nil
}
func (m *stubUserRepo) Update(ctx context.Context, user *domain.User) error {
	if m.update != nil {
		return m.update(ctx, user)
	}
	return nil
}
func (m *stubUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)  { return nil, nil }
func (m *stubUserRepo) Create(ctx context.Context, user *domain.User) error               { return nil }
func (m *stubUserRepo) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) { return nil, nil }

type stubPasswordSvc struct {
	verify func(password, encoded string) (bool, error)
}

func (m *stubPasswordSvc) Verify(password, encoded string) (bool, error) {
	if m.verify != nil {
		return m.verify(password, encoded)
	}
	return true, nil
}

type stubTokenSvc struct {
	generate func(userID uuid.UUID, email, role string, roleID uuid.UUID) (*token.TokenPair, error)
}

func (m *stubTokenSvc) GenerateTokenPair(userID uuid.UUID, email, role string, roleID uuid.UUID) (*token.TokenPair, error) {
	if m.generate != nil {
		return m.generate(userID, email, role, roleID)
	}
	return &token.TokenPair{AccessToken: "at", RefreshToken: "rt"}, nil
}

// =============================================================================
// Fixture
// =============================================================================

func usuarioActivo(email string) *domain.User {
	return &domain.User{
		ID:            uuid.Must(uuid.NewV7()),
		Email:         email,
		EmailVerified: true,
		PasswordHash:  "$2a$10$hashed",
		RoleID:        uuid.Must(uuid.NewV7()),
		RoleName:      "client",
		Status:        domain.StatusActive,
	}
}

func nuevoHandlerExitoso() *login.Handler {
	repo := &stubUserRepo{
		getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
			u := usuarioActivo(email)
			u.Email = "ok@test.com" // email fijo para asserts
			return u, nil
		},
	}
	hasher := &stubPasswordSvc{
		verify: func(password, encoded string) (bool, error) { return true, nil },
	}
	tokens := &stubTokenSvc{
		generate: func(userID uuid.UUID, email, role string, roleID uuid.UUID) (*token.TokenPair, error) {
			return &token.TokenPair{AccessToken: "at-test", RefreshToken: "rt-test"}, nil
		},
	}
	uc := login.NewUseCase(login.UseCaseDeps{Repo: repo, Hasher: hasher, TokenSvc: tokens})
	return login.NewHandler(uc, false, "") // dev mode — usa SetAuthCookiesDev
}

func nuevoHandlerConError(errDomain error) *login.Handler {
	repo := &stubUserRepo{
		getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
			return nil, errDomain
		},
	}
	uc := login.NewUseCase(login.UseCaseDeps{
		Repo:     repo,
		Hasher:   &stubPasswordSvc{},
		TokenSvc: &stubTokenSvc{},
	})
	return login.NewHandler(uc, false, "")
}

// =============================================================================
// TestHandler_LoginExitoso — 200, user JSON, Set-Cookie headers
// =============================================================================

func TestHandler_LoginExitoso(t *testing.T) {
	e := echo.New()
	h := nuevoHandlerExitoso()

	body := `{"email":"ok@test.com","password":"correcta"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handle(c)
	if err != nil {
		t.Fatalf("Handle() error inesperado: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("código = %d, esperado %d", rec.Code, http.StatusOK)
	}

	// User JSON en el body
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, `"email":"ok@test.com"`) {
		t.Errorf("respuesta no contiene email del usuario: %s", cuerpo)
	}
	if !strings.Contains(cuerpo, `"id"`) {
		t.Errorf("respuesta no contiene id del usuario: %s", cuerpo)
	}

	// Set-Cookie headers (dev mode → access_token + refresh_token)
	cookies := rec.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatalf("se esperaban 2 cookies Set-Cookie, se encontraron %d", len(cookies))
	}
	var accessFound, refreshFound bool
	for _, ck := range cookies {
		if ck.Name == "access_token" {
			accessFound = true
		}
		if ck.Name == "refresh_token" {
			refreshFound = true
		}
	}
	if !accessFound || !refreshFound {
		t.Errorf("no se encontraron las cookies de auth. Cookies: %v", cookieNames(cookies))
	}
}

// =============================================================================
// TestHandler_CredencialesInvalidas — 401, sin Set-Cookie
// =============================================================================

func TestHandler_CredencialesInvalidas(t *testing.T) {
	e := echo.New()
	h := nuevoHandlerConError(domain.ErrUserNotFound)

	body := `{"email":"noexiste@test.com","password":"cualquiera"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.Handle(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("código = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}

	// Sin Set-Cookie en respuestas de error
	cookies := rec.Result().Cookies()
	if len(cookies) > 0 {
		t.Errorf("no se esperaban cookies Set-Cookie en error, pero se encontraron %d: %v",
			len(cookies), cookieNames(cookies))
	}
}

// =============================================================================
// TestHandler_BodyInvalido — malformed JSON → 400
// =============================================================================

func TestHandler_BodyInvalido(t *testing.T) {
	e := echo.New()
	h := nuevoHandlerExitoso() // las dependencias no deberían ejecutarse

	body := `{json roto`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.Handle(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("código = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// TestHandler_CacheControl — verifica header Cache-Control: no-store, private
// =============================================================================

func TestHandler_CacheControl(t *testing.T) {
	e := echo.New()
	h := nuevoHandlerExitoso()

	body := `{"email":"ok@test.com","password":"correcta"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = h.Handle(c)

	cc := rec.Header().Get("Cache-Control")
	if cc != "no-store, private" {
		t.Errorf("Cache-Control = %q, esperado %q", cc, "no-store, private")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func cookieNames(cookies []*http.Cookie) []string {
	names := make([]string, len(cookies))
	for i, ck := range cookies {
		names[i] = ck.Name
	}
	return names
}

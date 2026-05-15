package login_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/login"
)

// =============================================================================
// Stubs — inline mocks stdlib-only
// =============================================================================

type useCaseUserRepo struct {
	getByEmail  func(ctx context.Context, email string) (*domain.User, error)
	update      func(ctx context.Context, user *domain.User) error
	lastUpdated *domain.User
}

func (m *useCaseUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.getByEmail != nil {
		return m.getByEmail(ctx, email)
	}
	return nil, nil
}

func (m *useCaseUserRepo) Update(ctx context.Context, user *domain.User) error {
	m.lastUpdated = user
	if m.update != nil {
		return m.update(ctx, user)
	}
	return nil
}

func (m *useCaseUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)   { return nil, nil }
func (m *useCaseUserRepo) Create(ctx context.Context, user *domain.User) error                { return nil }
func (m *useCaseUserRepo) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) { return nil, nil }

type useCasePasswordSvc struct {
	verify func(password, encoded string) (bool, error)
}

func (m *useCasePasswordSvc) Verify(password, encoded string) (bool, error) {
	if m.verify != nil {
		return m.verify(password, encoded)
	}
	return true, nil
}

type useCaseTokenSvc struct {
	generate func(userID uuid.UUID, email, role string, roleID, sessionID uuid.UUID) (*token.TokenPair, error)
}

func (m *useCaseTokenSvc) GenerateTokenPair(userID uuid.UUID, email, role string, roleID, sessionID uuid.UUID) (*token.TokenPair, error) {
	if m.generate != nil {
		return m.generate(userID, email, role, roleID, sessionID)
	}
	return &token.TokenPair{AccessToken: "at", RefreshToken: "rt"}, nil
}

// Compile-time interface satisfaction
var (
	_ domain.UserRepository = (*useCaseUserRepo)(nil)
	_ login.PasswordService = (*useCasePasswordSvc)(nil)
	_ login.TokenService    = (*useCaseTokenSvc)(nil)
)

// =============================================================================
// Fixture
// =============================================================================

func useCaseUsuarioActivo(email string) *domain.User {
	return &domain.User{
		ID:            uuid.Must(uuid.NewV7()),
		Email:         email,
		EmailVerified: true,
		PasswordHash:  "$2a$10$hashedpassword",
		RoleID:        uuid.Must(uuid.NewV7()),
		RoleName:      "client",
		Status:        domain.StatusActive,
	}
}

// =============================================================================
// TestUseCase_Execute — 7 escenarios table-driven
// =============================================================================

func TestUseCase_Execute(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		nombre      string
		comando     login.Command
		repo        *useCaseUserRepo
		hasher      *useCasePasswordSvc
		tokens      *useCaseTokenSvc
		quiereErr   bool
		errEs       error
		errNoEs     error
		errContiene string
		verificar   func(t *testing.T, repo *useCaseUserRepo, resp *login.Response)
	}{
		{
			nombre:  "credenciales_validas",
			comando: login.Command{Email: "ok@test.com", Password: "correcta"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					return useCaseUsuarioActivo("ok@test.com"), nil
				},
			},
			hasher: &useCasePasswordSvc{
				verify: func(password, encoded string) (bool, error) { return true, nil },
			},
			tokens: &useCaseTokenSvc{
				generate: func(userID uuid.UUID, email, role string, roleID, sessionID uuid.UUID) (*token.TokenPair, error) {
					return &token.TokenPair{AccessToken: "at-abc", RefreshToken: "rt-xyz"}, nil
				},
			},
			quiereErr: false,
			verificar: func(t *testing.T, repo *useCaseUserRepo, resp *login.Response) {
				if repo.lastUpdated == nil {
					t.Fatal("Update no fue llamado — RecordLogin debería disparar Update")
				}
				if repo.lastUpdated.LoginCount != 1 {
					t.Errorf("LoginCount = %d, esperado 1 — RecordLogin debe ejecutarse ANTES de Update",
						repo.lastUpdated.LoginCount)
				}
				if repo.lastUpdated.FailedLoginAttempts != 0 {
					t.Errorf("FailedLoginAttempts = %d, esperado 0 — RecordLogin resetea intentos fallidos",
						repo.lastUpdated.FailedLoginAttempts)
				}
				if resp.AccessToken != "at-abc" || resp.RefreshToken != "rt-xyz" {
					t.Error("token pair no coincide con el generado por TokenService")
				}
				if resp.User == nil || resp.User.Email != "ok@test.com" {
					t.Error("UserResponse inválido o email incorrecto")
				}
			},
		},
		{
			nombre:  "usuario_no_encontrado",
			comando: login.Command{Email: "noexiste@test.com", Password: "cualquiera"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					return nil, domain.ErrUserNotFound
				},
			},
			hasher:    &useCasePasswordSvc{},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrInvalidCredentials,
		},
		{
			nombre:  "email_no_verificado",
			comando: login.Command{Email: "sinverificar@test.com", Password: "password123"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					u := useCaseUsuarioActivo(email)
					u.EmailVerified = false
					return u, nil
				},
			},
			hasher:    &useCasePasswordSvc{},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrEmailNotVerified,
		},
		{
			nombre:  "cuenta_bloqueada",
			comando: login.Command{Email: "locked@test.com", Password: "password123"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					u := useCaseUsuarioActivo(email)
					u.Status = domain.StatusLocked
					u.LockedUntil = new(time.Now().Add(10 * time.Minute))
					return u, nil
				},
			},
			hasher:    &useCasePasswordSvc{},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrAccountLocked,
		},
		{
			nombre:  "contraseña_incorrecta",
			comando: login.Command{Email: "test@test.com", Password: "mala"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					return useCaseUsuarioActivo(email), nil
				},
			},
			hasher: &useCasePasswordSvc{
				verify: func(password, encoded string) (bool, error) { return false, nil },
			},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrInvalidCredentials,
			verificar: func(t *testing.T, repo *useCaseUserRepo, resp *login.Response) {
				if repo.lastUpdated == nil {
					t.Fatal("Update no fue llamado — RecordFailedLogin debería disparar Update")
				}
				if repo.lastUpdated.FailedLoginAttempts != 1 {
					t.Errorf("FailedLoginAttempts = %d, esperado 1 — RecordFailedLogin debe registrar el intento",
						repo.lastUpdated.FailedLoginAttempts)
				}
			},
		},
		{
			nombre:  "fallo_generacion_tokens",
			comando: login.Command{Email: "tokenerr@test.com", Password: "correcta"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					return useCaseUsuarioActivo(email), nil
				},
			},
			hasher: &useCasePasswordSvc{
				verify: func(password, encoded string) (bool, error) { return true, nil },
			},
			tokens: &useCaseTokenSvc{
				generate: func(userID uuid.UUID, email, role string, roleID, sessionID uuid.UUID) (*token.TokenPair, error) {
					return nil, errors.New("dragonfly connection refused")
				},
			},
			quiereErr:   true,
			errNoEs:     domain.ErrTokenInvalid,
			errContiene: "generar tokens de sesión",
		},
		{
			nombre:  "error_infraestructura",
			comando: login.Command{Email: "dbdown@test.com", Password: "password123"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					return nil, errors.New("connection refused")
				},
			},
			hasher:      &useCasePasswordSvc{},
			tokens:      &useCaseTokenSvc{},
			quiereErr:   true,
			errNoEs:     domain.ErrInvalidCredentials,
			errContiene: "get user by email",
		},
		{
			nombre:  "cuenta_deshabilitada",
			comando: login.Command{Email: "disabled@test.com", Password: "password123"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					u := useCaseUsuarioActivo(email)
					u.Status = domain.StatusDisabled
					return u, nil
				},
			},
			hasher:    &useCasePasswordSvc{},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrAccountDisabled,
		},
		{
			nombre:  "cuenta_suspendida",
			comando: login.Command{Email: "suspended@test.com", Password: "password123"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					u := useCaseUsuarioActivo(email)
					u.Status = domain.StatusSuspended
					return u, nil
				},
			},
			hasher:    &useCasePasswordSvc{},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrAccountSuspended,
		},
		{
			nombre:  "cuenta_pendiente_verificacion",
			comando: login.Command{Email: "pending@test.com", Password: "password123"},
			repo: &useCaseUserRepo{
				getByEmail: func(ctx context.Context, email string) (*domain.User, error) {
					u := useCaseUsuarioActivo(email)
					u.Status = domain.StatusPendingVerification
					u.EmailVerified = false // Consistencia con el estado
					return u, nil
				},
			},
			hasher:    &useCasePasswordSvc{},
			tokens:    &useCaseTokenSvc{},
			quiereErr: true,
			errEs:     domain.ErrEmailNotVerified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			uc := login.NewUseCase(login.UseCaseDeps{
				Repo:     tt.repo,
				Hasher:   tt.hasher,
				TokenSvc: tt.tokens,
			})

			resp, err := uc.Execute(ctx, tt.comando)

			if tt.quiereErr {
				if err == nil {
					t.Fatal("se esperaba error, pero Execute retornó nil")
				}
				if tt.errEs != nil && !errors.Is(err, tt.errEs) {
					t.Errorf("errors.Is(err, %v) = false\nerror obtenido: %v", tt.errEs, err)
				}
				if tt.errNoEs != nil && errors.Is(err, tt.errNoEs) {
					t.Errorf("errors.Is(err, %v) = true, pero NO debería coincidir", tt.errNoEs)
				}
				if tt.errContiene != "" && !strings.Contains(err.Error(), tt.errContiene) {
					t.Errorf("error no contiene %q:\n%v", tt.errContiene, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Execute() error inesperado: %v", err)
				}
				if resp == nil {
					t.Fatal("Execute() retornó respuesta nil")
				}
			}

			if tt.verificar != nil {
				tt.verificar(t, tt.repo, resp)
			}
		})
	}
}

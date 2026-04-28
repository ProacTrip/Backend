package auth

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/password"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/login"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/logout"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/register"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/verify_email"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/redis/go-redis/v9"
)

// Punto de entrada del módulo Auth.
// Inicializa dependencias y mapea errores de dominio a respuestas HTTP RFC 7807.

type Module struct {
	// Repository
	Repository domain.UserRepository

	// Adapters (implementaciones concretas)
	PasswordHasher      *password.Hasher
	TokenService        *token.PasetoService
	VerificationService *verification.VerificationService

	// Event Bus (for event-driven architecture)
	EventPublisher *eventbus.EventBus

	// Features
	Register           *register.UseCase
	RegisterHandler    func() *register.Handler
	VerifyEmail        *verify_email.UseCase
	VerifyEmailHandler func() *verify_email.Handler
	Login              *login.UseCase
	LoginHandler       *login.Handler
	Logout        *logout.UseCase
	LogoutHandler *logout.Handler
}

// Config contiene la configuración del módulo Auth
type Config struct {
	// Database - usa interfaz para permitir mocking
	PostgresPool postgres.PgxPool

	// Dragonfly - para cache e idempotencia
	DragonflyClient *redis.Client

	// Token
	PasetoKey []byte

	// TTLs
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	EmailVerificationTTL time.Duration
	PasswordResetTTL     time.Duration

	// Event Bus
	EventPublisher *eventbus.EventBus

	// Environment
	IsProduction bool
}

// NewModule crea e inicializa el módulo Auth con todas sus dependencias
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// 1. Inicializar Repository (PostgreSQL)
	m.Repository = postgres.NewUserRepository(cfg.PostgresPool)

	// 2. Inicializar Password Hasher (Argon2id)
	m.PasswordHasher = password.NewHasher()

	// 3. Inicializar Verification Service (PASETO)
	verifSvc, err := verification.NewVerificationService(verification.VerificationConfig{
		SymmetricKey: cfg.PasetoKey,
		TTL:          cfg.EmailVerificationTTL,
	}, cfg.DragonflyClient)
	if err != nil {
		return nil, err
	}
	m.VerificationService = verifSvc

	// 4. Inicializar Token Service (PASETO)
	tokenSvc, err := token.NewPasetoService(token.PasetoConfig{
		SymmetricKey: cfg.PasetoKey,
		AccessTTL:    cfg.AccessTokenTTL,
		RefreshTTL:   cfg.RefreshTokenTTL,
	})
	if err != nil {
		return nil, err
	}
	m.TokenService = tokenSvc

	// 5. Event Publisher (Dragonfly Streams)
	m.EventPublisher = cfg.EventPublisher

	// ========== FEATURES ==========

	// Register - con event publisher para event-driven architecture
	m.Register = register.NewUseCase(register.UseCaseDeps{
		Repo:           m.Repository,
		VerifySvc:      m.VerificationService,
		Hasher:         m.PasswordHasher,
		TokenSvc:       m.TokenService,
		EventPublisher: m.EventPublisher,
	})
	// Register Handler con soporte de idempotency (Dragonfly)
	m.RegisterHandler = func() *register.Handler {
		return register.NewHandlerWithIdempotency(m.Register, cfg.DragonflyClient, cfg.IsProduction)
	}

	// Verify Email
	m.VerifyEmail = verify_email.NewUseCase(verify_email.UseCaseDeps{
		VerifySvc: m.VerificationService,
		Repo:      m.Repository,
		TokenSvc:  m.TokenService,
	})
	m.VerifyEmailHandler = func() *verify_email.Handler {
		return verify_email.NewHandler(m.VerifyEmail, cfg.IsProduction)
	}

	// Login
	m.Login = login.NewUseCase(login.UseCaseDeps{
		Repo:     m.Repository,
		Hasher:   m.PasswordHasher,
		TokenSvc: m.TokenService,
	})
	m.LoginHandler = login.NewHandler(m.Login, cfg.IsProduction)

	// Logout
	m.Logout = logout.NewUseCase(logout.UseCaseDeps{
		TokenSvc:    m.TokenService,
		DragonflyDB: cfg.DragonflyClient,
	})
	m.LogoutHandler = logout.NewHandler(m.Logout, cfg.IsProduction)

	// Register domain error mappings
	registerAuthErrorMappings()

	slog.Info("Auth module initialized", "features", []string{"register", "verify_email", "login", "logout"})

	return m, nil
}

// MustNewModule crea el módulo y hace panic si hay error
func MustNewModule(cfg Config) *Module {
	mod, err := NewModule(cfg)
	if err != nil {
		panic(err)
	}
	return mod
}

// GeneratePasetoKey genera una clave PASETO aleatoria de 32 bytes
func GeneratePasetoKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// registerAuthErrorMappings registra los mapeos de errores de dominio auth
// a respuestas HTTP RFC 7807.
func registerAuthErrorMappings() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		// Credenciales
		case errors.Is(err, domain.ErrInvalidCredentials):
			return serrors.ErrUnauthorized("Credenciales inválidas", err)
		case errors.Is(err, domain.ErrInvalidPassword), errors.Is(err, domain.ErrPasswordTooShort), errors.Is(err, domain.ErrWeakPassword):
			return serrors.ErrBadRequest("La contraseña no cumple los requisitos de seguridad", err)
		case errors.Is(err, domain.ErrPasswordMismatch):
			return serrors.ErrBadRequest("Las contraseñas no coinciden", err)

		// Verificación de email
		case errors.Is(err, domain.ErrEmailNotVerified), errors.Is(err, domain.ErrAccountPending):
			return serrors.ErrUnauthorized("Email no verificado. Revisa tu bandeja de entrada.", err)
		case errors.Is(err, domain.ErrUserAlreadyVerified):
			return serrors.ErrConflict("El email ya fue verificado", err)
		case errors.Is(err, domain.ErrInvalidVerificationToken), errors.Is(err, domain.ErrTokenInvalid):
			return serrors.ErrUnauthorized("Token de verificación inválido o expirado", err)
		case errors.Is(err, domain.ErrTokenExpired):
			return serrors.ErrUnauthorized("El token ha expirado. Solicita uno nuevo.", err)
		case errors.Is(err, domain.ErrTokenRevoked), errors.Is(err, domain.ErrTokenNotFound):
			return serrors.ErrUnauthorized("Token inválido", err)

		// Cuenta
		case errors.Is(err, domain.ErrAccountLocked):
			return serrors.ErrTooManyRequests("Cuenta bloqueada temporalmente. Intenta más tarde.", err)
		case errors.Is(err, domain.ErrAccountSuspended):
			return serrors.ErrForbidden("Cuenta suspendida. Contacta a soporte.", err)
		case errors.Is(err, domain.ErrAccountInactive), errors.Is(err, domain.ErrAccountDisabled):
			return serrors.ErrForbidden("Cuenta deshabilitada", err)
		case errors.Is(err, domain.ErrEmailAlreadyExists):
			return serrors.ErrConflict("El email ya está registrado", err)
		case errors.Is(err, domain.ErrUserNotFound):
			return serrors.ErrNotFound("Usuario no encontrado", err)

		// Sesión
		case errors.Is(err, domain.ErrSessionExpired), errors.Is(err, domain.ErrSessionNotFound):
			return serrors.ErrUnauthorized("Sesión expirada. Inicia sesión nuevamente.", err)

		// Validación
		case errors.Is(err, domain.ErrInvalidEmail):
			return serrors.ErrBadRequest("Dirección de correo inválida", err)
		case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrValidationError):
			return serrors.ErrBadRequest("Datos de entrada inválidos", err)

		// MFA
		case errors.Is(err, domain.ErrMFARequired):
			return nil // MFA flow has its own handling — don't treat as error
		case errors.Is(err, domain.ErrMFAInvalidCode), errors.Is(err, domain.ErrInvalidBackupCode), errors.Is(err, domain.ErrMFAInvalidRecoveryCode):
			return serrors.ErrUnauthorized("Código MFA inválido", err)
		case errors.Is(err, domain.ErrMFACodeExpired):
			return serrors.ErrUnauthorized("Código MFA expirado. Solicita uno nuevo.", err)
		case errors.Is(err, domain.ErrMFAAlreadyEnabled):
			return serrors.ErrConflict("MFA ya está habilitado", err)
		case errors.Is(err, domain.ErrMFAInvalidMethod):
			return serrors.ErrBadRequest("Método MFA no configurado", err)
		case errors.Is(err, domain.ErrMFANotEnabled):
			return serrors.ErrBadRequest("MFA no habilitado para este usuario", err)
		case errors.Is(err, domain.ErrMFARecoveryCodesExhausted):
			return serrors.ErrBadRequest("Códigos de recuperación agotados", err)

		// OAuth / Identidad
		case errors.Is(err, domain.ErrOAuthProviderNotFound):
			return serrors.ErrBadRequest("Proveedor OAuth no soportado", err)
		case errors.Is(err, domain.ErrOAuthCodeInvalid), errors.Is(err, domain.ErrOAuthExchangeFailed):
			return serrors.ErrUnauthorized("Error de autenticación OAuth", err)
		case errors.Is(err, domain.ErrIdentityAlreadyExists):
			return serrors.ErrConflict("Ya existe una cuenta vinculada a este proveedor", err)
		case errors.Is(err, domain.ErrIdentityNotFound):
			return serrors.ErrNotFound("Cuenta externa no encontrada", err)

		// Roles / Permisos
		case errors.Is(err, domain.ErrPermissionDenied):
			return serrors.ErrForbidden("Permiso denegado", err)
		case errors.Is(err, domain.ErrRoleNotFound), errors.Is(err, domain.ErrPermissionNotFound):
			return serrors.ErrNotFound("Recurso no encontrado", err)

		default:
			return nil
		}
	})
}

// RegisterHandlerFactory crea un handler para el endpoint de registro
func RegisterHandlerFactory(reg *register.UseCase) *register.Handler {
	return register.NewHandler(reg)
}

// DefaultTTLs retorna los TTLs por defecto recomendados
func DefaultTTLs() (access, refresh, emailVerif, passwordReset time.Duration) {
	access = 15 * time.Minute
	refresh = 7 * 24 * time.Hour // 7 días
	emailVerif = 24 * time.Hour
	passwordReset = 1 * time.Hour
	return
}

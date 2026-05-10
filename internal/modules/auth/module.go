package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/oauth"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/password"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/login"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/logout"
	oauthorize "github.com/ProacTrip/Backend/internal/modules/auth/features/oauth/authorize"
	oacallback "github.com/ProacTrip/Backend/internal/modules/auth/features/oauth/callback"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/me"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/register"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/resend_verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/verify_email"
	sendverification "github.com/ProacTrip/Backend/internal/modules/notification/features/send_verification_email"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/redis/go-redis/v9"
)

// Punto de entrada del módulo Auth.
// Inicializa dependencias y mapea errores de dominio a respuestas HTTP RFC 9457.

type Module struct {
	// Repository
	Repository    domain.UserRepository
	OAuthRepo     domain.OAuthRepository

	// Adapters (implementaciones concretas)
	PasswordHasher      *password.Hasher
	TokenService        *token.PasetoService
	VerificationService *verification.VerificationService

	// OAuth
	GoogleOAuth *oauth.GoogleOAuth

	// Event Bus (for event-driven architecture)
	EventPublisher *eventbus.EventBus

	// Features
	Register           *register.UseCase
	RegisterHandler    func() *register.Handler
	VerifyEmail        *verify_email.UseCase
	VerifyEmailHandler func() *verify_email.Handler
	Login              *login.UseCase
	LoginHandler       *login.Handler
	Logout             *logout.UseCase
	LogoutHandler      *logout.Handler
	MeHandler          *me.Handler

	// OAuth Features
	OAuthAuthorize *oauthorize.UseCase
	OAuthAuthorizeHandler *oauthorize.Handler
	OAuthCallback         *oacallback.UseCase
	OAuthCallbackHandler  *oacallback.Handler

	// Resend Verification — wired after notification module is initialized
	ResendVerificationHandler *resend_verification.Handler
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

	// OAuth
	OAuthConfig  config.OAuthConfig // credenciales OAuth de config.Config
	FrontendURL  string             // URL del frontend para redirects (callback)
	IsProduction bool
	CookieDomain string             // Dominio para cookies (.proactrip.com en prod, vacío en dev)

	// Event Bus
	EventPublisher *eventbus.EventBus

	// EnvResolver resolves currency/language/country/timezone from client IP.
	// Nil-safe: when nil, registration proceeds without env defaults in the event.
	// Implementation lives in the environment module; injected here to avoid
	// auth → environment coupling.
	EnvResolver register.EnvironmentResolver
}

// NewModule crea e inicializa el módulo Auth con todas sus dependencias
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// 1. Inicializar Repository (PostgreSQL)
	m.Repository = postgres.NewUserRepository(cfg.PostgresPool)

	// 1b. Inicializar OAuth Repository (PostgreSQL)
	m.OAuthRepo = postgres.NewOAuthRepository(cfg.PostgresPool)

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
		SymmetricKey:    cfg.PasetoKey,
		AccessTTL:       cfg.AccessTokenTTL,
		RefreshTTL:      cfg.RefreshTokenTTL,
		DragonflyClient: cfg.DragonflyClient,
	})
	if err != nil {
		return nil, err
	}
	m.TokenService = tokenSvc

	// 5. Event Publisher (Dragonfly Streams)
	m.EventPublisher = cfg.EventPublisher

	// 6. Inicializar GoogleOAuth adapter
	m.GoogleOAuth = oauth.NewGoogleOAuth(cfg.OAuthConfig)

	// ========== FEATURES ==========

	// Register - con event publisher para event-driven architecture
	m.Register = register.NewUseCase(register.UseCaseDeps{
		Repo:           m.Repository,
		VerifySvc:      m.VerificationService,
		Hasher:         m.PasswordHasher,
		TokenSvc:       m.TokenService,
		EventPublisher: m.EventPublisher,
		EnvResolver:    cfg.EnvResolver,
	})
	// Register Handler con soporte de idempotency (Dragonfly)
	m.RegisterHandler = func() *register.Handler {
		return register.NewHandlerWithIdempotency(m.Register, cfg.DragonflyClient, cfg.IsProduction, cfg.CookieDomain)
	}

	// Verify Email
	m.VerifyEmail = verify_email.NewUseCase(verify_email.UseCaseDeps{
		VerifySvc: m.VerificationService,
		Repo:      m.Repository,
		TokenSvc:  m.TokenService,
	})
	m.VerifyEmailHandler = func() *verify_email.Handler {
		return verify_email.NewHandler(m.VerifyEmail, cfg.IsProduction, cfg.CookieDomain)
	}

	// Login
	m.Login = login.NewUseCase(login.UseCaseDeps{
		Repo:     m.Repository,
		Hasher:   m.PasswordHasher,
		TokenSvc: m.TokenService,
	})
	m.LoginHandler = login.NewHandler(m.Login, cfg.IsProduction, cfg.CookieDomain)

	// Logout
	m.Logout = logout.NewUseCase(logout.UseCaseDeps{
		TokenSvc:    m.TokenService,
		DragonflyDB: cfg.DragonflyClient,
	})
	m.LogoutHandler = logout.NewHandler(m.Logout, cfg.IsProduction, cfg.CookieDomain)

	// Me (current user)
	m.MeHandler = me.NewHandler(m.TokenService, m.Repository)

	// ========== OAUTH FEATURES ==========

	// Provider selector — resuelve el proveedor OAuth por código
	providerSelector := newOAuthProviderSelector(m.GoogleOAuth)

	// OAuth Authorize
	m.OAuthAuthorize = oauthorize.NewUseCase(oauthorize.UseCaseDeps{
		StateTokenSvc: m.TokenService,
		ProviderSel:   providerSelector,
		Dragonfly:     cfg.DragonflyClient,
	})
	m.OAuthAuthorizeHandler = oauthorize.NewHandler(m.OAuthAuthorize)

	// OAuth Callback
	m.OAuthCallback = oacallback.NewUseCase(oacallback.UseCaseDeps{
		Repo:           m.Repository,
		OAuthRepo:      m.OAuthRepo,
		StateTokenSvc:  m.TokenService,
		ProviderSel:    providerSelector,
		TokenSvc:       m.TokenService,
		Dragonfly:      cfg.DragonflyClient,
		EventPublisher: m.EventPublisher,
	})
	m.OAuthCallbackHandler = oacallback.NewHandler(m.OAuthCallback, cfg.IsProduction, cfg.FrontendURL, cfg.CookieDomain)

	// Register domain error mappings
	registerAuthErrorMappings()

	slog.Info("Auth module initialized", "features", []string{"register", "verify_email", "login", "logout", "resend_verification", "oauth_authorize", "oauth_callback"})

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

// =============================================================================
// Resend Verification — Wired after notification module is initialized
// (notification module is created after auth, so this is called from app.go)
// =============================================================================

// resendNotificationAdapter adapta el SendVerificationEmailUseCase del módulo
// notification al NotificationPort local definido en resend_verification/usecase.go.
type resendNotificationAdapter struct {
	uc *sendverification.UseCase
}

// SendVerificationEmail implementa resend_verification.NotificationPort.
func (a *resendNotificationAdapter) SendVerificationEmail(ctx context.Context, userID uuid.UUID, email, token string) error {
	return a.uc.Execute(ctx, userID, email, token)
}

// WireResendVerification crea el usecase y handler del feature resend-verification
// y los almacena en el Module. Debe llamarse DESPUÉS de que el notification module
// esté inicializado, pasando su SendVerificationEmailUseCase.
func (m *Module) WireResendVerification(notifUC *sendverification.UseCase) {
	adapter := &resendNotificationAdapter{uc: notifUC}

	uc := resend_verification.NewUseCase(resend_verification.UseCaseDeps{
		Repo:     m.Repository,
		TokenSvc: m.VerificationService,
		Notifier: adapter,
	})

	m.ResendVerificationHandler = resend_verification.NewHandler(uc)
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
// a respuestas HTTP RFC 9457.
func registerAuthErrorMappings() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		// Autenticación
		case errors.Is(err, domain.ErrNotAuthenticated):
			return serrors.ErrUnauthorized("se requiere autenticación", err)
		case errors.Is(err, domain.ErrTokenInvalid):
			return serrors.ErrUnauthorized("token inválido o expirado", err)

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
		case errors.Is(err, domain.ErrInvalidVerificationToken):
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
			return serrors.ErrValidationError("Dirección de correo inválida", err)
		case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrValidationError):
			return serrors.ErrValidationError("Datos de entrada inválidos", err)

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

		// OAuth
		case errors.Is(err, domain.ErrOAuthProviderNotFound):
			return serrors.ErrBadRequest("Proveedor OAuth no soportado", err)
		case errors.Is(err, domain.ErrOAuthCodeMissing):
			return serrors.ErrBadRequest("falta el parámetro code del proveedor OAuth", err)
		case errors.Is(err, domain.ErrOAuthStateMissing):
			return serrors.ErrBadRequest("falta el parámetro state del proveedor OAuth", err)
		case errors.Is(err, domain.ErrOAuthStateInvalid):
			return serrors.ErrBadRequest("state OAuth inválido o expirado", err)
		case errors.Is(err, domain.ErrOAuthAccessDenied):
			return serrors.ErrBadRequest("acceso OAuth denegado por el usuario", err)
		case errors.Is(err, domain.ErrOAuthExchangeFailed):
			return serrors.ErrUnauthorized("Error de autenticación OAuth", err)

		// Identidad
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

// =============================================================================
// OAuth Provider Selector
// =============================================================================

// oauthProviderSelector implementa OAuthProviderSelector para resolver
// proveedores por código (ej. "google" → GoogleOAuth).
type oauthProviderSelector struct {
	providers map[string]domain.OAuthProvider
}

func newOAuthProviderSelector(googleOAuth domain.OAuthProvider) *oauthProviderSelector {
	return &oauthProviderSelector{
		providers: map[string]domain.OAuthProvider{
			"google": googleOAuth,
		},
	}
}

// GetProvider devuelve el proveedor OAuth por código.
// Retorna ErrOAuthProviderNotFound si el proveedor no está soportado.
func (s *oauthProviderSelector) GetProvider(providerCode string) (domain.OAuthProvider, error) {
	provider, ok := s.providers[providerCode]
	if !ok {
		return nil, domain.ErrOAuthProviderNotFound
	}
	return provider, nil
}

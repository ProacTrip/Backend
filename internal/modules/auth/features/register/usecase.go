package register

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// Lógica de negocio del registro.
// Flujo: validar → crear usuario → generar tokens → publicar evento.
// Patrón: publica eventos, módulos User y Notification consumen independientemente.

type VerificationService interface {
	GenerateToken(ctx context.Context, email string) (string, error)
	VerifyToken(ctx context.Context, token string) (*verification.TokenClaims, error)
}

// =============================================================================
// PasswordHasher port - interfaz para hashear contraseñas
// =============================================================================

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}

// =============================================================================
// TokenService port - interfaz para generar auth tokens
// =============================================================================

type TokenService interface {
	GenerateTokenPair(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (*token.TokenPair, error)
}

// =============================================================================
// EventPublisher port - interfaz para publicar eventos
// =============================================================================

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UseCase - Lógica de negocio del registro
// Patrón: Publica eventos. Los módulos User y Notification consumen independently.
// =============================================================================

type UseCase struct {
	repo           domain.UserRepository
	verifySvc      VerificationService
	hasher         PasswordHasher
	tokenSvc       TokenService
	eventPublisher EventPublisher
	envResolver    EnvironmentResolver
}

type UseCaseDeps struct {
	Repo           domain.UserRepository
	VerifySvc      VerificationService
	Hasher         PasswordHasher
	TokenSvc       TokenService
	EventPublisher EventPublisher
	EnvResolver    EnvironmentResolver // nil-safe: when nil, env defaults are skipped
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		repo:           deps.Repo,
		verifySvc:      deps.VerifySvc,
		hasher:         deps.Hasher,
		tokenSvc:       deps.TokenSvc,
		eventPublisher: deps.EventPublisher,
		envResolver:    deps.EnvResolver,
	}
}

// Execute procesa el registro de usuario:
// 1. Resuelve defaults de entorno via IP (si hay resolver e IP)
// 2. Valida email y password
// 3. Verifica que el email no exista
// 4. Hashea la contraseña
// 5. Obtiene el rol "client" por defecto
// 6. Crea el usuario en DB
// 7. Genera verification token y auth tokens
// 8. Publica evento con env defaults para notification module
// Retorna tokens para Set-Cookie segun AUTH_API.md
func (uc *UseCase) Execute(ctx context.Context, cmd Command, envIP string) (*Response, error) {
	// Resolver defaults de entorno desde la IP del usuario.
	// Estos defaults son opcionales: si el resolver falla o no existe,
	// la registración continúa normalmente sin campos de entorno en el evento.
	var envLanguage, envCurrency, envCountry, envTimezone string
	if uc.envResolver != nil && envIP != "" {
		currency, language, country, tz, err := uc.envResolver.ResolveDefaults(ctx, envIP)
		if err != nil {
			slog.WarnContext(ctx, "env resolver failed, continuing without env defaults",
				slog.String("env_ip", envIP),
				slog.Any("error", err),
			)
		} else {
			envLanguage = language
			envCurrency = currency
			envCountry = country
			envTimezone = tz
		}
	}

	// 1. Validar email
	if _, err := mail.ParseAddress(cmd.Email); err != nil {
		return nil, domain.ErrInvalidEmail
	}

	// 2. Verificar si el email ya existe
	existingUser, err := uc.repo.GetByEmail(ctx, cmd.Email)
	if err != nil && err != domain.ErrUserNotFound {
		return nil, err
	}
	if existingUser != nil {
		return nil, domain.ErrEmailAlreadyExists
	}

	// 3. Validar password mínimo 8 caracteres
	if len(cmd.Password) < 8 {
		return nil, domain.ErrPasswordTooShort
	}

	// 4. Hashear contraseña
	passwordHash, err := uc.hasher.Hash(cmd.Password)
	if err != nil {
		return nil, domain.ErrWeakPassword
	}

	// 5. Obtener rol "client" por defecto
	role, err := uc.repo.GetRoleByName(ctx, "client")
	if err != nil {
		return nil, domain.ErrRoleNotFound
	}

	// 6. Crear usuario en DB
	user := domain.NewUser(cmd.Email, passwordHash, role.ID)
	if err := uc.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	// 7. Generar verification token para el email de verificación
	verificationToken, err := uc.verifySvc.GenerateToken(ctx, cmd.Email)
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate verification token",
			slog.String("email", cmd.Email),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("register: verification token generation failed: %w", err)
	}

	// 8. Generar auth tokens (cookies) - sesión pre-verificada con privilegios limitados
	var accessToken, refreshToken string
	if uc.tokenSvc != nil {
		sessionID := uuid.Must(uuid.NewV7())
		tokenPair, err := uc.tokenSvc.GenerateTokenPair(user.ID, user.Email, user.RoleID, sessionID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to generate token pair after registration",
				slog.String("email", cmd.Email),
				slog.Any("error", err),
			)
			return nil, fmt.Errorf("register: token generation failed: %w", err)
		}
		if tokenPair != nil {
			accessToken = tokenPair.AccessToken
			refreshToken = tokenPair.RefreshToken
		}
	}

	// 9. Publicar evento para notification module (flat payload para Dragonfly/Redis Streams)
	if uc.eventPublisher != nil {
		event := eventbus.NewUserRegisteredEvent(
			user.ID.String(),
			user.Email,
			verificationToken,
			envLanguage,
			envCurrency,
			envCountry,
			envTimezone,
		)
		streamName := eventbus.StreamName("auth.user.registered")
		flatPayload := map[string]interface{}{
			"event_type":         "user_registered",
			"event_version":      int64(1),
			"aggregate_id":       user.ID.String(),
			"timestamp":          event.Timestamp,
			"user_id":            user.ID.String(),
			"email":              user.Email,
			"verification_token": verificationToken,
		}
		// Env defaults — solo incluidos cuando están resueltos (omitempty equivalente en map)
		if envLanguage != "" {
			flatPayload["language_code"] = envLanguage
		}
		if envCurrency != "" {
			flatPayload["currency_code"] = envCurrency
		}
		if envCountry != "" {
			flatPayload["country_code"] = envCountry
		}
		if envTimezone != "" {
			flatPayload["timezone_name"] = envTimezone
		}
		_, _ = uc.eventPublisher.Publish(ctx, streamName, flatPayload)
	}

	// Retorna message + tokens para Set-Cookie
	return &Response{
		Message:      "Registration successful. Please verify your email.",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

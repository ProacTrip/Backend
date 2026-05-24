package register

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// Lógica de negocio del registro.
// Flujo: validar → crear usuario → publicar evento.
// Patrón: publica eventos, módulos User y Notification consumen independientemente.
// No genera tokens ni cookies — el usuario debe loguearse después de verificar el email.

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
// EventPublisher port - interfaz para publicar eventos
// =============================================================================

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]any) (string, error)
}

// =============================================================================
// UseCase - Lógica de negocio del registro
// Patrón: Publica eventos. Los módulos User y Notification consumen independientemente.
// =============================================================================

type UseCase struct {
	repo           domain.UserRepository
	verifySvc      VerificationService
	hasher         PasswordHasher
	eventPublisher EventPublisher
}

type UseCaseDeps struct {
	Repo           domain.UserRepository
	VerifySvc      VerificationService
	Hasher         PasswordHasher
	EventPublisher EventPublisher
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		repo:           deps.Repo,
		verifySvc:      deps.VerifySvc,
		hasher:         deps.Hasher,
		eventPublisher: deps.EventPublisher,
	}
}

// Execute procesa el registro de usuario:
// 1. Verifica que el email no exista
// 2. Hashea la contraseña
// 3. Obtiene el rol "client" por defecto
// 4. Crea el usuario en DB
// 5. Genera verification token
// 6. Publica evento con datos mínimos para notification module
// No genera tokens de sesión — el usuario debe loguearse después de verificar.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Verificar si el email ya existe
	existingUser, err := uc.repo.GetByEmail(ctx, cmd.Email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}
	if existingUser != nil {
		return nil, domain.ErrEmailAlreadyExists
	}

	// 2. Hashear contraseña
	passwordHash, err := uc.hasher.Hash(cmd.Password)
	if err != nil {
		return nil, domain.ErrWeakPassword
	}

	// 3. Obtener rol "client" por defecto
	role, err := uc.repo.GetRoleByName(ctx, "client")
	if err != nil {
		return nil, domain.ErrRoleNotFound
	}

	// 4. Crear usuario en DB
	user := domain.NewUser(cmd.Email, passwordHash, cmd.FirstName, role.ID)
	if err := uc.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	// 5. Generar verification token para el email de verificación
	verificationToken, err := uc.verifySvc.GenerateToken(ctx, cmd.Email)
	if err != nil {
		slog.ErrorContext(ctx, "register: falló la generación del token de verificación",
			slog.String("email", cmd.Email),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("register: verification token generation failed: %w", err)
	}

	// 6. Publicar evento mínimo para notification module
	if uc.eventPublisher != nil {
		// Evento sin datos de entorno — el user consumer resuelve defaults por su cuenta.
		event := eventbus.NewUserRegisteredEvent(
			user.ID.String(),
			user.Email,
			verificationToken,
			"", // language_code — no se resuelve en registro
			"", // currency_code — no se resuelve en registro
			"", // country_code — no se resuelve en registro
			"", // timezone_name — no se resuelve en registro
		)
		streamName := eventbus.StreamName("auth.user.registered")
		flatPayload := map[string]any{
			"event_type":         "user_registered",
			"event_version":      int64(1),
			"aggregate_id":       user.ID.String(),
			"timestamp":          event.Timestamp,
			"user_id":            user.ID.String(),
			"email":              user.Email,
			"verification_token": verificationToken,
		}
		// El nombre (first_name) es opcional — el user consumer lo pasa al perfil y
		// al template de verificación (fallback "Usuario" si está vacío)
		if cmd.FirstName != "" {
			flatPayload["first_name"] = cmd.FirstName
		}
		if _, err := uc.eventPublisher.Publish(ctx, streamName, flatPayload); err != nil {
			slog.ErrorContext(ctx, "register: falló la publicación del evento",
				slog.String("event", "auth.user.registered"),
				slog.Any("error", err),
			)
		}
	}

	// Retorna solo message — sin tokens ni cookies.
	return &Response{
		Message: "Registro exitoso. Por favor verificá tu email.",
	}, nil
}

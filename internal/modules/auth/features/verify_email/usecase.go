package verify_email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// Lógica de negocio de verificación de email.
// Flujo: validar token → buscar usuario → marcar verificado → publicar evento → generar tokens.

type VerificationService interface {
	VerifyToken(ctx context.Context, tokenString string) (*verification.TokenClaims, error)
}

type TokenService interface {
	GenerateTokenPair(userID uuid.UUID, email string, role string, roleID uuid.UUID) (*token.TokenPair, error)
}

// =============================================================================
// EventPublisher port — abstraction to publish domain events.
// =============================================================================

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]any) (string, error)
}

type UseCase struct {
	verifySvc      VerificationService
	repo           domain.UserRepository
	tokenSvc       TokenService
	eventPublisher EventPublisher
}

type UseCaseDeps struct {
	VerifySvc      VerificationService
	Repo           domain.UserRepository
	TokenSvc       TokenService
	EventPublisher EventPublisher // nil-safe: cuando es nil, no se publican eventos
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		verifySvc:      deps.VerifySvc,
		repo:           deps.Repo,
		tokenSvc:       deps.TokenSvc,
		eventPublisher: deps.EventPublisher,
	}
}

// Execute procesa la verificación de email:
// 1. Valida el token de verificación.
// 2. Busca el usuario por email.
// 3. Si el email no estaba verificado, lo marca como verificado.
// 4. Publica evento auth.user.verified con language_code para el user module.
// 5. Genera tokens de sesión y retorna.
func (uc *UseCase) Execute(ctx context.Context, cmd Command, languageCode string) (*Response, error) {
	claims, err := uc.verifySvc.VerifyToken(ctx, cmd.Token)
	if err != nil {
		return nil, fmt.Errorf("verify email: %w", domain.ErrTokenInvalid)
	}

	user, err := uc.repo.GetByEmail(ctx, claims.Email)
	if err != nil {
		return nil, fmt.Errorf("verify email: %w", err)
	}

	// Rastrear si es la primera verificación (para publicar el evento).
	isFirstVerification := !user.EmailVerified

	// Early-return: si el email ya está verificado, saltamos la actualización
	// innecesaria en DB y vamos directo a generar tokens de sesión.
	if isFirstVerification {
		user.VerifyEmail()
		if err := uc.repo.Update(ctx, user); err != nil {
			return nil, fmt.Errorf("verify email: update user: %w", err)
		}
	}

	// Publicar evento auth.user.verified solo en la primera verificación.
	// Incluye language_code del header Accept-Language.
	if isFirstVerification {
		// Evento mínimo — el user consumer resuelve defaults de entorno por su cuenta.
		// Pasamos "" para campos que ya no resolvemos desde IP.
		event := eventbus.NewUserVerifiedEvent(
			user.ID.String(),
			user.Email,
			languageCode,
			"", // currency_code — no se resuelve en verify-email
			"", // country_code — no se resuelve en verify-email
			"", // timezone_name — no se resuelve en verify-email
			"", // client_ip — no se resuelve en verify-email
		)
		streamName := eventbus.StreamName("auth.user.verified")
		flatPayload := map[string]any{
			"event_type":    string(eventbus.UserVerified),
			"event_version": int64(1),
			"aggregate_id":  user.ID.String(),
			"timestamp":     event.Timestamp,
			"user_id":       user.ID.String(),
			"email":         user.Email,
		}
		// Solo incluimos language_code cuando está resuelto
		if languageCode != "" {
			flatPayload["language_code"] = languageCode
		}
		if uc.eventPublisher != nil {
			if _, err := uc.eventPublisher.Publish(ctx, streamName, flatPayload); err != nil {
				slog.ErrorContext(ctx, "verify email: falló la publicación del evento",
					slog.String("event", "auth.user.verified"),
					slog.String("user_id", user.ID.String()),
					slog.Any("error", err),
				)
			}
		}
	}

	tokenPair, err := uc.tokenSvc.GenerateTokenPair(user.ID, user.Email, user.RoleName, user.RoleID)
	if err != nil {
		return nil, fmt.Errorf("verify email: generate tokens: %w", err)
	}

	return &Response{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		User: UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			RoleName: user.RoleName,
		},
	}, nil
}

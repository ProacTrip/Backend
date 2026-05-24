package resend_verification

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Ports locales — definidos por el usecase para inversión de dependencias.
// Implementados por adapters externos (notification module, etc.).
// =============================================================================

// NotificationPort es el puerto local para enviar emails de verificación.
// Implementado por el adapter de notification module en module.go.
type NotificationPort interface {
	SendVerificationEmail(ctx context.Context, userID uuid.UUID, email, verificationToken string) error
}

// VerificationTokenService es el puerto local para generar tokens de verificación.
// Implementado por el VerificationService adapter en el módulo auth.
type VerificationTokenService interface {
	GenerateToken(ctx context.Context, email string) (string, error)
}

// =============================================================================
// UseCase — Lógica de negocio del reenvío de verificación.
// Flujo: buscar usuario → si no existe → 200 (anti-enumeración).
//
//	si existe y verificado → 200 (anti-enumeración).
//	si existe y no verificado → generar token, enviar email → 200.
//
// Infra errors (DB, Dragonfly, notification) → logged, still 200.
// =============================================================================

type UseCase struct {
	repo     domain.UserRepository
	tokenSvc VerificationTokenService
	notifier NotificationPort
}

type UseCaseDeps struct {
	Repo     domain.UserRepository
	TokenSvc VerificationTokenService
	Notifier NotificationPort
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		repo:     deps.Repo,
		tokenSvc: deps.TokenSvc,
		notifier: deps.Notifier,
	}
}

// DefaultResponse es el mensaje genérico anti-enumeración que siempre se retorna.
const DefaultResponse = "Si el email existe y no está verificado, se enviará un nuevo email de verificación."

// Execute procesa el reenvío de verificación de email.
// Siempre retorna 200 OK con el mismo mensaje para prevenir enumeración de usuarios.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	resp := &Response{Message: DefaultResponse}

	user, err := uc.repo.GetByEmail(ctx, cmd.Email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// User doesn't exist — return generic message (anti-enumeration)
			return resp, nil
		}
		// Infrastructure error (DB down, timeout) — log but don't expose
		slog.ErrorContext(ctx, "failed to look up user for verification resend",
			slog.String("email", cmd.Email),
			slog.Any("error", err),
		)
		return resp, nil
	}

	// User exists — check if already verified
	if user.EmailVerified {
		return resp, nil // Already verified — return generic message (anti-enumeration)
	}

	// User exists and is unverified — generate new verification token
	token, err := uc.tokenSvc.GenerateToken(ctx, cmd.Email)
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate verification token for resend",
			slog.String("email", cmd.Email),
			slog.Any("error", err),
		)
		return resp, nil
	}

	// Send verification email via notification port (fire-and-forget semantics)
	if err := uc.notifier.SendVerificationEmail(ctx, user.ID, cmd.Email, token); err != nil {
		slog.ErrorContext(ctx, "failed to send verification email for resend",
			slog.String("email", cmd.Email),
			slog.Any("error", err),
		)
		return resp, nil
	}

	slog.InfoContext(ctx, "verification email resent",
		slog.String("email", cmd.Email),
	)

	return resp, nil
}

// Caso de uso: Envío de email de verificación.
// Maneja la lógica de envío de emails de verificación.
package send_verification_email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
	"github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// EmailSender port - interfaz para enviar emails
// =============================================================================

type EmailSender interface {
	SendVerifyEmail(ctx context.Context, to, verificationURL string) error
}

// =============================================================================
// UseCase - Lógica para enviar emails de verificación
// =============================================================================

type UseCase struct {
	repo           domain.NotificationRepository
	sender         EmailSender
	frontendConfig config.FrontendConfig
}

type Deps struct {
	Repo           domain.NotificationRepository
	Sender         EmailSender
	FrontendConfig config.FrontendConfig
}

func NewUseCase(deps Deps) *UseCase {
	return &UseCase{
		repo:           deps.Repo,
		sender:         deps.Sender,
		frontendConfig: deps.FrontendConfig,
	}
}

// Execute envía un email de verificación con idempotencia
func (uc *UseCase) Execute(ctx context.Context, userID uuid.UUID, email, verificationToken string) error {
	// 1. Generar URL de verificación usando la configuración del frontend
	baseURL := uc.frontendConfig.GetURL()
	verificationURL := fmt.Sprintf("%s/auth/verify-email?token=%s", baseURL, verificationToken)

	// 2. Crear notificación alineada con migración (channel + content son OBLIGATORIOS)
	notification := domain.NewEmailNotification(
		userID,
		"Verifica tu email en Proactrip", // subject
		"Haz clic en el siguiente enlace para verificar tu email: "+verificationURL, // content
		"verify_email", // template_code (Resend)
		domain.NotificationTypeTransactional,
		map[string]any{
			"verification_token": verificationToken,
			"verification_url":   verificationURL,
			"email":              email,
		},
	)

	// 3. Intentar guardar (si ya existe por idempotencia, retornar error de conflict)
	existingID, err := uc.repo.Save(ctx, notification)
	if err != nil {
		return errors.ErrInternalError("failed to save notification", err)
	}

	// Si ya existe, verificar si ya fue enviado exitosamente
	if existingID != uuid.Nil {
		existing, getErr := uc.repo.GetByID(ctx, existingID)
		if getErr == nil && existing != nil && existing.Status == domain.NotificationStatusSent {
			slog.Info("verification email already sent", "user_id", userID, "notification_id", existingID)
			return nil // Idempotency: ya enviado, no re-enviar
		}
	}

	// 4. Enviar email
	if err := uc.sender.SendVerifyEmail(ctx, email, verificationURL); err != nil {
		slog.Error("failed to send verification email", "error", err, "email", email)

		// 5. Registrar失败
		if updateErr := uc.repo.MarkFailed(ctx, notification.ID, err.Error()); updateErr != nil {
			slog.Error("failed to mark notification as failed", "error", updateErr)
		}

		return errors.ErrInternalError("failed to send verification email", err)
	}

	// 6. Actualizar estado a enviado (con provider message ID vacío por ahora)
	if err := uc.repo.MarkSent(ctx, notification.ID, ""); err != nil {
		slog.Error("failed to mark notification as sent", "error", err)
		// El email se envió, pero falló el update - no es crítico
	}

	slog.Info("verification email sent successfully", "user_id", userID, "email", email)
	return nil
}

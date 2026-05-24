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
// Resend Template ID para verificación de email
// =============================================================================

// ResendTemplateVerifyEmail es el ID del template de verificación en Resend.
const ResendTemplateVerifyEmail = "c58c6953-1bf9-41f1-9d8d-26d5d77b9879"

// =============================================================================
// EmailSender port - interfaz para enviar emails
// =============================================================================

// EmailSender define el contrato para enviar emails usando templates.
// Retorna el message ID del proveedor en caso de éxito.
type EmailSender interface {
	SendWithTemplate(ctx context.Context, to, templateID string, vars map[string]any) (string, error)
}

// =============================================================================
// Command - Datos de entrada para el caso de uso
// =============================================================================

// Command representa el comando para enviar un email de verificación.
type Command struct {
	UserID            uuid.UUID
	Email             string
	VerificationToken string
	FirstName         string // Opcional — puede estar vacío
	ForceResend       bool   // Si es true, ignora idempotencia y reenvía siempre
}

// Validate valida que los campos obligatorios del comando no estén vacíos.
func (c Command) Validate() error {
	if c.UserID == uuid.Nil {
		return fmt.Errorf("UserID es obligatorio")
	}
	if c.Email == "" {
		return fmt.Errorf("Email es obligatorio")
	}
	if c.VerificationToken == "" {
		return fmt.Errorf("VerificationToken es obligatorio")
	}
	return nil
}

// =============================================================================
// UseCase encapsula la lógica para enviar emails de verificación.
type UseCase struct {
	repo           domain.NotificationRepository
	sender         EmailSender
	frontendConfig config.FrontendConfig
}

// Deps agrupa las dependencias necesarias para construir un UseCase.
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

// Execute envía un email de verificación con idempotencia.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	// 1. Validar comando
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("comando inválido: %w", err)
	}

	// 2. Generar URL de verificación usando la configuración del frontend
	baseURL := uc.frontendConfig.GetURL()
	verificationURL := fmt.Sprintf("%s/auth/verify-email?token=%s", baseURL, cmd.VerificationToken)

	// 3. Crear notificación
	notification, err := domain.NewEmailNotification(cmd.UserID, "verify_email")
	if err != nil {
		return errors.ErrInternalError("failed to create notification", err)
	}

	// 4. Intentar guardar (si ya existe por idempotencia, retornar nil)
	existingID, err := uc.repo.Save(ctx, notification)
	if err != nil {
		return errors.ErrInternalError("failed to save notification", err)
	}

	// Si ya existe, verificar si ya fue enviado exitosamente
	if existingID != uuid.Nil {
		existing, getErr := uc.repo.GetByID(ctx, existingID)
		if getErr == nil && existing != nil && existing.SentAt != nil {
			if !cmd.ForceResend {
				// Envío inicial (event-driven): idempotente, no reenviar.
				slog.Info("verification email already sent",
					"user_id", cmd.UserID,
					"notification_id", existingID,
				)
				return nil
			}
			// Reenvío explícito: reusar registro existente, enviar con nuevo token.
			slog.Info("resending verification email",
				"user_id", cmd.UserID,
				"notification_id", existingID,
			)
			notification.ID = existingID
		}
	}

	// 5. Preparar variables del template
	templateVars := map[string]any{
		"verification_url": verificationURL,
	}
	if cmd.FirstName != "" {
		templateVars["first_name"] = cmd.FirstName
	}

	// 6. Enviar email
	if _, err := uc.sender.SendWithTemplate(ctx, cmd.Email, ResendTemplateVerifyEmail, templateVars); err != nil {
		slog.Error("failed to send verification email",
			"error", err,
			"email", cmd.Email,
		)
		return errors.ErrInternalError("failed to send verification email", err)
	}

	// 7. Actualizar sent_at
	if err := uc.repo.MarkSent(ctx, notification.ID); err != nil {
		slog.Error("failed to mark notification as sent", "error", err)
		// El email se envió, pero falló el update - no es crítico
	}

	slog.Info("verification email sent successfully",
		"user_id", cmd.UserID,
		"email", cmd.Email,
	)

	return nil
}

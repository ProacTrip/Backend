// Caso de uso: Envío de email de cambio de estado de cuenta.
// Maneja la lógica de envío de emails transaccionales cuando un admin
// habilita o deshabilita una cuenta de usuario.
package send_account_status_email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
	"github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Resend Template IDs para cambio de estado de cuenta
// =============================================================================

// TemplateAccountDisabled es el ID del template de cuenta deshabilitada en Resend.
const TemplateAccountDisabled = "d96a15e5-59e2-4c2a-b561-023287e858c5"

// TemplateAccountEnabled es el ID del template de cuenta habilitada en Resend.
const TemplateAccountEnabled = "01929326-fe76-40cd-83bd-1cfeff4ed477"

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

// Command representa el comando para enviar un email de cambio de estado.
// TemplateID determina si se envía el email de "disabled" o "enabled".
type Command struct {
	UserID     uuid.UUID
	Email      string
	TemplateID string // "d96a15e5-..." | "01929326-..."
}

// Validate valida que los campos obligatorios del comando no estén vacíos.
func (c Command) Validate() error {
	if c.UserID == uuid.Nil {
		return fmt.Errorf("UserID es obligatorio")
	}
	if c.Email == "" {
		return fmt.Errorf("Email es obligatorio")
	}
	if c.TemplateID == "" {
		return fmt.Errorf("TemplateID es obligatorio")
	}
	return nil
}

// =============================================================================
// UseCase encapsula la lógica para enviar emails de cambio de estado.
type UseCase struct {
	repo   domain.NotificationRepository
	sender EmailSender
}

// Deps agrupa las dependencias necesarias para construir un UseCase.
type Deps struct {
	Repo   domain.NotificationRepository
	Sender EmailSender
}

// NewUseCase crea un nuevo UseCase de envío de email de cambio de estado.
func NewUseCase(deps Deps) *UseCase {
	return &UseCase{
		repo:   deps.Repo,
		sender: deps.Sender,
	}
}

// Execute envía un email de cambio de estado de cuenta con idempotencia.
// El templateID en el comando determina si es el email de "disabled" o "enabled".
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	// 1. Validar comando
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("comando inválido: %w", err)
	}

	// 2. Determinar template_code según templateID
	var templateCode string
	switch cmd.TemplateID {
	case TemplateAccountDisabled:
		templateCode = "account_disabled"
	case TemplateAccountEnabled:
		templateCode = "account_enabled"
	default:
		return fmt.Errorf("templateID desconocido: %s", cmd.TemplateID)
	}

	// 3. Crear notificación
	notification, err := domain.NewEmailNotification(cmd.UserID, templateCode)
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
			slog.Info("account status email already sent",
				"user_id", cmd.UserID,
				"notification_id", existingID,
				"template_code", templateCode,
			)
			return nil // Idempotencia: ya fue enviado
		}
	}

	// 5. Preparar variables del template
	templateVars := map[string]any{
		"user_email": cmd.Email,
	}

	// 6. Enviar email
	if _, err := uc.sender.SendWithTemplate(ctx, cmd.Email, cmd.TemplateID, templateVars); err != nil {
		slog.Error("failed to send account status email",
			"error", err,
			"email", cmd.Email,
			"template_code", templateCode,
		)
		return errors.ErrInternalError("failed to send account status email", err)
	}

	// 7. Actualizar sent_at
	if err := uc.repo.MarkSent(ctx, notification.ID); err != nil {
		slog.Error("failed to mark notification as sent", "error", err)
		// El email se envió, pero falló el update - no es crítico
	}

	slog.Info("account status email sent successfully",
		"user_id", cmd.UserID,
		"email", cmd.Email,
		"template_code", templateCode,
	)

	return nil
}

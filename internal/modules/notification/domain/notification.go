// Domain: Entidades de dominio para notificaciones.
// Define Notification — lo único que hace es representar un email enviado.
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Tipo de notificación — solo queda transactional
// =============================================================================

// NotificationTypeTransactional es el único tipo de notificación que existe.
const NotificationTypeTransactional = "transactional"

// =============================================================================
// Notification — estructura simplificada
// =============================================================================

// Notification representa un email enviado a través de Resend.
// SentAt es nil si aún no fue enviado, NOT NULL si ya fue enviado.
type Notification struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	TemplateCode string     `json:"template_code,omitempty"`
	SentAt       *time.Time `json:"sent_at,omitzero"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// =============================================================================
// Constructores
// =============================================================================

// NewEmailNotification crea una nueva notificación de email transaccional.
func NewEmailNotification(userID uuid.UUID, templateCode string) (*Notification, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generar UUID de notificación: %w", err)
	}
	now := time.Now()
	return &Notification{
		ID:           id,
		UserID:       userID,
		TemplateCode: templateCode,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

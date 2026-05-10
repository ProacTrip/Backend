// Repository: Interfaz para persistencia de notificaciones.
// Define las operaciones disponibles para el repositorio.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// NotificationRepository define la interfaz para persistencia de notificaciones.
type NotificationRepository interface {
	// Save guarda una notificación (retorna ID existente si ya existe por idempotencia).
	Save(ctx context.Context, n *Notification) (existingID uuid.UUID, err error)

	// GetByID recupera una notificación por su ID.
	GetByID(ctx context.Context, id uuid.UUID) (*Notification, error)

	// MarkSent actualiza estado a enviado.
	MarkSent(ctx context.Context, id uuid.UUID, messageID string) error

	// MarkFailed registra intento fallido.
	MarkFailed(ctx context.Context, id uuid.UUID, errStr string) error

	// MarkDelivered actualiza el estado a entregado con la marca de tiempo del proveedor.
	MarkDelivered(ctx context.Context, id uuid.UUID, deliveredAt time.Time) error

	// UpdateFromWebhook actualiza el estado desde un evento de webhook del proveedor.
	UpdateFromWebhook(ctx context.Context, providerMessageID string, status NotificationStatus, eventTimestamp time.Time) error
}

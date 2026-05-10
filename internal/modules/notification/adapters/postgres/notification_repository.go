// Adapter de PostgreSQL para notificaciones.
// Implementa la interfaz del dominio.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
)

// =============================================================================
// Repositorio de Notificaciones — adapter PostgreSQL
// Alineado con migración 001_notifications.sql (fuente de truth)
// =============================================================================

// NotificationRepository implementa domain.NotificationRepository usando pgxpool.
type NotificationRepository struct {
	db *pgxpool.Pool
}

// =============================================================================
// Constructor
// =============================================================================

// NewNotificationRepository crea un repositorio de notificaciones respaldado por PostgreSQL.
func NewNotificationRepository(db *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// =============================================================================
// Operaciones de persistencia
// =============================================================================

// Save guarda una notificación con idempotencia
// Alineado con schema: id, user_id, template_code, type, channel, subject, content, data, status...
func (r *NotificationRepository) Save(ctx context.Context, n *domain.Notification) (existingID uuid.UUID, err error) {
	// Verificar idempotency: si ya existe una notificación enviada para este user + type + template_code
	existingQuery := `
		SELECT id FROM notifications 
		WHERE user_id = $1 AND type = $2 AND template_code = $3 AND status = 'sent'
	`
	var existingUUID uuid.UUID
	err = r.db.QueryRow(ctx, existingQuery, n.UserID, n.Type, n.TemplateCode).Scan(&existingUUID)
	if err == nil {
		// Ya existe una notificación enviada para este user + type
		return existingUUID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("check existing notification: %w", err)
	}

	// Insertar nueva notificación (con todas las columnas requeridas por la migración)
	query := `
		INSERT INTO notifications (
			id, user_id, template_code, type, channel, subject, content,
			data, status, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = r.db.Exec(ctx, query,
		n.ID,
		n.UserID,
		n.TemplateCode,
		n.Type,
		n.Channel,
		n.Subject,
		n.Content,
		n.Data,
		n.Status,
		n.Metadata,
		n.CreatedAt,
		n.UpdatedAt,
	)

	if err != nil {
		return uuid.Nil, fmt.Errorf("insert notification: %w", err)
	}

	return uuid.Nil, nil
}

// GetByID obtiene una notificación por ID
func (r *NotificationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	query := `
		SELECT id, user_id, template_code, type, channel, subject, content,
		       data, status, sent_at, delivered_at, opened_at, error_message,
		       provider_message_id, metadata, created_at, updated_at
		FROM notifications
		WHERE id = $1
	`

	var n domain.Notification
	err := r.db.QueryRow(ctx, query, id).Scan(
		&n.ID,
		&n.UserID,
		&n.TemplateCode,
		&n.Type,
		&n.Channel,
		&n.Subject,
		&n.Content,
		&n.Data,
		&n.Status,
		&n.SentAt,
		&n.DeliveredAt,
		&n.OpenedAt,
		&n.ErrorMessage,
		&n.ProviderMessageID,
		&n.Metadata,
		&n.CreatedAt,
		&n.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get notification: %w", err)
	}

	return &n, nil
}

// MarkSent actualiza estado a enviado con provider message ID
func (r *NotificationRepository) MarkSent(ctx context.Context, id uuid.UUID, providerMessageID string) error {
	query := `
		UPDATE notifications
		SET status = 'sent', provider_message_id = $2, sent_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id, providerMessageID)
	if err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

// MarkDelivered actualiza estado a entregado (desde webhook).
// Usa el timestamp provisto por el proveedor para delivered_at.
func (r *NotificationRepository) MarkDelivered(ctx context.Context, id uuid.UUID, deliveredAt time.Time) error {
	query := `
		UPDATE notifications
		SET status = 'delivered', delivered_at = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id, deliveredAt)
	if err != nil {
		return fmt.Errorf("mark delivered: %w", err)
	}
	return nil
}

// MarkFailed registra intento fallido
func (r *NotificationRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	query := `
		UPDATE notifications
		SET status = 'failed', error_message = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id, errMsg)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// UpdateFromWebhook actualiza una notificación desde datos del webhook del proveedor.
// Usa el timestamp del evento del proveedor para la columna de timestamp
// correspondiente según el estado reportado (delivered_at, opened_at).
func (r *NotificationRepository) UpdateFromWebhook(ctx context.Context, providerMessageID string, status domain.NotificationStatus, eventTimestamp time.Time) error {
	query := `
		UPDATE notifications
		SET status = $1,
		    delivered_at = CASE WHEN $1 = 'delivered' THEN $3 ELSE delivered_at END,
		    opened_at    = CASE WHEN $1 = 'opened'    THEN $3 ELSE opened_at END,
		    updated_at = NOW()
		WHERE provider_message_id = $2
	`

	_, err := r.db.Exec(ctx, query, status, providerMessageID, eventTimestamp)
	if err != nil {
		return fmt.Errorf("update from webhook: %w", err)
	}
	return nil
}

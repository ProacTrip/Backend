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
// Notification Repository - PostgreSQL adapter
// Alineado con migración 001_notifications.sql (fuente de truth)
// =============================================================================

type NotificationRepository struct {
	db *pgxpool.Pool
}

// =============================================================================
// Constructor
// =============================================================================

func NewNotificationRepository(db *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// =============================================================================
// Operaciones de persistencia
// =============================================================================

// Save guarda una notificación con idempotencia
// Alineado con schema: id, user_id, template_code, type, channel, subject, content, data, status...
func (r *NotificationRepository) Save(ctx context.Context, n *domain.Notification) (existingID uuid.UUID, err error) {
	// Verificar idempotency: si ya existe una notificación enviada para este user + type
	existingQuery := `
		SELECT id FROM notifications 
		WHERE user_id = $1 AND type = $2 AND status = 'sent'
	`
	var existingUUID uuid.UUID
	err = r.db.QueryRow(ctx, existingQuery, n.UserID, n.Type).Scan(&existingUUID)
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

// MarkDelivered actualiza estado a entregado (desde webhook)
func (r *NotificationRepository) MarkDelivered(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE notifications
		SET status = 'delivered', delivered_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id)
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

// GetPending retorna notificaciones pendientes para reintento
func (r *NotificationRepository) GetPending(ctx context.Context, olderThan time.Duration) ([]*domain.Notification, error) {
	query := `
		SELECT id, user_id, template_code, type, channel, subject, content,
		       data, status, sent_at, delivered_at, opened_at, error_message,
		       provider_message_id, metadata, created_at, updated_at
		FROM notifications
		WHERE status IN ('pending', 'failed')
		  AND created_at < $1
		ORDER BY created_at ASC
		LIMIT 100
	`

	rows, err := r.db.Query(ctx, query, time.Now().Add(-olderThan))
	if err != nil {
		return nil, fmt.Errorf("get pending: %w", err)
	}
	defer rows.Close()

	var results []*domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}
		results = append(results, &n)
	}

	return results, nil
}

// UpdateFromWebhook actualiza una notificación desde datos del webhook del proveedor
func (r *NotificationRepository) UpdateFromWebhook(ctx context.Context, providerMessageID string, status domain.NotificationStatus) error {
	query := `
		UPDATE notifications
		SET status = $2, 
		    provider_message_id = COALESCE(provider_message_id, $3),
		    updated_at = NOW()
		WHERE provider_message_id = $3
	`

	_, err := r.db.Exec(ctx, query, status, providerMessageID, providerMessageID)
	if err != nil {
		return fmt.Errorf("update from webhook: %w", err)
	}
	return nil
}

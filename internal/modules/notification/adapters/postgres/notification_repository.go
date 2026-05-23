// Adapter de PostgreSQL para notificaciones.
// Implementa la interfaz del dominio.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
)

// =============================================================================
// Repositorio de Notificaciones — adapter PostgreSQL
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

// Save guarda una notificación con idempotencia.
// Si ya existe una notificación enviada para este user + template_code, retorna el ID existente.
func (r *NotificationRepository) Save(ctx context.Context, n *domain.Notification) (existingID uuid.UUID, err error) {
	// Idempotency check: si ya existe con sent_at NOT NULL para este user + template_code
	existingQuery := `
		SELECT id FROM notifications
		WHERE user_id = $1 AND template_code = $2 AND sent_at IS NOT NULL
	`
	var existingUUID uuid.UUID
	err = r.db.QueryRow(ctx, existingQuery, n.UserID, n.TemplateCode).Scan(&existingUUID)
	if err == nil {
		return existingUUID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("check existing notification: %w", err)
	}

	// Insertar nueva notificación
	query := `
		INSERT INTO notifications (
			id, user_id, template_code, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5)
	`

	_, err = r.db.Exec(ctx, query,
		n.ID,
		n.UserID,
		n.TemplateCode,
		n.CreatedAt,
		n.UpdatedAt,
	)

	if err != nil {
		return uuid.Nil, fmt.Errorf("insert notification: %w", err)
	}

	return uuid.Nil, nil
}

// GetByID obtiene una notificación por ID.
func (r *NotificationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	query := `
		SELECT id, user_id, template_code, sent_at, created_at, updated_at
		FROM notifications
		WHERE id = $1
	`

	var n domain.Notification
	err := r.db.QueryRow(ctx, query, id).Scan(
		&n.ID,
		&n.UserID,
		&n.TemplateCode,
		&n.SentAt,
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

// MarkSent actualiza sent_at.
func (r *NotificationRepository) MarkSent(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE notifications
		SET sent_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

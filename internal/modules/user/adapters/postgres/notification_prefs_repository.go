// Adapter PostgreSQL para preferencias de notificación.
// Implementa domain.NotificationPrefsRepository.
package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// NotificationPrefsRepository — PostgreSQL adapter
// Alineado con migración user_notification_preferences
// =============================================================================

// Compile-time interface check
var _ domain.NotificationPrefsRepository = (*NotificationPrefsRepository)(nil)

type NotificationPrefsRepository struct {
	db *pgxpool.Pool
}

func NewNotificationPrefsRepository(db *pgxpool.Pool) *NotificationPrefsRepository {
	return &NotificationPrefsRepository{db: db}
}

// =============================================================================
// Create — Inserta una nueva preferencia
// =============================================================================

func (r *NotificationPrefsRepository) Create(ctx context.Context, pref *domain.NotificationPreference) error {
	query := `
		INSERT INTO user_notification_preferences (
			id, user_id, channel, notification_type, enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.Exec(ctx, query,
		pref.ID,
		pref.UserID,
		string(pref.Channel),
		string(pref.NotificationType),
		pref.Enabled,
		pref.CreatedAt,
		pref.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create notification preference: %w", err)
	}
	return nil
}

// =============================================================================
// GetByUserID — Recupera todas las preferencias de un usuario
// =============================================================================

func (r *NotificationPrefsRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	query := `
		SELECT id, user_id, channel, notification_type, enabled, created_at, updated_at
		FROM user_notification_preferences
		WHERE user_id = $1
		ORDER BY channel, notification_type
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get notification preferences by user_id: %w", err)
	}
	defer rows.Close()

	var prefs []*domain.NotificationPreference
	for rows.Next() {
		var np domain.NotificationPreference
		if err := rows.Scan(
			&np.ID,
			&np.UserID,
			&np.Channel,
			&np.NotificationType,
			&np.Enabled,
			&np.CreatedAt,
			&np.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification preference: %w", err)
		}
		prefs = append(prefs, &np)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification preferences: %w", err)
	}

	if len(prefs) == 0 {
		return nil, domain.ErrNotifPrefsNotFound
	}

	return prefs, nil
}

// =============================================================================
// Upsert — INSERT ON CONFLICT UPDATE
// =============================================================================

func (r *NotificationPrefsRepository) Upsert(ctx context.Context, pref *domain.NotificationPreference) error {
	query := `
		INSERT INTO user_notification_preferences (
			id, user_id, channel, notification_type, enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, channel, notification_type) DO UPDATE SET
			enabled    = EXCLUDED.enabled,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Exec(ctx, query,
		pref.ID,
		pref.UserID,
		string(pref.Channel),
		string(pref.NotificationType),
		pref.Enabled,
		pref.CreatedAt,
		pref.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert notification preference: %w", err)
	}
	return nil
}

// =============================================================================
// Delete — Elimina una preferencia específica
// =============================================================================

func (r *NotificationPrefsRepository) Delete(ctx context.Context, userID uuid.UUID, channel domain.NotificationChannel, notifType domain.NotificationType) error {
	query := `
		DELETE FROM user_notification_preferences
		WHERE user_id = $1 AND channel = $2 AND notification_type = $3
	`

	_, err := r.db.Exec(ctx, query, userID, string(channel), string(notifType))
	if err != nil {
		return fmt.Errorf("delete notification preference: %w", err)
	}
	return nil
}

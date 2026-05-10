// Caso de uso: Actualizar preferencia de notificación (PUT /v1/user/profile/notifications).
// Siempre hace upsert idempotente por (user_id, channel, notification_type).
// enabled=false preserva el registro para audit trail.
package update_notif_prefs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Ports
// =============================================================================

type NotifPrefsRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error)
	Upsert(ctx context.Context, pref *domain.NotificationPreference) error
	Delete(ctx context.Context, userID uuid.UUID, channel domain.NotificationChannel, notifType domain.NotificationType) error
}

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

type UseCaseDeps struct {
	NotifPrefsRepo  NotifPrefsRepo
	EventPublisher  EventPublisher
}

type UseCase struct {
	notifPrefsRepo  NotifPrefsRepo
	eventPublisher  EventPublisher
	wg              sync.WaitGroup
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		notifPrefsRepo: deps.NotifPrefsRepo,
		eventPublisher: deps.EventPublisher,
	}
}

// Wait espera a que todos los eventos publicados asíncronamente terminen.
func (uc *UseCase) Wait() { uc.wg.Wait() }

// Execute valida el comando (UserID, Channel, NotificationType) y hace upsert idempotente.
// La validación de canal y tipo ocurre en cmd.Validate().
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	if err := cmd.Validate(); err != nil {
		return err
	}

	userID := uuid.MustParse(cmd.UserID)
	channel := domain.NotificationChannel(cmd.Channel)
	notifType := domain.NotificationType(cmd.NotificationType)

	// SMS está planeado pero no implementado — loggear advertencia
	if channel == domain.NotifChannelSMS {
		slog.WarnContext(ctx, "SMS notification channel is planned but not yet implemented",
			slog.String("user_id", userID.String()),
		)
	}

	// Upsert idempotente: buscar preferencia existente por user+channel+type,
	// actualizar si existe, crear si no.
	var pref *domain.NotificationPreference
	existingPrefs, err := uc.notifPrefsRepo.GetByUserID(ctx, userID)
	if err != nil && !errors.Is(err, domain.ErrNotifPrefsNotFound) {
		return fmt.Errorf("get existing notification preferences: %w", err)
	}
	for _, p := range existingPrefs {
		if p.Channel == channel && p.NotificationType == notifType {
			pref = p
			break
		}
	}

	if pref != nil {
		// Actualizar preferencia existente — reutiliza UUID y CreatedAt
		pref.Enabled = cmd.Enabled
		pref.UpdatedAt = time.Now()
	} else {
		// Crear nueva preferencia
		pref = domain.NewNotificationPreference(userID, channel, notifType)
		pref.Enabled = cmd.Enabled
	}

	if err := uc.notifPrefsRepo.Upsert(ctx, pref); err != nil {
		return fmt.Errorf("upsert notification preference: %w", err)
	}

	// Emit event (best-effort)
	if uc.eventPublisher != nil {
		uc.wg.Go(func() {
			bgCtx := context.WithoutCancel(ctx)
			_, err := uc.eventPublisher.Publish(bgCtx,
				eventbus.StreamName("user.notification_preferences.updated"),
				map[string]interface{}{
					"user_id": userID.String(),
				},
			)
			if err != nil {
				slog.WarnContext(bgCtx, "publish notification preferences updated event failed",
					slog.String("user_id", userID.String()),
					slog.String("error", err.Error()),
				)
			}
		})
	}

	return nil
}
